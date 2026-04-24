package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	iam_middleware "github.com/openguard/services/iam/pkg/middleware"
	"github.com/openguard/services/iam/pkg/service"
	shared_middleware "github.com/openguard/shared/middleware"
)

// Handler manages HTTP requests for the IAM service.
type Handler struct {
	svc *service.Service
}

// NewHandler creates a new handler instance.
func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "OK"})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userAgent := r.UserAgent()
	ip := r.RemoteAddr

	user, token, err := h.svc.Login(r.Context(), body.Email, body.Password, userAgent, ip)
	if err != nil {
		slog.Error("login failed", "error", err, "email", body.Email)
		if err.Error() == "USER_PROVISIONING_IN_PROGRESS" {
			h.writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"error": "User provisioning in progress",
				"code":  "USER_PROVISIONING_IN_PROGRESS",
			})
			return
		}
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Handle MFA requirement (R-11)
	if mfaRequired, ok := user["mfa_required"].(bool); ok && mfaRequired {
		h.writeJSON(w, http.StatusAccepted, map[string]interface{}{
			"mfa_required":  true,
			"mfa_challenge": user["mfa_challenge"],
		})
		return
	}

	// Set HttpOnly cookie for session management per spec §5
	env := os.Getenv("ENV")
	secure := env == "prod"
	if cookieSecure := os.Getenv("COOKIE_SECURE"); cookieSecure != "" {
		secure = cookieSecure == "true"
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "openguard_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600, // 1 hour
	})

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":         user,
		"access_token": token,
	})
}

func (h *Handler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChallengeToken string `json:"mfa_challenge"`
		Code           string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, token, err := h.svc.VerifyMFAAndLogin(r.Context(), body.ChallengeToken, body.Code, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		http.Error(w, "Invalid MFA code", http.StatusUnauthorized)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "openguard_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":         user,
		"access_token": token,
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	res, err := h.svc.RefreshToken(r.Context(), body.RefreshToken, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		http.Error(w, "Invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	h.writeJSON(w, http.StatusOK, res)
}

func (h *Handler) OAuthLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		ClientID    string `json:"client_id"`
		RedirectURI string `json:"redirect_uri"`
		State       string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	user, _, err := h.svc.Login(r.Context(), body.Email, body.Password, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate auth code (R-03)
	code := uuid.New().String()
	err = h.svc.StoreAuthCode(r.Context(), code, user["org_id"].(string), user["id"].(string))
	if err != nil {
		http.Error(w, "failed to store auth code", http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"code":  code,
		"state": body.State,
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID := shared_middleware.GetUserID(r.Context())
	if userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.svc.GetCurrentUser(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// Extract JTI and expiry injected by auth middleware
	jti := shared_middleware.GetJTI(r.Context())
	expiresAt := shared_middleware.GetExpiresAt(r.Context())

	if jti == "" {
		http.Error(w, "invalid session: missing jti", http.StatusBadRequest)
		return
	}

	if err := h.svc.Logout(r.Context(), jti, expiresAt); err != nil {
		log := iam_middleware.GetLogger(r.Context())
		log.Error("logout failed", zap.Error(err))
		http.Error(w, "logout failed", http.StatusInternalServerError)
		return
	}

	// Clear the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "openguard_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})

	h.writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
}

func (h *Handler) CreateOrg(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := h.svc.RegisterOrg(r.Context(), body.Name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Pull org_id from context per spec §5
	orgID := shared_middleware.GetOrgID(r.Context())
	if orgID == "" {
		http.Error(w, "Unauthorized: missing org_id", http.StatusUnauthorized)
		return
	}

	tr := otel.Tracer("iam-service")
	ctx, span := tr.Start(r.Context(), "CreateUser")
	defer span.End()

	id, created, err := h.svc.RegisterUser(ctx, orgID, body.Email, body.Password, body.DisplayName, body.Role, "")
	if err != nil {
		log := iam_middleware.GetLogger(ctx)
		log.Error("CreateUser failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) ListConnectors(w http.ResponseWriter, r *http.Request) {
	connectors, err := h.svc.ListConnectors(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, http.StatusOK, connectors)
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	orgID := shared_middleware.GetOrgID(r.Context())
	filter := r.URL.Query().Get("filter")
	users, err := h.svc.ListUsers(r.Context(), orgID, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, http.StatusOK, users)
}

func (h *Handler) CreateConnector(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		ClientSecret string   `json:"client_secret"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orgID, err := h.svc.CreateConnector(r.Context(), body.ID, body.Name, body.ClientSecret, body.RedirectURIs)
	if err != nil {
		log := iam_middleware.GetLogger(r.Context())
		log.Error("CreateConnector failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"id": body.ID, "org_id": orgID})
}

func (h *Handler) TOTPSetup(w http.ResponseWriter, r *http.Request) {
	userID := shared_middleware.GetUserID(r.Context())
	// Get user email for TOTP label
	user, err := h.svc.GetCurrentUser(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	secret, url, err := h.svc.GenerateTOTPSetup(r.Context(), userID, user["email"].(string))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{
		"secret": secret,
		"url":    url,
	})
}

func (h *Handler) TOTPEnable(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code   string `json:"code"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	userID := shared_middleware.GetUserID(r.Context())
	orgID := shared_middleware.GetOrgID(r.Context())

	backupCodes, err := h.svc.EnableTOTP(r.Context(), orgID, userID, body.Code, body.Secret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":       "mfa_enabled",
		"backup_codes": backupCodes,
	})
}

func (h *Handler) VerifyBackupCode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChallengeToken string `json:"mfa_challenge"`
		Code           string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 1. Get userID from challenge (logic repeated from VerifyMFA for simplicity)
	// Ideally VerifyMFAAndLogin should handle both TOTP and Backup Codes
	// But the spec says POST /auth/mfa/backup-verify
	
	// I'll update VerifyMFAAndLogin in service.go to handle both, 
	// OR I'll implement the logic here.
	
	// Actually, let's keep it simple as per prompt:
	// "Add a POST /auth/mfa/backup-verify endpoint and wire it to VerifyBackupCode"
	
	// Wait, VerifyBackupCode in service.go takes userID. 
	// I need to get userID from challengeToken first.
	
	// I'll add a helper to service.go to get userID from challenge or just use VerifyBackupCode.
	// Actually, I'll update the handler to use a new service method that handles the challenge too.
	
	user, token, err := h.svc.VerifyBackupCodeAndLogin(r.Context(), body.ChallengeToken, body.Code, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		http.Error(w, "Invalid backup code", http.StatusUnauthorized)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "openguard_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":         user,
		"access_token": token,
	})
}

func (h *Handler) UpdateConnector(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Name         string   `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.UpdateConnector(r.Context(), id, body.Name, body.RedirectURIs); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) DeleteConnector(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.DeleteConnector(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ReprovisionUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := shared_middleware.GetOrgID(r.Context())
	if orgID == "" {
		http.Error(w, "Unauthorized: missing org_id", http.StatusUnauthorized)
		return
	}

	if err := h.svc.ReprovisionUser(r.Context(), orgID, id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "reprovisioning_started"})
}

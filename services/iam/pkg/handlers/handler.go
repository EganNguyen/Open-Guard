package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"

	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"

	"github.com/go-webauthn/webauthn/webauthn"
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

func (h *Handler) SetServiceWebAuthn(w *webauthn.WebAuthn) {
	h.svc.SetWebAuthn(w)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]string{"error": msg})
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
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userAgent := r.UserAgent()
	ip := r.RemoteAddr

	user, token, err := h.svc.Login(r.Context(), body.Email, body.Password, userAgent, ip)
	if err != nil {
		slog.Error("login failed", "error", err, "email", body.Email)
		if errors.Is(err, service.ErrAccountSetup) {
			h.writeJSON(w, http.StatusForbidden, errorResponse{
				Error: "Account setup in progress",
				Code:  "ACCOUNT_SETUP_PENDING",
			})
			return
		}
		h.writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	// Handle MFA requirement (R-11)
	if token != nil && user.Email == "" && user.OrgID != "" {
		// This is the challenge token return path from our refactored Login
		h.writeJSON(w, http.StatusAccepted, mfaChallengeResponse{
			MFARequired:  true,
			MFAChallenge: token.AccessToken,
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
		Value:    token.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600, // 1 hour
	})

	h.writeJSON(w, http.StatusOK, loginResponse{
		User:         user,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    token.ExpiresIn,
	})
}

func (h *Handler) VerifyMFA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ChallengeToken string `json:"mfa_challenge"`
		Code           string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, token, err := h.svc.VerifyMFAAndLogin(r.Context(), body.ChallengeToken, body.Code, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, "Invalid MFA code")
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "openguard_session",
		Value:    token.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})

	h.writeJSON(w, http.StatusOK, loginResponse{
		User:         user,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    token.ExpiresIn,
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	res, err := h.svc.RefreshToken(r.Context(), body.RefreshToken, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		if errors.Is(err, service.ErrSessionRevokedRisk) {
			h.writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Session revoked due to suspicious activity",
				"code":  "SESSION_REVOKED_RISK",
			})
			return
		}
		if errors.Is(err, service.ErrSessionCompromised) {
			h.writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Session compromised — refresh token reuse detected",
				"code":  "SESSION_COMPROMISED",
			})
			return
		}
		h.writeError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	h.writeJSON(w, http.StatusOK, res)
}

func (h *Handler) OAuthLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		ClientID      string `json:"client_id"`
		RedirectURI   string `json:"redirect_uri"`
		State         string `json:"state"`
		CodeChallenge string `json:"code_challenge"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, _, err := h.svc.Login(r.Context(), body.Email, body.Password, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if body.CodeChallenge == "" {
		h.writeError(w, http.StatusBadRequest, "code_challenge is required for OAuth login")
		return
	}

	// Generate auth code (R-03)
	code := uuid.New().String()
	err = h.svc.StoreAuthCode(r.Context(), code, user.OrgID, user.ID, body.CodeChallenge)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to store auth code")
		return
	}

	h.writeJSON(w, http.StatusOK, genericResponse{
		ID:     code,
		Status: body.State, // Borrowing Status for state
	})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID := shared_middleware.GetUserID(r.Context())
	if userID == "" {
		h.writeError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.svc.GetCurrentUser(r.Context(), userID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, loginResponse{
		User: user,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	// Extract JTI and expiry injected by auth middleware
	jti := shared_middleware.GetJTI(r.Context())
	expiresAt := shared_middleware.GetExpiresAt(r.Context())

	if jti == "" {
		h.writeError(w, http.StatusBadRequest, "invalid session: missing jti")
		return
	}

	if err := h.svc.Logout(r.Context(), jti, expiresAt); err != nil {
		log := iam_middleware.GetLogger(r.Context())
		log.Error("logout failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "logout failed")
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
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id, err := h.svc.RegisterOrg(r.Context(), body.Name)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgID       string `json:"org_id"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Pull org_id from context per spec §5
	ctxOrgID := shared_middleware.GetOrgID(r.Context())
	if ctxOrgID == "" {
		h.writeError(w, http.StatusUnauthorized, "Unauthorized: missing org_id")
		return
	}

	// If body.OrgID is provided, check if the caller is system admin (org_id = 0000...)
	// Otherwise, use the ctxOrgID
	targetOrgID := body.OrgID
	if targetOrgID == "" || ctxOrgID != "00000000-0000-0000-0000-000000000000" {
		targetOrgID = ctxOrgID
	}

	tr := otel.Tracer("iam-service")
	ctx, span := tr.Start(r.Context(), "CreateUser")
	defer span.End()

	id, _, err := h.svc.RegisterUser(ctx, service.RegisterUserRequest{
		OrgID:          targetOrgID,
		Email:          body.Email,
		Password:       body.Password,
		DisplayName:    body.DisplayName,
		Role:           body.Role,
		SCIMExternalID: "",
	})
	if err != nil {
		log := iam_middleware.GetLogger(ctx)
		log.Error("CreateUser failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handler) ListConnectors(w http.ResponseWriter, r *http.Request) {
	connectors, err := h.svc.ListConnectors(r.Context())
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, connectors)
}

func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	orgID := shared_middleware.GetOrgID(r.Context())
	filter := r.URL.Query().Get("filter")
	users, err := h.svc.ListUsers(r.Context(), orgID, filter)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
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
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	orgID, err := h.svc.CreateConnector(r.Context(), body.ID, body.Name, body.ClientSecret, body.RedirectURIs)
	if err != nil {
		log := iam_middleware.GetLogger(r.Context())
		log.Error("CreateConnector failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"id": body.ID, "org_id": orgID})
}

func (h *Handler) TOTPSetup(w http.ResponseWriter, r *http.Request) {
	userID := shared_middleware.GetUserID(r.Context())
	// Get user email for TOTP label
	user, err := h.svc.GetCurrentUser(r.Context(), userID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	secret, url, err := h.svc.GenerateTOTPSetup(r.Context(), userID, user.Email)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
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
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID := shared_middleware.GetUserID(r.Context())
	orgID := shared_middleware.GetOrgID(r.Context())

	backupCodes, err := h.svc.EnableTOTP(r.Context(), orgID, userID, body.Code, body.Secret)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
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
		h.writeError(w, http.StatusBadRequest, "invalid request body")
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
		h.writeError(w, http.StatusUnauthorized, "Invalid backup code")
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "openguard_session",
		Value:    token.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})

	h.writeJSON(w, http.StatusOK, loginResponse{
		User:         user,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    token.ExpiresIn,
	})
}

func (h *Handler) UpdateConnector(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Name         string   `json:"name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.UpdateConnector(r.Context(), id, body.Name, body.RedirectURIs); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *Handler) DeleteConnector(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.svc.DeleteConnector(r.Context(), id); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) ReprovisionUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	orgID := shared_middleware.GetOrgID(r.Context())
	if orgID == "" {
		h.writeError(w, http.StatusUnauthorized, "Unauthorized: missing org_id")
		return
	}

	if err := h.svc.ReprovisionUser(r.Context(), orgID, id); err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{"status": "reprovisioning_started"})
}

func (h *Handler) WebAuthnBeginRegistration(w http.ResponseWriter, r *http.Request) {
	userID := shared_middleware.GetUserID(r.Context())
	sessionID, _, options, err := h.svc.BeginWebAuthnRegistration(r.Context(), userID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, webAuthnBeginResponse{
		SessionID: sessionID,
		Options:   options,
	})
}

func (h *Handler) WebAuthnFinishRegistration(w http.ResponseWriter, r *http.Request) {
	userID := shared_middleware.GetUserID(r.Context())
	orgID := shared_middleware.GetOrgID(r.Context())
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		h.writeError(w, http.StatusBadRequest, "missing session_id")
		return
	}
	if err := h.svc.FinishWebAuthnRegistration(r.Context(), orgID, userID, sessionID, r); err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

func (h *Handler) WebAuthnBeginLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sessionID, _, options, err := h.svc.BeginWebAuthnLogin(r.Context(), body.Email)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, webAuthnBeginResponse{
		SessionID: sessionID,
		Options:   options,
	})
}

func (h *Handler) WebAuthnFinishLogin(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		h.writeError(w, http.StatusBadRequest, "missing session_id")
		return
	}
	user, token, err := h.svc.FinishWebAuthnLogin(r.Context(), email, sessionID, r.UserAgent(), r.RemoteAddr, r)
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "openguard_session",
		Value:    token.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   3600,
	})

	h.writeJSON(w, http.StatusOK, loginResponse{
		User:         user,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    token.ExpiresIn,
	})
}

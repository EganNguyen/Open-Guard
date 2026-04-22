package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/openguard/services/iam/pkg/middleware"
	"github.com/openguard/services/iam/pkg/service"
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

	user, err := h.svc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]interface{}{
		"user": user,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
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
		OrgID       string `json:"org_id"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tr := otel.Tracer("iam-service")
	ctx, span := tr.Start(r.Context(), "CreateUser")
	defer span.End()

	id, err := h.svc.RegisterUser(ctx, body.OrgID, body.Email, body.Password, body.DisplayName, body.Role)
	if err != nil {
		log := middleware.GetLogger(ctx)
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
	users, err := h.svc.ListUsers(r.Context())
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
		log := middleware.GetLogger(r.Context())
		log.Error("CreateConnector failed", zap.Error(err))
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusCreated, map[string]string{"id": body.ID, "org_id": orgID})
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

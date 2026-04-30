package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/shared/crypto"
	shared_middleware "github.com/openguard/shared/middleware"
	"github.com/openguard/shared/rls"
)

// SCIM v2 Models
type scimUser struct {
	Schemas     []string `json:"schemas"`
	ID          string   `json:"id"`
	ExternalID  string   `json:"externalId,omitempty"`
	UserName    string   `json:"userName"`
	DisplayName string   `json:"displayName"`
	Active      bool     `json:"active"`
	Emails      []struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	} `json:"emails"`
	Meta struct {
		ResourceType string `json:"resourceType"`
		Created      string `json:"created"`
		LastModified string `json:"lastModified"`
		Version      string `json:"version"`
		Location     string `json:"location"`
	} `json:"meta"`
}

type scimListResponse struct {
	Schemas      []string   `json:"schemas"`
	TotalResults int        `json:"totalResults"`
	StartIndex   int        `json:"startIndex"`
	ItemsPerPage int        `json:"itemsPerPage"`
	Resources    []scimUser `json:"Resources"`
}

func (h *Handler) ListScimUsers(w http.ResponseWriter, r *http.Request) {
	// org_id is derived from the validated SCIM bearer token — never from request headers.
	orgID := shared_middleware.GetSCIMOrgID(r.Context())
	if orgID == "" {
		h.writeScimError(w, http.StatusUnauthorized, "unauthorized", "Missing SCIM context")
		return
	}
	ctx := rls.WithOrgID(r.Context(), orgID)

	startIndex := max(parseIntParam(r.URL.Query().Get("startIndex"), 1), 1)
	count := min(parseIntParam(r.URL.Query().Get("count"), 100), 1000)
	offset := startIndex - 1 // SCIM is 1-indexed

	filter := r.URL.Query().Get("filter")

	users, total, err := h.svc.ListUsersPaginated(ctx, orgID, filter, offset, count)
	if err != nil {
		h.writeScimError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	scimUsers := []scimUser{}
	for _, u := range users {
		scimUsers = append(scimUsers, h.mapToScim(&u))
	}

	h.writeJSON(w, http.StatusOK, scimListResponse{
		Schemas:      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		TotalResults: total,
		StartIndex:   startIndex,
		ItemsPerPage: len(scimUsers),
		Resources:    scimUsers,
	})
}

func (h *Handler) PostScimUser(w http.ResponseWriter, r *http.Request) {
	// org_id is derived from the validated SCIM bearer token — never from request headers.
	orgID := shared_middleware.GetSCIMOrgID(r.Context())
	if orgID == "" {
		h.writeScimError(w, http.StatusUnauthorized, "unauthorized", "Missing SCIM context")
		return
	}
	ctx := rls.WithOrgID(r.Context(), orgID)

	var payload scimUser
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeScimError(w, http.StatusBadRequest, "invalidSyntax", err.Error())
		return
	}

	email := ""
	for _, e := range payload.Emails {
		if e.Primary {
			email = e.Value
			break
		}
	}
	if email == "" && len(payload.Emails) > 0 {
		email = payload.Emails[0].Value
	}

	// Password might be generated or provided in a different field for SCIM,
	// but here we use a random one if not provided.
	password := crypto.GenerateRandomString(32)

	id, created, err := h.svc.RegisterUser(ctx, service.RegisterUserRequest{
		OrgID:          orgID,
		Email:          email,
		Password:       password,
		DisplayName:    payload.DisplayName,
		Role:           "user",
		SCIMExternalID: payload.ExternalID,
	})
	if err != nil {
		if strings.HasPrefix(err.Error(), "CONFLICT:") {
			h.writeScimError(w, http.StatusConflict, "conflict", strings.TrimPrefix(err.Error(), "CONFLICT:"))
			return
		}
		h.writeScimError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	user, _ := h.svc.GetCurrentUser(ctx, id)

	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}

	h.writeJSON(w, status, h.mapToScim(user))
}

func (h *Handler) GetScimUser(w http.ResponseWriter, r *http.Request) {
	// org_id is derived from the validated SCIM bearer token — never from request headers.
	orgID := shared_middleware.GetSCIMOrgID(r.Context())
	ctx := rls.WithOrgID(r.Context(), orgID)
	userID := chi.URLParam(r, "id")
	user, err := h.svc.GetCurrentUser(ctx, userID)
	if err != nil || user.Status == "deprovisioned" {
		h.writeScimError(w, http.StatusNotFound, "notFound", "User not found")
		return
	}

	h.writeJSON(w, http.StatusOK, h.mapToScim(user))
}

func (h *Handler) DeleteScimUser(w http.ResponseWriter, r *http.Request) {
	// org_id is derived from the validated SCIM bearer token — never from request headers.
	orgID := shared_middleware.GetSCIMOrgID(r.Context())
	ctx := rls.WithOrgID(r.Context(), orgID)
	id := chi.URLParam(r, "id")
	if err := h.svc.DeleteUser(ctx, id); err != nil {
		h.writeScimError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent) // 204
}

func (h *Handler) PatchScimUser(w http.ResponseWriter, r *http.Request) {
	// org_id is derived from the validated SCIM bearer token — never from request headers.
	orgID := shared_middleware.GetSCIMOrgID(r.Context())
	ctx := rls.WithOrgID(r.Context(), orgID)
	id := chi.URLParam(r, "id")
	var body struct {
		Schemas    []string              `json:"schemas"`
		Operations []service.ScimPatchOp `json:"Operations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeScimError(w, http.StatusBadRequest, "invalidSyntax", err.Error())
		return
	}

	user, err := h.svc.PatchUser(ctx, id, body.Operations)
	if err != nil {
		h.writeScimError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, h.mapToScim(user))
}

func (h *Handler) mapToScim(user *iam_repo.User) scimUser {
	s := scimUser{
		Schemas:     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:          user.ID,
		UserName:    user.Email,
		DisplayName: user.DisplayName,
		Active:      user.Status == "active",
	}
	if user.SCIMExternalID != nil {
		s.ExternalID = *user.SCIMExternalID
	}
	s.Emails = append(s.Emails, struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	}{Value: user.Email, Primary: true})

	s.Meta.ResourceType = "User"
	s.Meta.Version = fmt.Sprintf("v%d", user.Version)
	s.Meta.Location = fmt.Sprintf("/scim/v2/Users/%s", s.ID)

	return s
}

func (h *Handler) writeScimError(w http.ResponseWriter, status int, scimType string, detail string) {
	w.Header().Set("Content-Type", "application/scim+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"schemas":  []string{"urn:ietf:params:scim:api:messages:2.0:Error"},
		"scimType": scimType,
		"detail":   detail,
		"status":   fmt.Sprintf("%d", status),
	})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func parseIntParam(s string, def int) int {
	if s == "" {
		return def
	}
	var res int
	if _, err := fmt.Sscanf(s, "%d", &res); err != nil {
		return def
	}
	return res
}

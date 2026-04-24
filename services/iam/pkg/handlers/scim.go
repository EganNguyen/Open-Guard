package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/shared/crypto"
)

// SCIM v2 Models
type scimUser struct {
	Schemas    []string `json:"schemas"`
	ID         string   `json:"id"`
	ExternalID string   `json:"externalId,omitempty"`
	UserName   string   `json:"userName"`
	DisplayName string  `json:"displayName"`
	Active     bool     `json:"active"`
	Emails     []struct {
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
	orgID := r.Header.Get("X-Org-ID") // SCIM usually identifies org via URL or header
	filter := r.URL.Query().Get("filter")

	users, err := h.svc.ListUsers(r.Context(), orgID, filter)
	if err != nil {
		h.writeScimError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	scimUsers := []scimUser{}
	for _, u := range users {
		scimUsers = append(scimUsers, h.mapToScim(u))
	}

	h.writeJSON(w, http.StatusOK, scimListResponse{
		Schemas:      []string{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		TotalResults: len(scimUsers),
		StartIndex:   1,
		ItemsPerPage: len(scimUsers),
		Resources:    scimUsers,
	})
}

func (h *Handler) PostScimUser(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	if orgID == "" {
		h.writeScimError(w, http.StatusUnauthorized, "unauthorized", "Missing X-Org-ID")
		return
	}

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

	id, err := h.svc.RegisterUser(r.Context(), orgID, email, password, payload.DisplayName, "user")
	if err != nil {
		h.writeScimError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	user, _ := h.svc.GetCurrentUser(r.Context(), id)
	h.writeJSON(w, http.StatusCreated, h.mapToScim(user))
}

func (h *Handler) GetScimUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	user, err := h.svc.GetCurrentUser(r.Context(), userID)
	if err != nil {
		h.writeScimError(w, http.StatusNotFound, "notFound", "User not found")
		return
	}

	h.writeJSON(w, http.StatusOK, h.mapToScim(user))
}

func (h *Handler) mapToScim(user map[string]interface{}) scimUser {
	s := scimUser{
		Schemas:    []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
		ID:         user["id"].(string),
		UserName:   user["email"].(string),
		DisplayName: user["display_name"].(string),
		Active:     user["status"].(string) == "active",
	}
	if extID, ok := user["scim_external_id"].(*string); ok && extID != nil {
		s.ExternalID = *extID
	}
	s.Emails = append(s.Emails, struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	}{Value: user["email"].(string), Primary: true})
	
	s.Meta.ResourceType = "User"
	s.Meta.Version = fmt.Sprintf("v%d", user["version"].(int))
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

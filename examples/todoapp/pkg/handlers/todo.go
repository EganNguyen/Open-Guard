package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/todoapp/pkg/repository"
	"github.com/openguard/todoapp/pkg/sdk"
)

type TodoRepository interface {
	Create(ctx context.Context, orgID string, todo *repository.Todo) error
	List(ctx context.Context, orgID string, userID string) ([]repository.Todo, error)
	Update(ctx context.Context, orgID string, todoID string, completed bool) error
	Delete(ctx context.Context, orgID string, todoID string) error
}

type PolicyClient interface {
	Evaluate(ctx context.Context, req sdk.PolicyRequest) (bool, string, error)
}

type AuditClient interface {
	Ingest(event sdk.AuditEvent)
}

type TodoHandler struct {
	repo   TodoRepository
	policy PolicyClient
	audit  AuditClient
}

func NewTodoHandler(repo TodoRepository, policy PolicyClient, audit AuditClient) *TodoHandler {
	return &TodoHandler{repo: repo, policy: policy, audit: audit}
}

func (h *TodoHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	userID := r.Header.Get("X-User-ID")

	connectorKey := r.Header.Get("X-OpenGuard-Connector-Key")
	permitted, reason, err := h.policy.Evaluate(r.Context(), sdk.PolicyRequest{
		UserID:     userID,
		OrgID:      orgID,
		UserGroups: []string{"admin", "member"},
		Action:     "read",
		Resource:   "todos",
		APIKey:     connectorKey,
	})
	if err != nil || !permitted {
		http.Error(w, fmt.Sprintf("forbidden: %s", reason), http.StatusForbidden)
		return
	}

	todos, err := h.repo.List(r.Context(), orgID, userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(todos)
}

func (h *TodoHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	userID := r.Header.Get("X-User-ID")

	var payload struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid_body", http.StatusBadRequest)
		return
	}

	connectorKey := r.Header.Get("X-OpenGuard-Connector-Key")
	permitted, reason, err := h.policy.Evaluate(r.Context(), sdk.PolicyRequest{
		UserID:     userID,
		OrgID:      orgID,
		UserGroups: []string{"admin", "member"},
		Action:     "write",
		Resource:   "todos",
		APIKey:     connectorKey,
	})
	if err != nil || !permitted {
		http.Error(w, fmt.Sprintf("forbidden: %s", reason), http.StatusForbidden)
		return
	}

	todo := &repository.Todo{
		UserID:    userID,
		OrgID:     orgID,
		Title:     payload.Title,
		Completed: false,
		CreatedAt: time.Now(),
	}

	if err := h.repo.Create(r.Context(), orgID, todo); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit Event
	h.audit.Ingest(sdk.AuditEvent{
		Type:       "todo.created",
		OrgID:      orgID,
		ActorID:    userID,
		ActorType:  "user",
		OccurredAt: time.Now(),
		Source:     "todo-app",
		Payload:    json.RawMessage(fmt.Sprintf(`{"todo_id":"%s", "title":"%s"}`, todo.ID, todo.Title)),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(todo)
}

func (h *TodoHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	userID := r.Header.Get("X-User-ID")
	todoID := chi.URLParam(r, "id")

	var payload struct {
		Completed bool `json:"completed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid_body", http.StatusBadRequest)
		return
	}

	connectorKey := r.Header.Get("X-OpenGuard-Connector-Key")
	permitted, reason, err := h.policy.Evaluate(r.Context(), sdk.PolicyRequest{
		UserID:     userID,
		OrgID:      orgID,
		UserGroups: []string{"admin", "member"},
		Action:     "write",
		Resource:   "todos",
		APIKey:     connectorKey,
	})
	if err != nil || !permitted {
		http.Error(w, fmt.Sprintf("forbidden: %s", reason), http.StatusForbidden)
		return
	}

	if err := h.repo.Update(r.Context(), orgID, todoID, payload.Completed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit Event
	h.audit.Ingest(sdk.AuditEvent{
		Type:       "todo.updated",
		OrgID:      orgID,
		ActorID:    userID,
		ActorType:  "user",
		OccurredAt: time.Now(),
		Source:     "todo-app",
		Payload:    json.RawMessage(fmt.Sprintf(`{"todo_id":"%s", "completed":%v}`, todoID, payload.Completed)),
	})

	w.WriteHeader(http.StatusNoContent)
}

func (h *TodoHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := r.Header.Get("X-Org-ID")
	userID := r.Header.Get("X-User-ID")
	todoID := chi.URLParam(r, "id")

	connectorKey := r.Header.Get("X-OpenGuard-Connector-Key")
	permitted, reason, err := h.policy.Evaluate(r.Context(), sdk.PolicyRequest{
		UserID:     userID,
		OrgID:      orgID,
		UserGroups: []string{"admin", "member"},
		Action:     "delete",
		Resource:   "todos",
		APIKey:     connectorKey,
	})
	if err != nil || !permitted {
		http.Error(w, fmt.Sprintf("forbidden: %s", reason), http.StatusForbidden)
		return
	}

	if err := h.repo.Delete(r.Context(), orgID, todoID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Audit Event
	h.audit.Ingest(sdk.AuditEvent{
		Type:       "todo.deleted",
		OrgID:      orgID,
		ActorID:    userID,
		ActorType:  "user",
		OccurredAt: time.Now(),
		Source:     "todo-app",
		Payload:    json.RawMessage(fmt.Sprintf(`{"todo_id":"%s"}`, todoID)),
	})

	w.WriteHeader(http.StatusNoContent)
}

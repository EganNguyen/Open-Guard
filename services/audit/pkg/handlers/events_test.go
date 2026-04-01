package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/audit/pkg/handlers"
	"github.com/openguard/audit/pkg/models"
	"github.com/openguard/audit/pkg/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"log/slog"
	"os"
)

type mockReadRepo struct {
	events []models.AuditEvent
}

func (m *mockReadRepo) FindEvents(ctx context.Context, filter interface{}, limit int64, skip int64) ([]models.AuditEvent, error) {
	return m.events, nil
}

func (m *mockReadRepo) GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error) {
	return m.events, nil
}

func (m *mockReadRepo) GetLastChainState(ctx context.Context, orgID string) (int64, string, error) {
	return 0, "", nil
}

type mockErrRepo struct {
	mockReadRepo
}

func (m *mockErrRepo) FindEvents(ctx context.Context, filter interface{}, limit int64, skip int64) ([]models.AuditEvent, error) {
	return nil, context.DeadlineExceeded
}

func (m *mockErrRepo) GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error) {
	return nil, context.DeadlineExceeded
}

func TestEventsHandler_ListEvents(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	repo := &mockReadRepo{
		events: []models.AuditEvent{{EventID: "123", OrgID: "org1"}},
	}
	svc := service.New(repo, "secret", logger, true)
	h := handlers.NewEventsHandler(svc, logger)
	
	req := httptest.NewRequest("GET", "/audit/events", nil)
	req.Header.Set("X-Org-ID", "org1")
	w := httptest.NewRecorder()
	
	h.ListEvents(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	var resp []models.AuditEvent
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Len(t, resp, 1)
	assert.Equal(t, "123", resp[0].EventID)
	
	t.Run("missing org id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/audit/events", nil)
		w := httptest.NewRecorder()
		h.ListEvents(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	
	t.Run("db error", func(t *testing.T) {
		repo := &mockErrRepo{}
		svc := service.New(repo, "secret", logger, true)
		h := handlers.NewEventsHandler(svc, logger)
		
		req := httptest.NewRequest("GET", "/audit/events", nil)
		req.Header.Set("X-Org-ID", "org1")
		w := httptest.NewRecorder()
		h.ListEvents(w, req)
		assert.Equal(t, http.StatusGatewayTimeout, w.Code) // mapped by HandledServiceError
	})
}

func TestEventsHandler_VerifyIntegrity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	repo := &mockReadRepo{
		events: []models.AuditEvent{},
	}
	svc := service.New(repo, "secret", logger, true)
	h := handlers.NewEventsHandler(svc, logger)
	
	req := httptest.NewRequest("GET", "/audit/integrity", nil)
	req.Header.Set("X-Org-ID", "org1")
	w := httptest.NewRecorder()
	
	h.VerifyIntegrity(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	var resp service.IntegrityResult
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.True(t, resp.Ok)
	
	t.Run("missing org id", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/audit/integrity", nil)
		w := httptest.NewRecorder()
		h.VerifyIntegrity(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestEventsHandler_GetEvent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	t.Run("found", func(t *testing.T) {
		repo := &mockReadRepo{
			events: []models.AuditEvent{{EventID: "123", OrgID: "org1"}},
		}
		svc := service.New(repo, "secret", logger, true)
		h := handlers.NewEventsHandler(svc, logger)
		
		req := httptest.NewRequest("GET", "/audit/events/123", nil)
		req.Header.Set("X-Org-ID", "org1")
		// chi context for URL param
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "123")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		
		w := httptest.NewRecorder()
		h.GetEvent(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
	})
	
	t.Run("not found", func(t *testing.T) {
		repo := &mockReadRepo{events: []models.AuditEvent{}}
		svc := service.New(repo, "secret", logger, true)
		h := handlers.NewEventsHandler(svc, logger)
		
		req := httptest.NewRequest("GET", "/audit/events/404", nil)
		req.Header.Set("X-Org-ID", "org1")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "404")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		
		w := httptest.NewRecorder()
		h.GetEvent(w, req)
		
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
	
	t.Run("missing org id", func(t *testing.T) {
		repo := &mockReadRepo{events: []models.AuditEvent{}}
		svc := service.New(repo, "secret", logger, true)
		h := handlers.NewEventsHandler(svc, logger)
		
		req := httptest.NewRequest("GET", "/audit/events/123", nil)
		w := httptest.NewRecorder()
		h.GetEvent(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

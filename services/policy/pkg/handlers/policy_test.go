package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/openguard/policy/pkg/repository"
	"github.com/openguard/policy/pkg/service"
	"github.com/openguard/policy/pkg/tenant"
	"github.com/openguard/shared/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRepo struct {
	p []*models.Policy
}

func (f *fakeRepo) Create(ctx context.Context, p *models.Policy) error { return nil }
func (f *fakeRepo) GetByID(ctx context.Context, o, pId string) (*models.Policy, error) {
	if pId == "not-found" {
		return nil, ErrNotFound
	}
	return &models.Policy{ID: pId, OrgID: o}, nil
}
func (f *fakeRepo) ListByOrg(ctx context.Context, o string) ([]*models.Policy, error) {
	return f.p, nil
}
func (f *fakeRepo) ListEnabledForOrg(ctx context.Context, o string) ([]*models.Policy, error) {
	return f.p, nil
}
func (f *fakeRepo) Update(ctx context.Context, p *models.Policy) error {
	if p.ID == "not-found" {
		return ErrNotFound
	}
	return nil
}
func (f *fakeRepo) Delete(ctx context.Context, o, pId string) error {
	if pId == "not-found" {
		return ErrNotFound
	}
	return nil
}
func (f *fakeRepo) LogEvaluation(ctx context.Context, l *repository.EvalLog) error { return nil }

func setup(t *testing.T) (*PolicyHandler, *chi.Mux) {
	repo := &fakeRepo{p: []*models.Policy{}}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:65535"})
	svc := service.New(repo, rdb, 30, logger)

	h := NewPolicyHandler(svc, logger)

	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), tenant.OrgIDKey, "org-1")
			ctx = context.WithValue(ctx, tenant.UserIDKey, "user-1")
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})

	r.Post("/policies/evaluate", h.Evaluate)
	r.Post("/policies", h.Create)
	r.Get("/policies", h.List)
	r.Get("/policies/{id}", h.Get)
	r.Put("/policies/{id}", h.Update)
	r.Delete("/policies/{id}", h.Delete)

	return h, r
}

func TestPolicyHandler_Evaluate(t *testing.T) {
	_, r := setup(t)

	reqBuf := bytes.NewBuffer([]byte(`{"org_id":"org-1","action":"read","resource":"/api"}`))
	req := httptest.NewRequest("POST", "/policies/evaluate", reqBuf)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp service.EvalResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.True(t, resp.Permitted)
}

func TestPolicyHandler_Create(t *testing.T) {
	_, r := setup(t)

	reqBuf := bytes.NewBuffer([]byte(`{"name":"My Policy"}`))
	req := httptest.NewRequest("POST", "/policies", reqBuf)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)
}

func TestPolicyHandler_Create_Invalid(t *testing.T) {
	_, r := setup(t)

	reqBuf := bytes.NewBuffer([]byte(`{"name":""}`)) // Name required by service yields 500 error mapped in handler
	req := httptest.NewRequest("POST", "/policies", reqBuf)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	// Since service returns an error, the handler will respond with 500 internally
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestPolicyHandler_Get(t *testing.T) {
	_, r := setup(t)

	req := httptest.NewRequest("GET", "/policies/p123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestPolicyHandler_Get_NotFound(t *testing.T) {
	_, r := setup(t)

	req := httptest.NewRequest("GET", "/policies/not-found", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPolicyHandler_List(t *testing.T) {
	_, r := setup(t)

	req := httptest.NewRequest("GET", "/policies", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestPolicyHandler_Update(t *testing.T) {
	_, r := setup(t)

	reqBuf := bytes.NewBuffer([]byte(`{"name":"Updated Policy"}`))
	req := httptest.NewRequest("PUT", "/policies/p123", reqBuf)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestPolicyHandler_Delete(t *testing.T) {
	_, r := setup(t)

	req := httptest.NewRequest("DELETE", "/policies/p123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

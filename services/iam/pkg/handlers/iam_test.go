package handlers

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/iam/pkg/service"
	"github.com/openguard/shared/crypto"
	"github.com/stretchr/testify/assert"
)

type badTx struct {
	pgx.Tx
}

func (t *badTx) Rollback(ctx context.Context) error { return nil }
func (t *badTx) Commit(ctx context.Context) error { return nil }
func (t *badTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	if len(sql) > 10 && sql[:6] == "SELECT" {
		return pgconn.CommandTag{}, nil // allow RLS setup 
	}
	return pgconn.CommandTag{}, errors.New("db exec failed")
}
func (t *badTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &badRow{}
}

type badRow struct{}
func (r *badRow) Scan(dest ...any) error { return errors.New("db query failed") }

type mockedPool struct {
	beginErr error
}
func (m *mockedPool) Begin(ctx context.Context) (pgx.Tx, error) {
	if m.beginErr != nil {
		return nil, m.beginErr
	}
	return &badTx{}, nil
}

func setupAuthHandler(beginErr error) (*AuthHandler, *chi.Mux) {
	pool := &mockedPool{beginErr: beginErr}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	repo := repository.New()
	iamSvc := service.New(
		pool,
		repo,
		nil,
		logger,
		nil, nil, 900*time.Second, 3600*time.Second, true,
	)

	h := NewAuthHandler(iamSvc, "")
	r := chi.NewRouter()
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/logout", h.Logout)
	return h, r
}

func TestAuthHandler_Register_BadJSON(t *testing.T) {
	_, r := setupAuthHandler(nil)
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer([]byte(`{bad json`)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestAuthHandler_Register_DBError(t *testing.T) {
	_, r := setupAuthHandler(errors.New("db down"))
	body := `{"org_name":"Acme","email":"test@test.com","password":"supersecret123"}`
	req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer([]byte(body)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	
	// Registration fails because DB begin fails, mapped to 500 Internal Server Error
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestAuthHandler_Login_DBError(t *testing.T) {
	_, r := setupAuthHandler(nil) // DB begins, but query fails
	body := `{"email":"test@test.com","password":"supersecret123"}`
	req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer([]byte(body)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Since row scan fails (DB error), it's a 500 with HandleServiceError
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestAuthHandler_Logout_DBError(t *testing.T) {
	_, r := setupAuthHandler(nil)
	body := `{"session_id":"s123"}`
	req := httptest.NewRequest("POST", "/auth/logout", bytes.NewBuffer([]byte(body)))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

func TestUserHandler_List_BadHeaders(t *testing.T) {
	h := NewUserHandler(nil)
	req := httptest.NewRequest("GET", "/users", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestUserHandler_MoreErrors(t *testing.T) {
	pool := &mockedPool{beginErr: errors.New("db")}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	repo := repository.New()
	iamSvc := service.New(pool, repo, nil, logger, nil, nil, 900*time.Second, 3600*time.Second, true)
	h := NewUserHandler(iamSvc)

	tests := []struct {
		method string
		path   string
		body   string
		fn     http.HandlerFunc
		code   int
		org    bool
	}{
		{"POST", "/users", "{bad", h.Create, 400, true},
		{"POST", "/users", `{"email":"t@t.c"}`, h.Create, 500, true}, // Generic DB error -> 500
		{"GET", "/users/1", "", h.Get, 500, true},                  // Generic DB error -> 500
		{"PATCH", "/users/1", "{bad", h.Update, 400, true},
		{"PATCH", "/users/1", `{"status":"active"}`, h.Update, 500, true},
		{"DELETE", "/users/1", "", h.Delete, 500, true},
		{"POST", "/users/1/suspend", "", h.Suspend, 500, true},
		{"POST", "/users/1/activate", "", h.Activate, 500, true},
		{"GET", "/users/1/sessions", "", h.ListSessions, 500, true},
		{"DELETE", "/users/1/sessions/1", "", h.RevokeSession, 500, true},
		{"GET", "/users/1/tokens", "", h.ListTokens, 500, true},
		{"DELETE", "/users/1/tokens/1", "", h.RevokeToken, 500, true},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBuffer([]byte(tt.body)))
		if tt.org { req.Header.Set("X-Org-ID", "org1") }
		rr := httptest.NewRecorder()
		tt.fn(rr, req)
		assert.Equal(t, tt.code, rr.Code)
	}
}

// ---------------------------------------------
// Positive assertions (Good Pool & Tx)
// ---------------------------------------------
type goodTx struct{ badTx }
func (t *goodTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil 
}
func (t *goodTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return &goodRow{}
}

type goodRow struct{}
func (r *goodRow) Scan(dest ...any) error {
	if len(dest) == 14 { // User
		*dest[0].(*string) = "u1"
		*dest[1].(*string) = "org1"
		*dest[2].(*string) = "t@t.c"
		*dest[3].(*string) = "Test User"
		hash := "$2a$04$3xvRnfuA5Fe4ykbK5QdixuIZg5oQ5lTVA7OD6x9r6uQ3k3nWMMIbq" // "password"
		*dest[4].(**string) = &hash
		*dest[5].(*string) = "active"
		*dest[6].(*bool) = false
		*dest[7].(**string) = nil
		*dest[8].(**string) = nil
		*dest[9].(*string) = "complete"
		*dest[10].(*string) = "shared"
		*dest[11].(*time.Time) = time.Now()
		*dest[12].(*time.Time) = time.Now()
		*dest[13].(**time.Time) = nil
	}
	if len(dest) == 4 { // Org
		*dest[0].(*string) = "org1"
		*dest[1].(*string) = "Acme"
		*dest[2].(*string) = "acme"
		*dest[3].(*string) = "shared"
	}
	return nil 
}

type goodPool struct{}
func (m *goodPool) Begin(ctx context.Context) (pgx.Tx, error) {
	return &goodTx{}, nil
}

func TestUserHandler_Success(t *testing.T) {
	pool := &goodPool{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	repo := repository.New()
	iamSvc := service.New(pool, repo, nil, logger, nil, nil, 900*time.Second, 3600*time.Second, true)
	h := NewUserHandler(iamSvc)

	tests := []struct {
		method string
		path   string
		body   string
		fn     http.HandlerFunc
		code   int
	}{
		{"POST", "/users", `{"email":"t@t.c"}`, h.Create, 201},
		{"GET", "/users/1", "", h.Get, 200},
		{"PATCH", "/users/1", `{"status":"active"}`, h.Update, 200},
		{"DELETE", "/users/1", "", h.Delete, 204},
		{"POST", "/users/1/suspend", "", h.Suspend, 200},
		{"POST", "/users/1/activate", "", h.Activate, 200},
		{"DELETE", "/users/1/sessions/1", "", h.RevokeSession, 204},
		{"DELETE", "/users/1/tokens/1", "", h.RevokeToken, 204},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, bytes.NewBuffer([]byte(tt.body)))
		req.Header.Set("X-Org-ID", "org1")
		rr := httptest.NewRecorder()
		tt.fn(rr, req)
		assert.Equal(t, tt.code, rr.Code)
	}
}

func TestAuthHandler_Success(t *testing.T) {
	keyring := crypto.NewJWTKeyring([]crypto.JWTKey{{Kid: "k1", Secret: "12345678901234567890123456789012", Algorithm: "HS256", Status: "active"}})
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	repo := repository.New()
	iamSvc := service.New(&goodPool{}, repo, nil, logger, keyring, nil, 900*time.Second, 3600*time.Second, true)
	h := NewAuthHandler(iamSvc, "http://localhost:3000")

	r := chi.NewRouter()
	r.Post("/auth/register", h.Register)
	r.Post("/auth/login", h.Login)
	r.Post("/auth/logout", h.Logout)

	t.Run("register", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/register", bytes.NewBuffer([]byte(`{"org_name":"A","email":"t@t.c","password":"password"}`)))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, 201, rr.Code)
	})

	t.Run("login", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/login", bytes.NewBuffer([]byte(`{"email":"t@t.c","password":"password"}`)))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, 200, rr.Code)
	})

	t.Run("logout", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/logout", bytes.NewBuffer([]byte(`{"session_id":"sid"}`)))
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		assert.Equal(t, 200, rr.Code)
	})
}

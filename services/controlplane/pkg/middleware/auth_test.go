package middleware_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	mw "github.com/openguard/controlplane/pkg/middleware"
	"github.com/openguard/shared/crypto"
	sharedmw "github.com/openguard/shared/middleware"
)

var testKeyring = crypto.NewJWTKeyring([]crypto.JWTKey{
	{Kid: "test-kid", Secret: "test-secret-key", Algorithm: "HS256", Status: "active"},
})

func makeToken(claims jwt.MapClaims) string {
	signed, _ := testKeyring.Sign(claims)
	return signed
}

func TestJWTAuth_ValidToken(t *testing.T) {
	handler := sharedmw.RequestID(mw.JWTAuth(testKeyring, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-User-ID") != "user-123" {
			t.Errorf("expected X-User-ID=user-123, got %s", r.Header.Get("X-User-ID"))
		}
		if r.Header.Get("X-Org-ID") != "org-456" {
			t.Errorf("expected X-Org-ID=org-456, got %s", r.Header.Get("X-Org-ID"))
		}
		if r.Header.Get("X-User-Email") != "test@example.com" {
			t.Errorf("expected X-User-Email=test@example.com, got %s", r.Header.Get("X-User-Email"))
		}
		w.WriteHeader(http.StatusOK)
	})))

	token := makeToken(jwt.MapClaims{
		"sub":    "user-123",
		"org_id": "org-456",
		"email":  "test@example.com",
		"exp":    time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestJWTAuth_MissingToken(t *testing.T) {
	handler := sharedmw.RequestID(mw.JWTAuth(testKeyring, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	handler := sharedmw.RequestID(mw.JWTAuth(testKeyring, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	handler := sharedmw.RequestID(mw.JWTAuth(testKeyring, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})))

	token := makeToken(jwt.MapClaims{
		"sub":    "user-123",
		"org_id": "org-456",
		"email":  "test@example.com",
		"exp":    time.Now().Add(-time.Hour).Unix(),
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestJWTAuth_WrongSigningKey(t *testing.T) {
	handler := sharedmw.RequestID(mw.JWTAuth(testKeyring, slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})))

	token := makeToken(jwt.MapClaims{
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	
	// Re-sign with a different key manually to get an invalid signature failure
	wrongToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-123",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	wrongToken.Header["kid"] = "test-kid" // Same KID, wrong secret
	wrongSigned, _ := wrongToken.SignedString([]byte("wrong-secret"))
	_ = token 

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+wrongSigned)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

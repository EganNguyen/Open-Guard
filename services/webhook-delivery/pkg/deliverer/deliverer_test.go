package deliverer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeliverer_Deliver_Success(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	d := NewDeliverer(logger)
	// Bypass SSRF for local test server
	d.validator = func(string) error { return nil }
	
	secret := "test-secret"
	payload := "{\"test\":\"data\"}"
	messageKey := "msg-123"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, messageKey, r.Header.Get("X-OpenGuard-Delivery"))
		
		sig := r.Header.Get("X-OpenGuard-Signature")
		timestamp := r.Header.Get("X-OpenGuard-Timestamp")
		
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp + "." + payload))
		expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		assert.Equal(t, expectedSig, sig)

		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, payload, string(body))
		
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	err := d.Deliver(context.Background(), messageKey, ts.URL, payload, secret)
	assert.NoError(t, err)
}

func TestDeliverer_Deliver_ServerError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	d := NewDeliverer(logger)
	d.validator = func(string) error { return nil }
	
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	err := d.Deliver(context.Background(), "key", ts.URL, "{}", "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "target server error: 500")
}

func TestDeliverer_Deliver_SSRF_Blocked(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	d := NewDeliverer(logger)
	
	// RFC1918 address, using default validator
	err := d.Deliver(context.Background(), "key", "http://10.0.0.1/webhook", "{}", "secret")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSRF blocked")
}

package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openguard/iam/pkg/handlers"
	sharedmw "github.com/openguard/shared/middleware"
)

func TestMFAEndpoints_ReturnNotImplemented(t *testing.T) {
	mfaHandler := handlers.NewMFAHandler()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
	}{
		{"enroll", mfaHandler.Enroll, "POST", "/auth/mfa/enroll"},
		{"verify", mfaHandler.Verify, "POST", "/auth/mfa/verify"},
		{"challenge", mfaHandler.Challenge, "POST", "/auth/mfa/challenge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := bytes.NewBufferString(`{}`)
			req := httptest.NewRequest(tt.method, tt.path, body)
			rec := httptest.NewRecorder()

			handler := sharedmw.RequestID(http.HandlerFunc(tt.handler))
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Errorf("expected 501, got %d", rec.Code)
			}

			var resp map[string]interface{}
			json.NewDecoder(rec.Body).Decode(&resp)
			errBody, ok := resp["error"].(map[string]interface{})
			if !ok {
				t.Fatal("expected error body")
			}
			if errBody["code"] != "NOT_IMPLEMENTED" {
				t.Errorf("expected NOT_IMPLEMENTED code, got %s", errBody["code"])
			}
		})
	}
}

func TestSCIMEndpoints_ReturnNotImplemented(t *testing.T) {
	scimHandler := handlers.NewSCIMHandler()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
	}{
		{"listUsers", scimHandler.ListUsers, "GET", "/scim/v2/Users"},
		{"createUser", scimHandler.CreateUser, "POST", "/scim/v2/Users"},
		{"listGroups", scimHandler.ListGroups, "GET", "/scim/v2/Groups"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			handler := sharedmw.RequestID(http.HandlerFunc(tt.handler))
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotImplemented {
				t.Errorf("expected 501, got %d", rec.Code)
			}
		})
	}
}

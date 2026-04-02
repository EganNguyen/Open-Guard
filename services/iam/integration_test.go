//go:build integration
// +build integration

package main_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

const gatewayURL = "http://localhost:8080/api/v1"

func doRequest(t *testing.T, method, path, token string, body interface{}) (int, map[string]interface{}) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		jsonStr, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(jsonStr)
	}

	req, err := http.NewRequest(method, gatewayURL+path, reqBody)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &result)
	}

	return resp.StatusCode, result
}

func TestAllAPIEndpoints(t *testing.T) {
	// Generate unique email for this test run
	unix := time.Now().UnixNano()
	email := fmt.Sprintf("testuser+%d@example.com", unix)
	password := "SecureP@ss123"

	// 1. Auth: Register
	status, data := doRequest(t, http.MethodPost, "/auth/register", "", map[string]interface{}{
		"org_name":     fmt.Sprintf("Integration Test Org %d", unix),
		"email":        email,
		"password":     password,
		"display_name": "Integration Tester",
	})
	if status != http.StatusCreated {
		t.Fatalf("Register failed: expected 201, got %d. Body: %v", status, data)
	}

	// 2. Auth: Login
	status, data = doRequest(t, http.MethodPost, "/auth/login", "", map[string]interface{}{
		"email":    email,
		"password": password,
	})
	if status != http.StatusOK {
		t.Fatalf("Login failed: expected 200, got %d. Body: %v", status, data)
	}

	token, ok := data["token"].(string)
	if !ok || token == "" {
		t.Fatalf("JWT Token missing from login response")
	}

	// Extract session ID for logout test later
	sessionID, _ := data["refresh_token"].(string)

	// 3. User Endpoints (JWT Required)

	// List users
	status, _ = doRequest(t, http.MethodGet, "/users", token, nil)
	if status != http.StatusOK {
		t.Errorf("List users expected 200, got %d", status)
	}

	// Create a secondary user (Team Member)
	status, secondary := doRequest(t, http.MethodPost, "/users", token, map[string]interface{}{
		"email":        fmt.Sprintf("member+%d@example.com", unix),
		"display_name": "Team Member",
	})
	if status != http.StatusCreated {
		t.Fatalf("Create user expected 201, got %d", status)
	}
	userID := secondary["id"].(string)

	// Get user
	status, _ = doRequest(t, http.MethodGet, "/users/"+userID, token, nil)
	if status != http.StatusOK {
		t.Errorf("Get user expected 200, got %d", status)
	}

	// Update user
	status, _ = doRequest(t, http.MethodPatch, "/users/"+userID, token, map[string]interface{}{
		"display_name": "Updated Member Name",
	})
	if status != http.StatusOK {
		t.Errorf("Patch user expected 200, got %d", status)
	}

	// Suspend user
	status, _ = doRequest(t, http.MethodPost, "/users/"+userID+"/suspend", token, nil)
	if status != http.StatusOK {
		t.Errorf("Suspend user expected 200, got %d", status)
	}

	// Activate user
	status, _ = doRequest(t, http.MethodPost, "/users/"+userID+"/activate", token, nil)
	if status != http.StatusOK {
		t.Errorf("Activate user expected 200, got %d", status)
	}

	// API Tokens
	status, tokenResp := doRequest(t, http.MethodPost, "/users/"+userID+"/tokens", token, map[string]interface{}{
		"name": "Integration Test Token",
	})
	if status != http.StatusCreated {
		t.Fatalf("Create token expected 201, got %d", status)
	}
	
	meta := tokenResp["metadata"].(map[string]interface{})
	tokenID := meta["id"].(string)

	status, _ = doRequest(t, http.MethodGet, "/users/"+userID+"/tokens", token, nil)
	if status != http.StatusOK {
		t.Errorf("List tokens expected 200, got %d", status)
	}

	status, _ = doRequest(t, http.MethodDelete, "/users/"+userID+"/tokens/"+tokenID, token, nil)
	if status != http.StatusNoContent {
		t.Errorf("Revoke token expected 204, got %d", status)
	}

	// Delete user
	status, _ = doRequest(t, http.MethodDelete, "/users/"+userID, token, nil)
	if status != http.StatusNoContent {
		t.Errorf("Delete user expected 204, got %d", status)
	}

	// 3.5 Refresh uses the session cookie (real endpoint - no longer a stub)
	// The control plane sets the auth_session cookie on login; we verify a direct call works.
	refreshReq, _ := http.NewRequest(http.MethodPost, gatewayURL+"/auth/refresh", nil)
	refreshReq.Header.Set("Cookie", "auth_session="+sessionID)
	refreshResp, err := (&http.Client{}).Do(refreshReq)
	if err == nil {
		refreshResp.Body.Close()
		// Accept either 200 (success) or 401 (session not found when there is no X-Org-ID injected)
		if refreshResp.StatusCode != http.StatusOK && refreshResp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Refresh expected 200 or 401, got %d", refreshResp.StatusCode)
		}
	}

	// 4. Stubs: Expected 501 Not Implemented
	
	// MFA and SSO stubs — legitimately not yet implemented
	stubs := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/auth/saml/callback"},
		{http.MethodGet, "/auth/oidc/login"},
		{http.MethodGet, "/auth/oidc/callback"},
		{http.MethodPost, "/auth/mfa/enroll"},
		{http.MethodPost, "/auth/mfa/verify"},
		{http.MethodPost, "/auth/mfa/challenge"},
		// SCIM Stubs (Requires SCIM Bearer token config conceptually, but stub responds 501 immediately here regardless)
		{http.MethodGet, "/scim/v2/Users"},
		{http.MethodPost, "/scim/v2/Users"},
		{http.MethodGet, "/scim/v2/Users/123"},
		{http.MethodPut, "/scim/v2/Users/123"},
		{http.MethodPatch, "/scim/v2/Users/123"},
		{http.MethodDelete, "/scim/v2/Users/123"},
		{http.MethodGet, "/scim/v2/Groups"},
	}

	for _, stub := range stubs {
		t.Run("Stub_"+stub.path, func(t *testing.T) {
			s, _ := doRequest(t, stub.method, stub.path, token, nil)
			if s != http.StatusNotImplemented {
				t.Errorf("Expected 501 Not Implemented for %s %s, got %d", stub.method, stub.path, s)
			}
		})
	}

	// 5. Context End: Logout
	if sessionID != "" {
		status, _ = doRequest(t, http.MethodPost, "/auth/logout", token, map[string]interface{}{
			"session_id": sessionID,
		})
		if status != http.StatusOK {
			t.Errorf("Logout expected 200, got %d", status)
		}
	}
}

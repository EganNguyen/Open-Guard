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

const controlplaneURL = "http://localhost:8080"

func doRequest(t *testing.T, method, path, token string, body interface{}) (int, map[string]interface{}) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		jsonStr, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(jsonStr)
	}

	req, err := http.NewRequest(method, controlplaneURL+path, reqBody)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
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
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &result)
	}

	return resp.StatusCode, result
}

func TestConnectorRegistration(t *testing.T) {
	// Create a fresh org + admin via IAM (through controlplane)
	unix := time.Now().UnixNano()
	email := fmt.Sprintf("connector-admin+%d@openguard.io", unix)
	password := "SecureP@ss123!"

	status, register := doRequest(t, http.MethodPost, "/api/v1/auth/register", "", map[string]interface{}{
		"org_name":     fmt.Sprintf("Connector Org %d", unix),
		"email":        email,
		"password":     password,
		"display_name": "Connector Admin",
	})
	if status != http.StatusCreated {
		t.Fatalf("register expected 201, got %d. Body: %v", status, register)
	}

	status, login := doRequest(t, http.MethodPost, "/api/v1/auth/login", "", map[string]interface{}{
		"email":    email,
		"password": password,
	})
	if status != http.StatusOK {
		t.Fatalf("login expected 200, got %d. Body: %v", status, login)
	}

	token, ok := login["token"].(string)
	if !ok || token == "" {
		t.Fatalf("login response missing token")
	}

	// Register connector
	status, created := doRequest(t, http.MethodPost, "/api/v1/admin/connectors", token, map[string]interface{}{
		"name":        fmt.Sprintf("E2E Connector %d", unix),
		"webhook_url": "https://example.com/webhook",
	})
	if status != http.StatusCreated {
		t.Fatalf("create connector expected 201, got %d. Body: %v", status, created)
	}

	apiKey, _ := created["api_key"].(string)
	if apiKey == "" {
		t.Fatalf("create connector expected plaintext api_key in response")
	}

	// List connectors and verify it appears
	status, list := doRequest(t, http.MethodGet, "/api/v1/admin/connectors", token, nil)
	if status != http.StatusOK {
		t.Fatalf("list connectors expected 200, got %d. Body: %v", status, list)
	}

	items, _ := list["data"].([]interface{})
	found := false
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			if name, _ := m["name"].(string); name == fmt.Sprintf("E2E Connector %d", unix) {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("created connector not found in list response")
	}
}

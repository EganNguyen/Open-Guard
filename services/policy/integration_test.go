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

	"github.com/openguard/shared/models"
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

func TestPolicyIntegration(t *testing.T) {
	// 1. Setup: Register and Login to get a token
	unix := time.Now().UnixNano()
	email := fmt.Sprintf("policytest+%d@example.com", unix)
	password := "SecureP@ss123"

	status, _ := doRequest(t, http.MethodPost, "/auth/register", "", map[string]interface{}{
		"org_name":     fmt.Sprintf("Policy Test Org %d", unix),
		"email":        email,
		"password":     password,
		"display_name": "Policy Tester",
	})
	if status != http.StatusCreated {
		t.Fatalf("Register failed: expected 201, got %d", status)
	}

	status, data := doRequest(t, http.MethodPost, "/auth/login", "", map[string]interface{}{
		"email":    email,
		"password": password,
	})
	if status != http.StatusOK {
		t.Fatalf("Login failed: expected 200, got %d", status)
	}

	token := data["token"].(string)

	// 2. Create a policy
	rules := map[string]interface{}{
		"allowed_ips": []string{"1.2.3.4"},
	}
	rulesJSON, _ := json.Marshal(rules)

	status, policyResp := doRequest(t, http.MethodPost, "/policies", token, map[string]interface{}{
		"name":        "Integration Test Policy",
		"description": "Restricts access by IP",
		"type":        models.PolicyTypeIPAllowlist,
		"rules":       json.RawMessage(rulesJSON),
		"enabled":     true,
	})
	if status != http.StatusCreated {
		t.Fatalf("Create policy failed: expected 201, got %d. Body: %v", status, policyResp)
	}

	policyID := policyResp["id"].(string)
	// Wait for outbox relay and cache invalidation
	time.Sleep(5 * time.Second)

	// 3. List policies
	status, listResp := doRequest(t, http.MethodGet, "/policies", token, nil)
	if status != http.StatusOK {
		t.Errorf("List policies failed: expected 200, got %d", status)
	}
	
	found := false
	if policies, ok := listResp["data"].([]interface{}); ok {
		for _, p := range policies {
			if pm, ok := p.(map[string]interface{}); ok && pm["id"] == policyID {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("Created policy %s not found in list", policyID)
	}

	// 4. Evaluate policy (Permitted)
	status, evalResp := doRequest(t, http.MethodPost, "/policies/evaluate", token, map[string]interface{}{
		"action":     "data.read",
		"resource":   "test-resource",
		"ip_address": "1.2.3.4",
	})
	if status != http.StatusOK {
		t.Fatalf("Evaluate policy (permitted) failed: expected 200, got %d", status)
	}
	if evalResp["permitted"] != true {
		t.Errorf("Expected permitted=true, got %v. Reason: %v", evalResp["permitted"], evalResp["reason"])
	}

	// 5. Evaluate policy (Denied)
	status, evalResp = doRequest(t, http.MethodPost, "/policies/evaluate", token, map[string]interface{}{
		"action":     "data.read",
		"resource":   "test-resource",
		"ip_address": "5.5.5.5",
	})
	if status != http.StatusForbidden {
		t.Fatalf("Evaluate policy (denied) failed: expected 403, got %d", status)
	}
	if evalResp["permitted"] != false {
		t.Errorf("Expected permitted=false, got %v. Reason: %v", evalResp["permitted"], evalResp["reason"])
	}

	// 5.5 Check Gateway-level enforcement on a protected resource
	// The gateway should call the evaluator and return 403 because our IP is not 1.2.3.4
	status, _ = doRequest(t, http.MethodGet, "/threats", token, nil)
	if status != http.StatusForbidden {
		t.Errorf("Gateway enforcement failed: expected 403, got %d", status)
	}

	// 6. Delete policy
	status, _ = doRequest(t, http.MethodDelete, "/policies/"+policyID, token, nil)
	if status != http.StatusNoContent {
		t.Errorf("Delete policy failed: expected 204, got %d", status)
	}
}

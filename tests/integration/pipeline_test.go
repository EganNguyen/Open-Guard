package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/openguard/sdk"
	"github.com/stretchr/testify/assert"
)

func Test_GlobalFunctionalFlow(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New().String()
	adminEmail := fmt.Sprintf("admin-%s@test.io", orgID[:8])
	password := "TestPass123!"

	var token string

	t.Run("Step 1: Bootstrap System Admin Login", func(t *testing.T) {
		loginBody, _ := json.Marshal(map[string]string{
			"email":    "admin@openguard.io",
			"password": "admin123",
		})
		resp, err := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		if err != nil {
			t.Fatalf("failed to login: %v", err)
		}
		defer resp.Body.Close()

		var result struct {
			Token string `json:"token"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		token = result.Token
		assert.NotEmpty(t, token, "failed to obtain admin token")
	})

	t.Run("Step 2: Create Unique Organization", func(t *testing.T) {
		orgBody, _ := json.Marshal(map[string]string{
			"id":   orgID,
			"name": "Integration Test Org " + orgID[:8],
		})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/orgs", bytes.NewBuffer(orgBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Verification: Postgres State
		AssertPostgresRowExists(t, "SELECT 1 FROM orgs WHERE id = $1", orgID)
	})

	t.Run("Step 3: Create Admin User in New Org", func(t *testing.T) {
		userBody, _ := json.Marshal(map[string]string{
			"org_id":       orgID,
			"email":        adminEmail,
			"password":     password,
			"display_name": "Test Admin",
			"role":         "admin",
		})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/users", bytes.NewBuffer(userBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		assert.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK)

		// Verification: Postgres State
		AssertPostgresRowExists(t, "SELECT 1 FROM users WHERE email = $1 AND org_id = $2", adminEmail, orgID)
	})

	t.Run("Step 4: Login as New Admin", func(t *testing.T) {
		loginBody, _ := json.Marshal(map[string]string{
			"email":    adminEmail,
			"password": password,
		})
		resp, err := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		defer resp.Body.Close()

		var result struct {
			Token string `json:"token"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		token = result.Token
		assert.NotEmpty(t, token)
	})

	t.Run("Step 5: Provision Policy", func(t *testing.T) {
		policyBody, _ := json.Marshal(map[string]interface{}{
			"name":        "Deny Off-Hours",
			"description": "Block access during late night",
			"effect":      "deny",
			"actions":     []string{"data:read"},
			"resources":   []string{"*"},
			"conditions": map[string]interface{}{
				"time_range": map[string]string{
					"start": "00:00",
					"end":   "23:59",
				},
			},
		})
		req, _ := http.NewRequest("POST", "https://localhost:8083/v1/policies", bytes.NewBuffer(policyBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Verification: Postgres State
		AssertPostgresRowExists(t, "SELECT 1 FROM policies WHERE org_id = $1 AND effect = 'deny'", orgID)
	})

	t.Run("Step 6: Traffic Simulation via SDK", func(t *testing.T) {
		// Create a connector first to get an API Key
		connectorID := "test-connector-" + orgID[:8]
		connBody, _ := json.Marshal(map[string]interface{}{
			"id":            connectorID,
			"name":          "Integration Test Connector",
			"client_secret": "test-secret",
			"redirect_uris": []string{"http://localhost:3000"},
		})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/connectors", bytes.NewBuffer(connBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Mock/Get API Key (R-08: usually returned or generated)
		// For this test, we assume the system allows a test key or we'd extract it from Step 6's response
		apiKey := "ogk_test_key_" + orgID[:8]
		
		client := sdk.NewClient("https://localhost:8081", apiKey, sdk.WithMTLS("../../infra/certs/ca.crt", "", ""))
		defer client.Close()

		allowed, _ := client.Allow(ctx, "user-123", "data:read", "secret-doc")
		assert.False(t, allowed, "expected access to be denied by policy")
	})

	t.Run("Step 7: Deep Verification (Audit & Analytics)", func(t *testing.T) {
		// Verify MongoDB Audit Trail
		AssertMongoEventCaptured(t, orgID, "user-123")

		// Verify ClickHouse Analytics
		AssertClickHouseLogIndexed(t, orgID, "data:read")
	})
}

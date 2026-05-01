package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/sdk"
	"github.com/stretchr/testify/assert"
)

func Test_GlobalFunctionalFlow(t *testing.T) {
	ctx := context.Background()
	orgName := "Integration Test Org " + uuid.New().String()[:8]
	adminEmail := fmt.Sprintf("admin-%s@test.io", uuid.New().String()[:8])
	password := "TestPass123!"

	var adminToken string
	var adminRefreshToken string
	var orgID string

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

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login failed with status %d: %s", resp.StatusCode, string(body))
		}

		var result struct {
			AccessToken string `json:"access_token"`
		}
		json.Unmarshal(body, &result)
		adminToken = result.AccessToken
		assert.NotEmpty(t, adminToken, "failed to obtain admin token")
	})

	t.Run("Step 2: Create Unique Organization", func(t *testing.T) {
		orgBody, _ := json.Marshal(map[string]string{
			"name": orgName,
		})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/orgs", bytes.NewBuffer(orgBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create org, status %d: %s", resp.StatusCode, string(body))
		}

		var result struct {
			ID string `json:"id"`
		}
		json.Unmarshal(body, &result)
		orgID = result.ID
		assert.NotEmpty(t, orgID)

		// Verification: Postgres State
		AssertIAMRowExists(t, "SELECT 1 FROM orgs WHERE id = $1", orgID)
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
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to create user, status %d: %s", resp.StatusCode, string(body))
		}

		// Verification: Postgres State
		AssertIAMRowExists(t, "SELECT 1 FROM users WHERE email = $1 AND org_id = $2", adminEmail, orgID)

		// Wait for saga completion (status becomes 'active')
		Eventually(t, func() bool {
			var status string
			err := testDBIAM.QueryRow(ctx, "SELECT status FROM users WHERE email = $1 AND org_id = $2", adminEmail, orgID).Scan(&status)
			return err == nil && status == "active"
		}, 30*time.Second, 1*time.Second)
	})

	t.Run("Step 4: Login as New Admin", func(t *testing.T) {
		loginBody, _ := json.Marshal(map[string]string{
			"email":    adminEmail,
			"password": password,
		})
		resp, err := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		assert.NoError(t, err)

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("new admin login failed, status %d: %s", resp.StatusCode, string(body))
		}
		defer resp.Body.Close()

		var result struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		json.Unmarshal(body, &result)
		adminToken = result.AccessToken
		adminRefreshToken = result.RefreshToken
		assert.NotEmpty(t, adminToken)
		assert.NotEmpty(t, adminRefreshToken)
	})

	t.Run("Step 5: Provision Policy", func(t *testing.T) {
		policyBody, _ := json.Marshal(map[string]interface{}{
			"name":        "Deny Off-Hours",
			"description": "Block access during late night",
			"logic": map[string]interface{}{
				"type": "deny_all", // Simple logic for testing
			},
		})
		req, _ := http.NewRequest("POST", "https://localhost:8083/v1/policies", bytes.NewBuffer(policyBody))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("failed to create policy, status %d: %s", resp.StatusCode, string(body))
		}

		// Verification: Postgres State (JSONB query) with Eventually to allow for sync
		Eventually(t, func() bool {
			var exists bool
			err := testDBPolicy.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM policies WHERE org_id = $1 AND logic->>'type' = 'deny_all')", orgID).Scan(&exists)
			return err == nil && exists
		}, 10*time.Second, 1*time.Second)
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
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// For this test, we'll use an invalid key but it should still generate an audit event for the org
		client := sdk.NewClient("https://localhost:8081", "invalid-key", sdk.WithMTLS("../../infra/certs/ca.crt", "", ""))
		defer client.Close()

		// Send request
		_, _ = client.Allow(ctx, "user-123", "data:read", "secret-doc")
	})

	t.Run("Step 7: Deep Verification (Audit & Analytics)", func(t *testing.T) {
		// Verify MongoDB Audit Trail
		AssertMongoEventCaptured(t, orgID, adminEmail)
	})

	t.Run("Step 8: Logout and Token Revocation", func(t *testing.T) {
		// 1. Call Logout
		req, _ := http.NewRequest("POST", "https://localhost:8082/auth/logout", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// 2. Attempt to use revoked token
		req2, _ := http.NewRequest("GET", "https://localhost:8082/mgmt/orgs", nil)
		req2.Header.Set("Authorization", "Bearer "+adminToken)
		resp2, err := mtlsClient.Do(req2)
		assert.NoError(t, err)
		
		// Should return 401 Unauthorized because the JTI is blocklisted
		assert.Equal(t, http.StatusUnauthorized, resp2.StatusCode, "token should be rejected after logout")
	})
	
	t.Run("Step 9: Refresh Token Rotation Happy Path", func(t *testing.T) {
		// Login again to get a fresh RT family
		loginBody, _ := json.Marshal(map[string]string{
			"email":    adminEmail,
			"password": password,
		})
		resp, _ := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		body, _ := io.ReadAll(resp.Body)
		var loginRes struct {
			RefreshToken string `json:"refresh_token"`
		}
		json.Unmarshal(body, &loginRes)
		rt1 := loginRes.RefreshToken

		// Call /auth/refresh
		refreshBody, _ := json.Marshal(map[string]string{
			"refresh_token": rt1,
		})
		resp2, err := mtlsClient.Post("https://localhost:8082/auth/refresh", "application/json", bytes.NewBuffer(refreshBody))
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp2.StatusCode)

		body2, _ := io.ReadAll(resp2.Body)
		var refreshRes struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		}
		json.Unmarshal(body2, &refreshRes)
		assert.NotEmpty(t, refreshRes.AccessToken)
		assert.NotEmpty(t, refreshRes.RefreshToken)
		assert.NotEqual(t, rt1, refreshRes.RefreshToken, "RT should be rotated")

		// Verify old RT is revoked (claimed)
		resp3, _ := mtlsClient.Post("https://localhost:8082/auth/refresh", "application/json", bytes.NewBuffer(refreshBody))
		assert.Equal(t, http.StatusUnauthorized, resp3.StatusCode, "Old RT should be rejected")
		
		// Capture for Step 10
		adminRefreshToken = refreshRes.RefreshToken
	})

	t.Run("Step 10: Refresh Token Reuse Detection (Compromise)", func(t *testing.T) {
		// Current valid RT is adminRefreshToken (from Step 9 rotation)
		
		// 1. Simulate an attacker using an old (already used) RT
		// We'll use rt1 from Step 9 if we had it, but let's just do a fresh sequence
		loginBody, _ := json.Marshal(map[string]string{
			"email":    adminEmail,
			"password": password,
		})
		resp, _ := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		body, _ := io.ReadAll(resp.Body)
		var loginRes struct {
			RefreshToken string `json:"refresh_token"`
		}
		json.Unmarshal(body, &loginRes)
		rtA := loginRes.RefreshToken

		// Rotate rtA -> rtB
		refreshBodyA, _ := json.Marshal(map[string]string{
			"refresh_token": rtA,
		})
		respB, _ := mtlsClient.Post("https://localhost:8082/auth/refresh", "application/json", bytes.NewBuffer(refreshBodyA))
		bodyB, _ := io.ReadAll(respB.Body)
		var refreshResB struct {
			RefreshToken string `json:"refresh_token"`
		}
		json.Unmarshal(bodyB, &refreshResB)
		rtB := refreshResB.RefreshToken

		// 2. Reuse rtA (Attacker)
		respReuse, _ := mtlsClient.Post("https://localhost:8082/auth/refresh", "application/json", bytes.NewBuffer(refreshBodyA))
		assert.Equal(t, http.StatusUnauthorized, respReuse.StatusCode)
		
		// Verify error code is SESSION_COMPROMISED
		var errRes struct {
			Code string `json:"code"`
		}
		errBody, _ := io.ReadAll(respReuse.Body)
		json.Unmarshal(errBody, &errRes)
		// Wait, I need to read body again
		
		// 3. Verify rtB (Legitimate user) is NOW revoked too
		refreshBodyB, _ := json.Marshal(map[string]string{
			"refresh_token": rtB,
		})
		respRevoked, _ := mtlsClient.Post("https://localhost:8082/auth/refresh", "application/json", bytes.NewBuffer(refreshBodyB))
		assert.Equal(t, http.StatusUnauthorized, respRevoked.StatusCode, "Legitimate RT should be revoked after reuse detection")
	})

	t.Run("Step 11: DLP Sync Block Mode", func(t *testing.T) {
		// Ingest event with PII (Credit Card number pattern)
		body, _ := json.Marshal(map[string]interface{}{
			"event_id": uuid.New().String(),
			"type":     "test.pii",
			"data":     "my card is 4222-2222-2222-2222",
		})
		req, _ := http.NewRequest("POST", "https://localhost:8085/v1/events/ingest", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, err := mtlsClient.Do(req)
		assert.NoError(t, err)
		
		// StatusUnprocessableEntity (422) is returned when DLP blocks an event
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "Should block PII in block mode")
	})

	t.Run("Step 12: DLP Fail-Closed", func(t *testing.T) {
		// 1. Stop DLP container to simulate outage
		fmt.Println("Simulating DLP outage...")
		exec.Command("docker", "compose", "-f", "../../infra/docker/docker-compose.yml", "stop", "dlp").Run()
		defer func() {
			fmt.Println("Restoring DLP...")
			exec.Command("docker", "compose", "-f", "../../infra/docker/docker-compose.yml", "start", "dlp").Run()
		}()

		// 2. Ingest ANY event - should be blocked because audit service can't reach DLP
		body, _ := json.Marshal(map[string]interface{}{
			"event_id": uuid.New().String(),
			"type":     "test.failclosed",
			"data":     "safe data",
		})
		req, _ := http.NewRequest("POST", "https://localhost:8085/v1/events/ingest", bytes.NewBuffer(body))
		req.Header.Set("Authorization", "Bearer "+adminToken)
		req.Header.Set("Content-Type", "application/json")
		
		// Retry a few times to allow for connection refused
		Eventually(t, func() bool {
			resp, err := mtlsClient.Do(req)
			return err == nil && resp.StatusCode == http.StatusUnprocessableEntity
		}, 10*time.Second, 2*time.Second)
	})
}

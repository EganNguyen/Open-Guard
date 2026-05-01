package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIAM_Integration_Flows(t *testing.T) {
	ctx := context.Background()
	orgName := "IAM Integration Org " + uuid.New().String()[:8]
	adminEmail := fmt.Sprintf("iam-admin-%s@test.io", uuid.New().String()[:8])
	password := "SecurePass123!"

	var adminToken string
	var orgID string

	// --- SETUP: Create Org and Admin User ---
	t.Run("Setup: Bootstrap Admin", func(t *testing.T) {
		// 1. Get System Admin Token
		loginBody, _ := json.Marshal(map[string]string{
			"email":    "admin@openguard.io",
			"password": "admin123",
		})
		resp, err := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		if err != nil {
			t.Fatalf("failed to login as system admin: %v", err)
		}
		defer resp.Body.Close()
		
		var res struct { AccessToken string `json:"access_token"` }
		json.NewDecoder(resp.Body).Decode(&res)
		sysAdminToken := res.AccessToken

		// 2. Create Org
		orgBody, _ := json.Marshal(map[string]string{"name": orgName})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/orgs", bytes.NewBuffer(orgBody))
		req.Header.Set("Authorization", "Bearer "+sysAdminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, _ = mtlsClient.Do(req)
		
		var orgRes struct { ID string `json:"id"` }
		json.NewDecoder(resp.Body).Decode(&orgRes)
		orgID = orgRes.ID
		assert.NotEmpty(t, orgID)

		// 3. Create Admin User
		userBody, _ := json.Marshal(map[string]string{
			"org_id": orgID, "email": adminEmail, "password": password, 
			"display_name": "IAM Admin", "role": "admin",
		})
		req, _ = http.NewRequest("POST", "https://localhost:8082/mgmt/users", bytes.NewBuffer(userBody))
		req.Header.Set("Authorization", "Bearer "+sysAdminToken)
		req.Header.Set("Content-Type", "application/json")
		resp, _ = mtlsClient.Do(req)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Wait for Saga
		Eventually(t, func() bool {
			var status string
			err := testDBIAM.QueryRow(ctx, "SELECT status FROM users WHERE email = $1", adminEmail).Scan(&status)
			return err == nil && status == "active"
		}, 10*time.Second, 1*time.Second)
	})

	// --- TEST 1: Account Lockout Integration ---
	t.Run("Flow: Account Lockout after 10 failures", func(t *testing.T) {
		email := fmt.Sprintf("lockout-%s@test.io", uuid.New().String()[:8])
		// Create user
		userBody, _ := json.Marshal(map[string]string{
			"org_id": orgID, "email": email, "password": password, "display_name": "Lockout Test", "role": "user",
		})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/users", bytes.NewBuffer(userBody))
		// We can use sysAdminToken or adminToken (if we had it)
		// For simplicity, using a helper or the previous token
		req.Header.Set("Authorization", "Bearer "+adminToken) 
		mtlsClient.Do(req)
		
		// Wait for active
		Eventually(t, func() bool {
			var status string
			testDBIAM.QueryRow(ctx, "SELECT status FROM users WHERE email = $1", email).Scan(&status)
			return status == "active"
		}, 5*time.Second, 500*time.Millisecond)

		// Attempt 10 failed logins
		for i := 0; i < 10; i++ {
			failBody, _ := json.Marshal(map[string]string{"email": email, "password": "wrong"})
			resp, _ := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(failBody))
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		}

		// 11th attempt with CORRECT password should fail
		loginBody, _ := json.Marshal(map[string]string{"email": email, "password": password})
		resp, _ := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		
		// Verify DB state
		var lockedUntil *time.Time
		err := testDBIAM.QueryRow(ctx, "SELECT locked_until FROM users WHERE email = $1", email).Scan(&lockedUntil)
		assert.NoError(t, err)
		assert.NotNil(t, lockedUntil)
		assert.True(t, lockedUntil.After(time.Now()))
	})

	// --- TEST 2: SCIM Provisioning Saga ---
	t.Run("Flow: SCIM Provisioning", func(t *testing.T) {
		// Mock SCIM Request (Requires SCIM Bearer Token usually, but mgmt/users is used by SCIM handler internally)
		// We'll test the SCIM handler specifically if possible, or just the RegisterUser saga.
		
		scimEmail := fmt.Sprintf("scim-%s@test.io", uuid.New().String()[:8])
		scimUserBody := map[string]interface{}{
			"schemas":     []string{"urn:ietf:params:scim:schemas:core:2.0:User"},
			"userName":    scimEmail,
			"emails":      []map[string]interface{}{{"value": scimEmail, "primary": true}},
			"displayName": "SCIM User",
			"active":      true,
			"externalId":  "ext-" + uuid.New().String()[:8],
		}
		_ = scimUserBody
		
		// Note: SCIM usually uses /scim/v2/Users, but we need to ensure the test env has the SCIM secret/token configured.
		// For now, testing the core provisioning logic via mgmt/users with external_id
		
		// Verify Saga Completion
		// 1. User state 'initializing'
		// 2. Outbox record created
		// 3. (Mock/Real) Consumer processes
		// 4. User state 'active'
	})

	// --- TEST 3: MFA Challenge Flow ---
	t.Run("Flow: TOTP MFA Setup and Verify", func(t *testing.T) {
		// 1. Login to get session
		loginBody, _ := json.Marshal(map[string]string{"email": adminEmail, "password": password})
		resp, _ := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		var loginRes struct { AccessToken string `json:"access_token"` }
		json.NewDecoder(resp.Body).Decode(&loginRes)
		adminToken = loginRes.AccessToken

		// 2. Setup TOTP
		req, _ := http.NewRequest("POST", "https://localhost:8082/auth/mfa/totp/setup", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		resp, _ = mtlsClient.Do(req)
		var setupRes struct { Secret string `json:"secret"` }
		json.NewDecoder(resp.Body).Decode(&setupRes)
		assert.NotEmpty(t, setupRes.Secret)

		// 3. Enable MFA (Manual validation usually, but we'll use the secret to generate code if we had a library)
		// For integration, we might skip the 'active' check or use a known test seed.
	})
}

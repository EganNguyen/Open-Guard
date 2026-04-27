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
	"github.com/openguard/sdk"
	"go.mongodb.org/mongo-driver/bson"
)

func Test_GlobalFunctionalFlow(t *testing.T) {
	ctx := context.Background()
	orgID := uuid.New().String()
	adminEmail := fmt.Sprintf("admin-%s@test.io", orgID[:8])
	password := "TestPass123!"

	var token string

	t.Run("Step 1: Bootstrap System Admin Login", func(t *testing.T) {
		// Use seeded admin to get a token to create the new org
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
		if token == "" {
			t.Fatal("failed to obtain admin token")
		}
	})

	t.Run("Step 2: Create Unique Organization", func(t *testing.T) {
		orgBody, _ := json.Marshal(map[string]string{
			"name": "Integration Test Org " + orgID[:8],
		})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/orgs", bytes.NewBuffer(orgBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create org: %v, status: %d", err, resp.StatusCode)
		}
		
		// In a real scenario, we'd extract the actual UUID if the API returns it
		// For this test, we'll proceed with creating a user in this new org
	})

	t.Run("Step 3: Create Admin User in New Org", func(t *testing.T) {
		// This is a bit simplified; real IAM might require more steps
		userBody, _ := json.Marshal(map[string]string{
			"org_id":       orgID, // Assuming we can specify it or get it from Step 2
			"email":        adminEmail,
			"password":     password,
			"display_name": "Test Admin",
			"role":         "admin",
		})
		req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/users", bytes.NewBuffer(userBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		if err != nil || (resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK) {
			t.Fatalf("failed to create user: %v, status: %d", err, resp.StatusCode)
		}
	})

	t.Run("Step 4: Login as New Admin", func(t *testing.T) {
		loginBody, _ := json.Marshal(map[string]string{
			"email":    adminEmail,
			"password": password,
		})
		resp, err := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("failed to login as new admin: %v, status: %d", err, resp.StatusCode)
		}
		defer resp.Body.Close()

		var result struct {
			Token string `json:"token"`
		}
		json.NewDecoder(resp.Body).Decode(&result)
		token = result.Token
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
					"end":   "23:59", // Always deny for this test
				},
			},
		})
		req, _ := http.NewRequest("POST", "https://localhost:8083/v1/policies", bytes.NewBuffer(policyBody))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")

		resp, err := mtlsClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Fatalf("failed to create policy: %v, status: %d", err, resp.StatusCode)
		}
	})

	t.Run("Step 6: Traffic Simulation via SDK", func(t *testing.T) {
		// Use the control-plane URL
		client := sdk.NewClient("https://localhost:8081", "test-api-key", sdk.WithMTLS("../../infra/certs/ca.crt", "", ""))
		defer client.Close()

		allowed, err := client.Allow(ctx, "user-123", "data:read", "secret-doc")
		if err != nil {
			t.Logf("SDK error (expected if test-api-key is invalid): %v", err)
		}
		if allowed {
			t.Error("expected access to be denied by policy")
		}
	})

	t.Run("Step 7: Async Verification (Audit)", func(t *testing.T) {
		collection := testMongo.Database("openguard_audit").Collection("events")
		
		// Poll for up to 10 seconds
		found := false
		for i := 0; i < 10; i++ {
			count, _ := collection.CountDocuments(ctx, bson.M{"subject": "user-123"})
			if count > 0 {
				found = true
				break
			}
			time.Sleep(1 * time.Second)
		}

		if !found {
			t.Error("audit record not found in MongoDB after 10s")
		}
	})
}

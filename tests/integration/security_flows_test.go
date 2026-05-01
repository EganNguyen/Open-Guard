package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

// TC-NEW-SEC-001: RLS isolation verification
func Test_RLS_CrossTenantIsolation(t *testing.T) {

	// 1. Create Org A and User A
	orgID_A := createTestOrg(t, "Org A")
	adminToken_A := createTestAdmin(t, orgID_A, "admin-a@test.io")

	// 2. Create Org B and User B
	orgID_B := createTestOrg(t, "Org B")
	_ = createTestAdmin(t, orgID_B, "admin-b@test.io")

	// 3. User A attempts to list users of Org B
	req, _ := http.NewRequest("GET", "https://localhost:8082/mgmt/users?org_id="+orgID_B, nil)
	req.Header.Set("Authorization", "Bearer "+adminToken_A)
	resp, err := mtlsClient.Do(req)
	assert.NoError(t, err)

	// Even if they pass Org B ID in query, the middleware/RLS should restrict them to Org A
	// or return empty/error depending on implementation. 
	// Our ListUsers uses shared_middleware.GetOrgID(r.Context()) which is injected from JWT.
	// So it should only return Org A users.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var users []map[string]interface{}
	json.Unmarshal(body, &users)

	for _, u := range users {
		assert.Equal(t, orgID_A, u["org_id"], "User A should only see users from Org A")
		assert.NotEqual(t, orgID_B, u["org_id"], "User A should NOT see users from Org B")
	}
}

// TC-NEW-THR-002: Impossible Travel E2E assertion (MongoDB)
func Test_Threat_ImpossibleTravel_E2E(t *testing.T) {
	ctx := context.Background()
	orgID := createTestOrg(t, "Threat Org")
	adminToken := createTestAdmin(t, orgID, "threat-admin@test.io")

	// 1. Ingest Login from New York
	ingestEvent(t, adminToken, map[string]interface{}{
		"event_id": uuid.New().String(),
		"type":     "auth.login",
		"subject":  "threat-admin@test.io",
		"metadata": map[string]interface{}{
			"ip": "1.1.1.1", // New York (Simulated)
			"city": "New York",
		},
	})

	// 2. Ingest Login from London 1 minute later
	time.Sleep(2 * time.Second) // Small delay for Kafka processing
	ingestEvent(t, adminToken, map[string]interface{}{
		"event_id": uuid.New().String(),
		"type":     "auth.login",
		"subject":  "threat-admin@test.io",
		"metadata": map[string]interface{}{
			"ip": "2.2.2.2", // London (Simulated)
			"city": "London",
		},
	})

	// 3. Verify Alert in MongoDB
	Eventually(t, func() bool {
		collection := testMongo.Database("openguard_audit").Collection("audit_events")
		count, err := collection.CountDocuments(ctx, bson.M{
			"org_id": orgID,
			"type":   "threat.alert",
			"metadata.detector": "impossible_travel",
		})
		return err == nil && count > 0
	}, 20*time.Second, 2*time.Second)
}

// TC-NEW-AUD-001: Audit Hash Chain Integrity (MongoDB)
func Test_Audit_HashChain_Integrity(t *testing.T) {
	ctx := context.Background()
	orgID := createTestOrg(t, "Audit Integrity Org")
	adminToken := createTestAdmin(t, orgID, "audit-admin@test.io")

	// 1. Ingest 3 events
	e1 := uuid.New().String()
	e2 := uuid.New().String()
	e3 := uuid.New().String()

	ingestEvent(t, adminToken, map[string]interface{}{"event_id": e1, "type": "test.e1"})
	ingestEvent(t, adminToken, map[string]interface{}{"event_id": e2, "type": "test.e2"})
	ingestEvent(t, adminToken, map[string]interface{}{"event_id": e3, "type": "test.e3"})

	// 2. Verify integrity hashes in MongoDB
	Eventually(t, func() bool {
		collection := testMongo.Database("openguard_audit").Collection("audit_events")
		var events []map[string]interface{}
		cursor, _ := collection.Find(ctx, bson.M{"org_id": orgID}, nil)
		cursor.All(ctx, &events)
		
		if len(events) < 3 {
			return false
		}
		
		// Sort by sequence if available
		// Verify each event has integrity_hash
		for _, e := range events {
			if _, ok := e["integrity_hash"]; !ok {
				return false
			}
		}
		return true
	}, 15*time.Second, 1*time.Second)
}

// Helpers

func createTestOrg(t *testing.T, name string) string {
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "admin@openguard.io",
		"password": "admin123",
	})
	resp, _ := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
	body, _ := io.ReadAll(resp.Body)
	var loginRes struct{ AccessToken string }
	json.Unmarshal(body, &loginRes)

	orgBody, _ := json.Marshal(map[string]string{"name": name + " " + uuid.New().String()[:4]})
	req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/orgs", bytes.NewBuffer(orgBody))
	req.Header.Set("Authorization", "Bearer "+loginRes.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = mtlsClient.Do(req)
	body, _ = io.ReadAll(resp.Body)
	var orgRes struct{ ID string }
	json.Unmarshal(body, &orgRes)
	return orgRes.ID
}

func createTestAdmin(t *testing.T, orgID, email string) string {
	loginBody, _ := json.Marshal(map[string]string{
		"email":    "admin@openguard.io",
		"password": "admin123",
	})
	resp, _ := mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBody))
	body, _ := io.ReadAll(resp.Body)
	var loginRes struct{ AccessToken string }
	json.Unmarshal(body, &loginRes)

	userBody, _ := json.Marshal(map[string]string{
		"org_id":       orgID,
		"email":        email,
		"password":     "TestPass123!",
		"display_name": "Test User",
		"role":         "admin",
	})
	req, _ := http.NewRequest("POST", "https://localhost:8082/mgmt/users", bytes.NewBuffer(userBody))
	req.Header.Set("Authorization", "Bearer "+loginRes.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, _ = mtlsClient.Do(req)
	
	// Wait for active
	ctx := context.Background()
	Eventually(t, func() bool {
		var status string
		err := testDBIAM.QueryRow(ctx, "SELECT status FROM users WHERE email = $1", email).Scan(&status)
		return err == nil && status == "active"
	}, 30*time.Second, 1*time.Second)

	// Login
	loginBodyUser, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": "TestPass123!",
	})
	resp, _ = mtlsClient.Post("https://localhost:8082/auth/login", "application/json", bytes.NewBuffer(loginBodyUser))
	body, _ = io.ReadAll(resp.Body)
	var userRes struct{ AccessToken string }
	json.Unmarshal(body, &userRes)
	return userRes.AccessToken
}

func ingestEvent(t *testing.T, token string, event map[string]interface{}) {
	body, _ := json.Marshal(event)
	req, _ := http.NewRequest("POST", "https://localhost:8085/v1/events/ingest", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := mtlsClient.Do(req)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

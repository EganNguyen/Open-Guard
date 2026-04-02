package models_test

import (
	"testing"
	"time"

	"github.com/openguard/audit/pkg/models"
)

func TestChainHash(t *testing.T) {
	secret := "test-secret"
	prevHash := "foo_hash"
	
	occurred := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	
	ev := models.AuditEvent{
		EventID:    "evt-123",
		OrgID:      "org-abc",
		Type:       "auth.login",
		OccurredAt: occurred,
	}
	
	hash1 := models.ChainHash(secret, prevHash, ev)
	
	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}
	
	// Ensure determinism
	hash2 := models.ChainHash(secret, prevHash, ev)
	if hash1 != hash2 {
		t.Fatalf("expected hash determinism, got %s and %s", hash1, hash2)
	}
	
	// Change payload changes hash
	ev2 := ev
	ev2.OrgID = "org-xyz"
	hash3 := models.ChainHash(secret, prevHash, ev2)
	if hash1 == hash3 {
		t.Fatal("expected different hash for different org")
	}
}

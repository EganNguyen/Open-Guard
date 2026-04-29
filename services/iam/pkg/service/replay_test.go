package service_test

import (
	"context"
	"testing"

	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/shared/crypto"
)

func (m *MockRepository) GetMFAConfig(ctx context.Context, userID, mfaType string) (map[string]interface{}, error) {
	for _, config := range m.MFAConfigs[userID] {
		if config["mfa_type"] == mfaType {
			return config, nil
		}
	}
	return nil, nil
}

func TestVerifyTOTP_ReplayProtection(t *testing.T) {
	s, repo, _ := setup(t)
	
	userID := "user1"
	
	secret := "JBSWY3DPEHPK3PXP" // Base32
	aesKey := []byte("01234567890123456789012345678901") // 32 bytes
	aesKeyring := []crypto.EncryptionKey{{Kid: "a1", Key: string(aesKey), Status: "active"}}
	
	// Create a new service with AES keyring
	pool := service.NewAuthWorkerPool(1, context.Background())
	keyring := []crypto.JWTKey{{Kid: "k1", Secret: "test-secret-at-least-32-bytes!!", Algorithm: "HS256", Status: "active"}}
	s = service.NewService(repo, pool, keyring, aesKeyring, s.Redis())

	encrypted, _ := crypto.Encrypt([]byte(secret), aesKeyring)
	repo.MFAConfigs[userID] = []map[string]interface{}{
		{"mfa_type": "totp", "secret_encrypted": encrypted},
	}

	code := "123456" 

	ctx := context.Background()
	
	// 1. First attempt
	_, err := s.VerifyTOTP(ctx, userID, code)
	if err != nil && err.Error() == "totp code already used" {
		t.Error("First call should not trigger replay protection")
	}

	// 2. Second attempt with the SAME code should trigger the replay error
	_, err = s.VerifyTOTP(ctx, userID, code)
	if err == nil || err.Error() != "totp code already used" {
		t.Errorf("Expected 'totp code already used' error on second attempt, got %v", err)
	}
}

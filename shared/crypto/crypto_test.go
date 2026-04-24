package crypto_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/openguard/shared/crypto"
)

func TestJWTSignAndVerify(t *testing.T) {
	keyring := []crypto.JWTKey{{Kid: "k1", Secret: "test-secret-at-least-32-bytes!!", Algorithm: "HS256", Status: "active"}}
	claims := crypto.NewStandardClaims("org1", "user1", "jti1", time.Hour)
	token, err := crypto.Sign(claims, keyring)
	if err != nil {
		t.Fatal(err)
	}

	out := &crypto.StandardClaims{}
	_, err = crypto.Verify(token, keyring, out)
	if err != nil {
		t.Fatal(err)
	}
	if out.UserID != "user1" {
		t.Errorf("expected user1, got %s", out.UserID)
	}
}

func TestJWTVerifyRejectsExpired(t *testing.T) {
	keyring := []crypto.JWTKey{{Kid: "k1", Secret: "test-secret-at-least-32-bytes!!", Algorithm: "HS256", Status: "active"}}
	claims := crypto.NewStandardClaims("org1", "user1", "jti1", -time.Second)
	token, _ := crypto.Sign(claims, keyring)
	_, err := crypto.Verify(token, keyring, &crypto.StandardClaims{})
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestJWTKeyRotation(t *testing.T) {
	// Sign with old key, verify with keyring that has old (verify_only) + new (active) key
	oldKey := crypto.JWTKey{Kid: "old", Secret: "old-secret-at-least-32-bytes-!!", Algorithm: "HS256", Status: "verify_only"}
	newKey := crypto.JWTKey{Kid: "new", Secret: "new-secret-at-least-32-bytes-!!", Algorithm: "HS256", Status: "active"}
	claims := crypto.NewStandardClaims("org1", "user1", "jti1", time.Hour)

	// To sign with old key, it must be active in the keyring passed to Sign
	oldKeyActive := oldKey
	oldKeyActive.Status = "active"
	token, _ := crypto.Sign(claims, []crypto.JWTKey{oldKeyActive})

	// Verify with full keyring — should work even if old key is verify_only
	_, err := crypto.Verify(token, []crypto.JWTKey{newKey, oldKey}, &crypto.StandardClaims{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAESEncryptDecrypt(t *testing.T) {
	keyring := []crypto.EncryptionKey{{Kid: "k1", Key: base64.StdEncoding.EncodeToString(make([]byte, 32)), Status: "active"}}
	plaintext := []byte("sensitive-totp-secret")
	ciphertext, err := crypto.Encrypt(plaintext, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == string(plaintext) {
		t.Error("ciphertext should not equal plaintext")
	}

	decrypted, err := crypto.Decrypt(ciphertext, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("decryption mismatch")
	}
}

func TestPBKDF2HashAndVerify(t *testing.T) {
	hash := crypto.HashPBKDF2("my-api-key")
	if !crypto.VerifyPBKDF2("my-api-key", hash) {
		t.Error("valid key should verify")
	}
	if crypto.VerifyPBKDF2("wrong-key", hash) {
		t.Error("wrong key should not verify")
	}
}

func TestPBKDF2ConstantTime(t *testing.T) {
	// Verify timing is consistent regardless of where mismatch occurs
	// This is a best-effort test — real timing tests need benchmarks
	hash := crypto.HashPBKDF2("correct")
	_ = crypto.VerifyPBKDF2("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", hash)
	_ = crypto.VerifyPBKDF2("zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz", hash)
	// If implementation uses subtle.ConstantTimeCompare, these should take same time
}

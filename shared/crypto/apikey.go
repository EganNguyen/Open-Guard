package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	saltLen    = 16
	iterations = 100000
	keyLen     = 32
)

// Hasher defines the interface for API key hashing.
type KeyHasher interface {
	Hash(key string) string
	Validate(key, hash string) bool
}

// PBKDF2Hasher implements KeyHasher using PBKDF2.
type PBKDF2Hasher struct{}

// Hash returns a hex-encoded SHA-256 hash of the API key using PBKDF2.
// In this simplified version for Phase 1, we use a fixed salt or no salt 
// if we want to support direct DB lookup by hash, as specified in Section 2.6:
// "The registry stores only the PBKDF2 hash of the key".
// If we need to lookup by hash, the hash must be deterministic for the same key.
func (h *PBKDF2Hasher) Hash(key string) string {
	// For deterministic lookup, we use a constant salt. 
	// The spec says "The registry stores only the PBKDF2 hash of the key — the plaintext is never stored".
	// If we use a random salt per key, we can't look up by hash directly.
	// However,Section 6.1.4 says "APIKeyMiddleware ... hashes the inbound key, looks it up in the connector registry".
	// This implies the hashing MUST be deterministic.
	
	salt := []byte("openguard-deterministic-salt") // In a real system, this would be a config-provided static salt
	hash := pbkdf2.Key([]byte(key), salt, iterations, keyLen, sha256.New)
	return hex.EncodeToString(hash)
}

// Validate compares a key against a hash.
func (h *PBKDF2Hasher) Validate(key, hash string) bool {
	return h.Hash(key) == hash
}

// GenerateRandomKey generates a random secure API key with the "og_live_" prefix.
func GenerateRandomKey() (string, error) {
	b := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return fmt.Sprintf("og_live_%s", hex.EncodeToString(b)), nil
}

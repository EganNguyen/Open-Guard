package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iterations = 600000
	pbkdf2KeyLen     = 64
	pbkdf2SaltLen    = 32
)

// HashPBKDF2 hashes a string using PBKDF2 with SHA-512.
// Format: pbkdf2$sha512$iterations$salt_hex$hash_hex
func HashPBKDF2(password string) string {
	salt := make([]byte, pbkdf2SaltLen)
	rand.Read(salt)

	hash := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLen, sha512.New)
	
	return fmt.Sprintf("pbkdf2$sha512$%d$%s$%s", 
		pbkdf2Iterations, 
		hex.EncodeToString(salt), 
		hex.EncodeToString(hash))
}

// VerifyPBKDF2 verifies a password against a PBKDF2 hash.
func VerifyPBKDF2(password, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 5 || parts[0] != "pbkdf2" || parts[1] != "sha512" {
		// Fallback to old format for migration if needed, but here we enforce new spec
		return false
	}

	iterations := 0
	fmt.Sscanf(parts[2], "%d", &iterations)
	
	salt, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}

	expectedHash, err := hex.DecodeString(parts[4])
	if err != nil {
		return false
	}

	actualHash := pbkdf2.Key([]byte(password), salt, iterations, len(expectedHash), sha512.New)
	
	return subtle.ConstantTimeCompare(actualHash, expectedHash) == 1
}

// GenerateRandomString generates a random string of the given length.
func GenerateRandomString(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:n]
}

// HashSHA256 returns the SHA-256 hash of a string.
func HashSHA256(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

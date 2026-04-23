package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iterations = 10000
	pbkdf2KeyLen     = 32
	pbkdf2SaltLen    = 16
)

// HashPBKDF2 hashes a string using PBKDF2 with SHA-256.
func HashPBKDF2(password string) string {
	salt := make([]byte, pbkdf2SaltLen)
	rand.Read(salt)

	hash := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLen, sha256.New)
	
	// Format: iterations.salt.hash
	return fmt.Sprintf("%d.%s.%s", 
		pbkdf2Iterations, 
		base64.StdEncoding.EncodeToString(salt), 
		base64.StdEncoding.EncodeToString(hash))
}

// VerifyPBKDF2 verifies a password against a PBKDF2 hash.
func VerifyPBKDF2(password, encodedHash string) bool {
	parts := strings.Split(encodedHash, ".")
	if len(parts) != 3 {
		return false
	}

	iterations := 0
	fmt.Sscanf(parts[0], "%d", &iterations)
	
	salt, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	expectedHash, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}

	actualHash := pbkdf2.Key([]byte(password), salt, iterations, len(expectedHash), sha256.New)
	
	return string(actualHash) == string(expectedHash)
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

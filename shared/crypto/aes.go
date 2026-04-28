package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrInvalidCiphertext = errors.New("invalid ciphertext format")
	ErrKeyNotFound       = errors.New("encryption key not found")
)

// EncryptionKey represents an AES key in a multi-key keyring.
type EncryptionKey struct {
	Kid    string `json:"kid"`
	Key    string `json:"key"`    // base64-encoded 32-byte key
	Status string `json:"status"` // "active" | "verify_only"
}

// Encrypt encrypts plaintext using the first active key in the keyring.
// Output format: "<kid>:<base64(nonce+ciphertext)>"
func Encrypt(plaintext []byte, keyring []EncryptionKey) (string, error) {
	var activeKey *EncryptionKey
	for _, k := range keyring {
		if k.Status == "active" {
			activeKey = &k
			break
		}
	}

	if activeKey == nil {
		return "", ErrKeyNotFound
	}

	key, err := base64.StdEncoding.DecodeString(activeKey.Key)
	if err != nil {
		return "", fmt.Errorf("decode key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return fmt.Sprintf("%s:%s", activeKey.Kid, base64.StdEncoding.EncodeToString(ciphertext)), nil
}

// Decrypt decrypts ciphertext by identifying the kid from the prefix and using the matching key.
func Decrypt(encodedCiphertext string, keyring []EncryptionKey) ([]byte, error) {
	parts := strings.Split(encodedCiphertext, ":")
	if len(parts) != 2 {
		return nil, ErrInvalidCiphertext
	}

	kid := parts[0]
	data, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}

	var targetKey *EncryptionKey
	for _, k := range keyring {
		if k.Kid == kid {
			targetKey = &k
			break
		}
	}

	if targetKey == nil {
		return nil, ErrKeyNotFound
	}

	key, err := base64.StdEncoding.DecodeString(targetKey.Key)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// LoadAESKeyring parses a JSON-encoded AES keyring.
func LoadAESKeyring(jsonStr string) ([]EncryptionKey, error) {
	var keyring []EncryptionKey
	if err := json.Unmarshal([]byte(jsonStr), &keyring); err != nil {
		return nil, fmt.Errorf("invalid AES keyring JSON: %w", err)
	}
	return keyring, nil
}

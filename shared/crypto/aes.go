package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

type AESKey struct {
	ID     string `json:"id"`
	Secret string `json:"secret"` // base64 encoded 32-byte key
	Status string `json:"status"` // "active" | "retiring"
}

type AESKeyring struct {
	keys []AESKey
}

func NewAESKeyring(keys []AESKey) *AESKeyring {
	return &AESKeyring{keys: keys}
}

// Encrypt uses the first 'active' key and returns "v1:keyid:base64iv:base64cipher".
func (k *AESKeyring) Encrypt(plaintext string) (string, error) {
	for _, key := range k.keys {
		if key.Status == "active" {
			keyBytes, err := base64.StdEncoding.DecodeString(key.Secret)
			if err != nil {
				return "", err
			}
			block, err := aes.NewCipher(keyBytes)
			if err != nil {
				return "", err
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return "", err
			}
			nonce := make([]byte, gcm.NonceSize())
			if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
				return "", err
			}
			ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
			return fmt.Sprintf("v1:%s:%s:%s", key.ID, base64.StdEncoding.EncodeToString(nonce), base64.StdEncoding.EncodeToString(ciphertext)), nil
		}
	}
	return "", errors.New("no active aes key found")
}

// Decrypt parses the ciphertext enveloped string and decrypts using the specified key ID.
func (k *AESKeyring) Decrypt(encrypted string) (string, error) {
	parts := strings.Split(encrypted, ":")
	if len(parts) != 4 || parts[0] != "v1" {
		return "", errors.New("invalid ciphertext format")
	}
	keyID, nonceB64, cipherB64 := parts[1], parts[2], parts[3]

	for _, key := range k.keys {
		if key.ID == keyID {
			keyBytes, err := base64.StdEncoding.DecodeString(key.Secret)
			if err != nil {
				return "", err
			}
			block, err := aes.NewCipher(keyBytes)
			if err != nil {
				return "", err
			}
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				return "", err
			}
			nonce, err := base64.StdEncoding.DecodeString(nonceB64)
			if err != nil {
				return "", err
			}
			ciphertext, err := base64.StdEncoding.DecodeString(cipherB64)
			if err != nil {
				return "", err
			}
			pt, err := gcm.Open(nil, nonce, ciphertext, nil)
			if err != nil {
				return "", err
			}
			return string(pt), nil
		}
	}
	return "", errors.New("unknown decryption key id")
}

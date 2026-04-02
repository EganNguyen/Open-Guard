package crypto

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func genKey(lenBytes int) string {
	b := make([]byte, lenBytes)
	for i := range b {
		b[i] = 'a'
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestAESKeyring_EncryptDecrypt(t *testing.T) {
	key32 := genKey(32)

	kr := NewAESKeyring([]AESKey{
		{ID: "key-1", Secret: key32, Status: "active"},
		{ID: "key-old", Secret: key32, Status: "retiring"},
	})

	plaintext := "enterprise secret data"

	// Test Encrypt
	ciphertext, err := kr.Encrypt(plaintext)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ciphertext, "v1:key-1:"))

	// Test Decrypt using the same keyring
	decrypted, err := kr.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestAESKeyring_Errors(t *testing.T) {
	key32 := genKey(32)

	t.Run("no active key encrypt", func(t *testing.T) {
		kr := NewAESKeyring([]AESKey{
			{ID: "key-1", Secret: key32, Status: "retiring"},
		})
		_, err := kr.Encrypt("test")
		assert.ErrorContains(t, err, "no active aes key found")
	})

	t.Run("bad config base64", func(t *testing.T) {
		kr := NewAESKeyring([]AESKey{
			{ID: "key-1", Secret: "not-base64-!@#", Status: "active"},
		})
		_, err := kr.Encrypt("test")
		assert.Error(t, err)

		// Create a fake ciphertext to trigger decrypt error
		_, err = kr.Decrypt("v1:key-1:abcd:efgh")
		assert.Error(t, err)
	})

	t.Run("decrypt bad format", func(t *testing.T) {
		kr := NewAESKeyring([]AESKey{})
		_, err := kr.Decrypt("invalid:format")
		assert.ErrorContains(t, err, "invalid ciphertext format")
	})

	t.Run("decrypt unknown key id", func(t *testing.T) {
		kr := NewAESKeyring([]AESKey{
			{ID: "key-1", Secret: key32, Status: "active"},
		})
		_, err := kr.Decrypt("v1:unknown-key:abcd:efgh")
		assert.ErrorContains(t, err, "unknown decryption key id")
	})

	t.Run("decrypt bad base64 nonces", func(t *testing.T) {
		kr := NewAESKeyring([]AESKey{{ID: "k1", Secret: key32, Status: "active"}})
		_, err := kr.Decrypt("v1:k1:!@#$:!@#$")
		assert.Error(t, err)
	})
}

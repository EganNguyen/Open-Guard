package crypto

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWTKeyring_SignVerify(t *testing.T) {
	kr := NewJWTKeyring([]JWTKey{
		{Kid: "key-1", Secret: "super-secret-1", Algorithm: "HS256", Status: "active"},
		{Kid: "key-2", Secret: "super-secret-2", Algorithm: "HS256", Status: "verify_only"},
	})

	claims := jwt.RegisteredClaims{
		Subject:   "user-123",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
	}

	// Sign
	tokenStr, err := kr.Sign(claims)
	require.NoError(t, err)
	assert.NotEmpty(t, tokenStr)

	// Verify
	parsedClaims, err := kr.Verify(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "user-123", parsedClaims["sub"])
}

func TestJWTKeyring_Errors(t *testing.T) {
	t.Run("no active key sign", func(t *testing.T) {
		kr := NewJWTKeyring([]JWTKey{{Status: "verify_only"}})
		_, err := kr.Sign(jwt.MapClaims{})
		assert.ErrorContains(t, err, "no active jwt key found")
	})

	t.Run("verify unknown kid", func(t *testing.T) {
		kr := NewJWTKeyring([]JWTKey{{Kid: "known", Secret: "secret", Algorithm: "HS256", Status: "active"}})
		krErr := NewJWTKeyring([]JWTKey{{Kid: "unknown", Secret: "diff-secret", Algorithm: "HS256", Status: "active"}})

		token, _ := krErr.Sign(jwt.MapClaims{"sub": "1"})
		_, err := kr.Verify(token)
		assert.ErrorContains(t, err, "unknown kid")
	})

	t.Run("verify invalid token format", func(t *testing.T) {
		kr := NewJWTKeyring([]JWTKey{})
		_, err := kr.Verify("not.a.token")
		assert.Error(t, err)
	})

	t.Run("verify algorithm mismatch", func(t *testing.T) {
		krSign := NewJWTKeyring([]JWTKey{{Kid: "k1", Secret: "sec", Algorithm: "HS256", Status: "active"}})
		// The verifier expects HS512 for the same kid
		krVer := NewJWTKeyring([]JWTKey{{Kid: "k1", Secret: "sec", Algorithm: "HS512", Status: "active"}})

		token, _ := krSign.Sign(jwt.MapClaims{"sub": "1"})
		_, err := krVer.Verify(token)
		assert.ErrorContains(t, err, "unexpected signing method")
	})

	t.Run("expired token", func(t *testing.T) {
		kr := NewJWTKeyring([]JWTKey{{Kid: "k1", Secret: "sec", Algorithm: "HS256", Status: "active"}})
		claims := jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		}
		token, _ := kr.Sign(claims)

		_, err := kr.Verify(token)
		assert.ErrorContains(t, err, "token is expired")
	})
}

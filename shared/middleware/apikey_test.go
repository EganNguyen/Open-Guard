package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVerifyPBKDF2_ValidHash(t *testing.T) {
	cases := []struct {
		name      string
		rawKey   string
		hash     string
		wantPass bool
	}{
		{
			name:      "Valid hash format",
			rawKey:   "test-key-12345",
			hash:     "pbkdf2$sha512$600000$deadbeef$" + derivePBKDF2("test-key-12345", "deadbeef", 600000),
			wantPass: true,
		},
		{
			name:      "Invalid algorithm",
			rawKey:   "test-key-12345",
			hash:     "pbkdf2$sha256$600000$deadbeef$abc123",
			wantPass: false,
		},
		{
			name:      "Too few iterations",
			rawKey:   "test-key-12345",
			hash:     "pbkdf2$sha512$1000$deadbeef$abc123",
			wantPass: false,
		},
		{
			name:      "Invalid format",
			rawKey:   "test-key-12345",
			hash:     "invalid$format",
			wantPass: false,
		},
		{
			name:      "Wrong key",
			rawKey:   "wrong-key",
			hash:     "pbkdf2$sha512$600000$deadbeef$" + derivePBKDF2("test-key-12345", "deadbeef", 600000),
			wantPass: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := verifyPBKDF2(tc.rawKey, tc.hash)
			assert.Equal(t, tc.wantPass, result)
		})
	}
}

func TestConstantTimeCompare(t *testing.T) {
	cases := []struct {
		name  string
		a     string
		b     string
		want  bool
	}{
		{"Equal strings", "hello", "hello", true},
		{"Different strings", "hello", "world", false},
		{"Both empty", "", "", true},
		{"One empty", "", "a", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := constantTimeCompare(tc.a, tc.b)
			assert.Equal(t, tc.want, result)
		})
	}
}

func TestDerivePBKDF2(t *testing.T) {
	hash := derivePBKDF2("test-key", "deadbeef", 600000)
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 128) // 64 bytes hex encoded

	// Same input produces same output
	hash2 := derivePBKDF2("test-key", "deadbeef", 600000)
	assert.Equal(t, hash, hash2)

	// Different salt produces different output
	hash3 := derivePBKDF2("test-key", "cafebabe", 600000)
	assert.NotEqual(t, hash, hash3)
}
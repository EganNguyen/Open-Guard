package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMust(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		os.Setenv("TEST_MUST", "value")
		defer os.Unsetenv("TEST_MUST")
		assert.Equal(t, "value", Must("TEST_MUST"))
	})
	t.Run("missing", func(t *testing.T) {
		os.Unsetenv("TEST_MUST_MISSING")
		assert.Panics(t, func() { Must("TEST_MUST_MISSING") })
	})
}

func TestDefault(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		os.Setenv("TEST_DEFAULT", "value")
		defer os.Unsetenv("TEST_DEFAULT")
		assert.Equal(t, "value", Default("TEST_DEFAULT", "fallback"))
	})
	t.Run("missing", func(t *testing.T) {
		os.Unsetenv("TEST_DEFAULT_MISSING")
		assert.Equal(t, "fallback", Default("TEST_DEFAULT_MISSING", "fallback"))
	})
}

func TestMustInt(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		os.Setenv("TEST_MUST_INT", "123")
		defer os.Unsetenv("TEST_MUST_INT")
		assert.Equal(t, 123, MustInt("TEST_MUST_INT"))
	})
	t.Run("invalid", func(t *testing.T) {
		os.Setenv("TEST_MUST_INT_INV", "abc")
		defer os.Unsetenv("TEST_MUST_INT_INV")
		assert.Panics(t, func() { MustInt("TEST_MUST_INT_INV") })
	})
}

func TestDefaultInt(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		os.Setenv("TEST_DEFAULT_INT", "456")
		defer os.Unsetenv("TEST_DEFAULT_INT")
		assert.Equal(t, 456, DefaultInt("TEST_DEFAULT_INT", 999))
	})
	t.Run("missing", func(t *testing.T) {
		os.Unsetenv("TEST_DEFAULT_INT_MISSING")
		assert.Equal(t, 999, DefaultInt("TEST_DEFAULT_INT_MISSING", 999))
	})
	t.Run("invalid", func(t *testing.T) {
		os.Setenv("TEST_DEFAULT_INT_INV", "abc")
		defer os.Unsetenv("TEST_DEFAULT_INT_INV")
		assert.Panics(t, func() { DefaultInt("TEST_DEFAULT_INT_INV", 999) })
	})
}

func TestMustDuration(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		os.Setenv("TEST_MUST_DUR", "5000")
		defer os.Unsetenv("TEST_MUST_DUR")
		assert.Equal(t, 5000*time.Millisecond, MustDuration("TEST_MUST_DUR"))
	})
}

func TestMustJSON(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		os.Setenv("TEST_MUST_JSON", `{"key":"value"}`)
		defer os.Unsetenv("TEST_MUST_JSON")
		var dest map[string]string
		require.NotPanics(t, func() { MustJSON("TEST_MUST_JSON", &dest) })
		assert.Equal(t, "value", dest["key"])
	})
	t.Run("invalid", func(t *testing.T) {
		os.Setenv("TEST_MUST_JSON_INV", `{invalid}`)
		defer os.Unsetenv("TEST_MUST_JSON_INV")
		var dest map[string]string
		assert.Panics(t, func() { MustJSON("TEST_MUST_JSON_INV", &dest) })
	})
}

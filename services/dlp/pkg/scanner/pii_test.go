package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskValue_PII_Integrity(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		input    string
		expected string
	}{
		{
			name:     "Mask Credit Card",
			kind:     "credit_card",
			input:    "4111-1111-1111-4444",
			expected: "****-****-****-4444",
		},
		{
			name:     "Mask SSN",
			kind:     "ssn",
			input:    "123-45-6789",
			expected: "***-**-6789",
		},
		{
			name:     "Mask Default (Email)",
			kind:     "email",
			input:    "test@example.com",
			expected: "[REDACTED]",
		},
		{
			name:     "Short Input handling",
			kind:     "credit_card",
			input:    "123",
			expected: "[REDACTED]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := MaskValue(tc.kind, tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestScanRegex_MaskingIntegration(t *testing.T) {
	text := "My card is 4111111111111111 and my SSN is 123-45-6789"
	findings := ScanRegex(text)

	assert.Len(t, findings, 2)

	// Findings should contain masked values only
	for _, f := range findings {
		if f.Kind == "credit_card" {
			assert.Equal(t, "****-****-****-1111", f.Value)
		} else if f.Kind == "ssn" {
			assert.Equal(t, "***-**-6789", f.Value)
		}
		// Confirm no raw data leaked into Value field
		assert.NotContains(t, f.Value, "411111111")
		assert.NotContains(t, f.Value, "123-45")
	}
}

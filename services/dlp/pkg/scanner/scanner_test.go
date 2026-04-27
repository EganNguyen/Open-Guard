package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanRegex_TableDriven(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantKind string
		wantVal  string
	}{
		{
			name:     "Valid Email",
			input:    "Contact us at security@openguard.io for info",
			wantKind: "email",
			wantVal:  "[REDACTED]",
		},
		{
			name:     "Valid SSN",
			input:    "My ID is 123-45-6789",
			wantKind: "ssn",
			wantVal:  "***-**-6789",
		},
		{
			name:     "Valid Credit Card",
			input:    "Pay with 4111111111111111", // Valid Luhn Visa
			wantKind: "credit_card",
			wantVal:  "****-****-****-1111",
		},
		{
			name:     "AWS Access Key",
			input:    "Access: AKIAIOSFODNN7EXAMPLE",
			wantKind: "aws_access_key",
			wantVal:  "[REDACTED]",
		},
		{
			name:     "Private Key Boundary",
			input:    "-----BEGIN RSA PRIVATE KEY-----",
			wantKind: "private_key",
			wantVal:  "[REDACTED]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := ScanRegex(tc.input)
			assert.NotEmpty(t, findings)
			assert.Equal(t, tc.wantKind, findings[0].Kind)
			assert.Equal(t, tc.wantVal, findings[0].Value)
		})
	}
}

func TestLuhnCheck(t *testing.T) {
	assert.True(t, LuhnCheck("4111111111111111"))
	assert.False(t, LuhnCheck("4111111111111112"))
}

func TestScanEntropy(t *testing.T) {
	// Low entropy sentence
	low := ScanEntropy("this is a normal low entropy sentence for testing")
	assert.Empty(t, low)

	// High entropy secret
	high := ScanEntropy("SECRET_KEY=Ym9sZC1pbnZlbnRvcnktY29yZS1zZWNyZXQta2V5LXdpdGgtbG90cy1vZi1lbnRyb3B5")
	assert.NotEmpty(t, high)
	assert.Equal(t, "potential_credential", high[0].Kind)
}

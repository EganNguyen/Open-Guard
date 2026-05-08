package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScanContent_CombinesRegexAndEntropy(t *testing.T) {
	cases := []struct {
		name          string
		input        string
		wantFindings int
		wantRisk     float64
		wantPII     bool
		wantSecrets bool
	}{
		{
			name:          "Email Only PII",
			input:        "Contact security@openguard.io",
			wantFindings: 1,
			wantRisk:    0.8,
			wantPII:    true,
			wantSecrets: false,
		},
		{
			name:          "Secret Only",
			input:        "KEY=AKIAIOSFODNN7EXAMPLE",
			wantFindings: 1,
			wantRisk:    0.8,
			wantPII:    false,
			wantSecrets: true,
		},
		{
			name:          "No Finding",
			input:        "Hello world",
			wantFindings: 0,
			wantRisk:    0,
			wantPII:    false,
			wantSecrets: false,
		},
		{
			name:          "Credit Card Financial",
			input:        "Card: 4111111111111111",
			wantFindings: 1,
			wantRisk:    0.8,
			wantSecrets: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ScanContent(tc.input)
			assert.Equal(t, tc.wantFindings, len(result.Findings))
			assert.Equal(t, tc.wantRisk, result.RiskScore)
			assert.Equal(t, tc.wantPII, result.HasPII)
			assert.Equal(t, tc.wantSecrets, result.HasSecrets)
		})
	}
}

func TestScanContent_RiskScoreIsMaximum(t *testing.T) {
	text := "email@test.com AWS_KEY=xxx SSN=123-45-6789"
	result := ScanContent(text)
	assert.Equal(t, 0.8, result.RiskScore) // max of all
}

func TestScanContent_EmptyInput(t *testing.T) {
	result := ScanContent("")
	assert.Empty(t, result.Findings)
	assert.Equal(t, 0.0, result.RiskScore)
	assert.False(t, result.HasPII)
	assert.False(t, result.HasSecrets)
}
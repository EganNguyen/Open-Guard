package scanner

import (
	"math"
	"strings"
)

func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int)
	for _, c := range s {
		freq[c]++
	}
	var entropy float64
	for _, count := range freq {
		p := float64(count) / float64(len(s))
		entropy -= p * math.Log2(p)
	}
	return entropy
}

func ScanEntropy(text string) []Finding {
	var findings []Finding
	// Split by whitespace and common delimiters to find potential tokens/secrets
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ',' || r == ';' || r == '='
	})

	for _, token := range tokens {
		// Threshold: entropy > 4.5 AND len(token) >= 20 → potential credential
		if len(token) >= 20 && ShannonEntropy(token) > 4.5 {
			findings = append(findings, Finding{
				Kind:      "potential_credential",
				Value:     token,
				RiskScore: 0.9,
			})
		}
	}
	return findings
}

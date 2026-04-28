package scanner

// ScanResult combines all findings from a content scan.
type ScanResult struct {
	Findings     []Finding
	RiskScore    float64 // 0.0 – 1.0 composite
	HasPII       bool
	HasSecrets   bool
	HasFinancial bool
}

// ScanContent runs all detectors (regex + entropy) and returns a combined result.
// This is the primary entry point for the DLP engine.
func ScanContent(text string) ScanResult {
	var allFindings []Finding

	// Regex-based PII/credential detection
	regexFindings := ScanRegex(text)
	allFindings = append(allFindings, regexFindings...)

	// Entropy-based secret detection
	entropyFindings := ScanEntropy(text)
	allFindings = append(allFindings, entropyFindings...)

	// Compute composite risk score (max of individual scores)
	var maxScore float64
	var hasPII, hasSecrets, hasFinancial bool
	for _, f := range allFindings {
		if f.RiskScore > maxScore {
			maxScore = f.RiskScore
		}
		switch f.Kind {
		case "email", "ssn", "phone_us":
			hasPII = true
		case "aws_access_key", "private_key", "potential_credential":
			hasSecrets = true
		case "credit_card":
			hasFinancial = true
		}
	}

	return ScanResult{
		Findings:     allFindings,
		RiskScore:    maxScore,
		HasPII:       hasPII,
		HasSecrets:   hasSecrets,
		HasFinancial: hasFinancial,
	}
}

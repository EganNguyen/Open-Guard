package scanner

import (
	"regexp"
	"strconv"
)

type ScanRule struct {
	Kind     string
	Re       *regexp.Regexp
	Validate func(string) bool
}

var Rules = []ScanRule{
	{
		Kind: "email",
		Re:   regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	},
	{
		Kind: "ssn",
		Re:   regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	},
	{
		Kind: "credit_card",
		Re:   regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13})\b`),
		Validate: LuhnCheck,
	},
	{
		Kind: "phone_us",
		Re:   regexp.MustCompile(`\b\+?1?[\-.\s]?\(?\d{3}\)?[\-.\s]\d{3}[\-.\s]\d{4}\b`),
	},
	{
		Kind: "aws_access_key",
		Re:   regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	},
	{
		Kind: "private_key",
		Re:   regexp.MustCompile(`-----BEGIN (RSA|EC|PGP|OPENSSH) PRIVATE KEY-----`),
	},
}

func LuhnCheck(s string) bool {
	var sum int
	var alternate bool
	
	// Remove non-digits
	digits := ""
	for _, r := range s {
		if r >= '0' && r <= '9' {
			digits += string(r)
		}
	}
	
	if len(digits) < 13 {
		return false
	}

	for i := len(digits) - 1; i >= 0; i-- {
		n, _ := strconv.Atoi(string(digits[i]))
		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alternate = !alternate
	}
	return sum%10 == 0
}

type Finding struct {
	Kind      string `json:"kind"`
	Value     string `json:"value,omitempty"`
	Location  string `json:"location,omitempty"`
	RiskScore float64 `json:"risk_score"`
}

func MaskValue(kind, value string) string {
	switch kind {
	case "credit_card":
		digits := ""
		for _, r := range value {
			if r >= '0' && r <= '9' {
				digits += string(r)
			}
		}
		if len(digits) >= 4 {
			return "****-****-****-" + digits[len(digits)-4:]
		}
		return "[REDACTED]"
	case "ssn":
		if len(value) >= 4 {
			return "***-**-" + value[len(value)-4:]
		}
		return "[REDACTED]"
	default:
		return "[REDACTED]"
	}
}

func ScanRegex(text string) []Finding {
	var findings []Finding
	for _, rule := range Rules {
		matches := rule.Re.FindAllString(text, -1)
		for _, m := range matches {
			if rule.Validate != nil && !rule.Validate(m) {
				continue
			}
			findings = append(findings, Finding{
				Kind:      rule.Kind,
				Value:     MaskValue(rule.Kind, m),
				RiskScore: 0.8,
			})
		}
	}
	return findings
}

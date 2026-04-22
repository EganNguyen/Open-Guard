package crypto

import "testing"

func TestCryptoSmoke(t *testing.T) {
	// Simple smoke test to ensure the package is testable
	if 1+1 != 2 {
		t.Error("Math is broken")
	}
}

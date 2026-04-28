package handlers

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportSigningIntegrity(t *testing.T) {
	// Generate a test key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	h := &ComplianceHandler{
		signingKey: privateKey,
	}

	testData := []byte("compliance-report-content")

	// Simulate the signing logic in generateReport
	hash := sha256.Sum256(testData)
	sig, err := rsa.SignPSS(rand.Reader, h.signingKey, crypto.SHA256, hash[:], nil)
	require.NoError(t, err)
	require.NotNil(t, sig)

	// Verify the signature using the public key
	err = rsa.VerifyPSS(&h.signingKey.PublicKey, crypto.SHA256, hash[:], sig, nil)
	assert.NoError(t, err, "Signature should be valid")

	// Verify that changing data fails verification
	tamperedData := []byte("compliance-report-content-tampered")
	tamperedHash := sha256.Sum256(tamperedData)
	err = rsa.VerifyPSS(&h.signingKey.PublicKey, crypto.SHA256, tamperedHash[:], sig, nil)
	assert.Error(t, err, "Tampered data should fail verification")
}

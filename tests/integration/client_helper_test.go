package integration

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
)

func NewMTLSClient(caCertPath string) (*http.Client, error) {
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA cert to pool")
	}

	tlsConfig := &tls.Config{
		RootCAs:            caCertPool,
		InsecureSkipVerify: true, // During integration tests, we often use localhost or service names that might not match CN exactly
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

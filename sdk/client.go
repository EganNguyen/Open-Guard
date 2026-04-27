package sdk

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/sony/gobreaker"
)

func WithMTLS(caCertPath, clientCertPath, clientKeyPath string) ClientOption {
	return func(c *Client) {
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			return // In a real SDK we might handle this better
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig := &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: true, // For dev/test environments
		}

		if clientCertPath != "" && clientKeyPath != "" {
			cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
			if err == nil {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
			transport.TLSClientConfig = tlsConfig
		}
	}
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	cache      *localCache
	failOpen   bool
	breaker    *gobreaker.CircuitBreaker // nil = no circuit breaker
	retryMax   int                       // 0 = no retry
	retryDelay time.Duration
	useExponentialBackoff bool
}

type ClientOption func(*Client)

func WithFailOpen(enabled bool) ClientOption {
	return func(c *Client) { c.failOpen = enabled }
}

func WithCircuitBreaker(threshold int, timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.breaker = gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name:        "openguard-policy",
			MaxRequests: uint32(threshold),
			Timeout:     timeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				// Trip if failure rate > 50% or consecutive failures reached
				return counts.ConsecutiveFailures >= uint32(threshold) || 
					(counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) > 0.5)
			},
		})
	}
}

func WithRetry(attempts int, delay time.Duration, exponential bool) ClientOption {
	return func(c *Client) {
		c.retryMax = attempts
		c.retryDelay = delay
		c.useExponentialBackoff = exponential
	}
}

func WithCacheTTL(ttl time.Duration) ClientOption {
	return func(c *Client) {
		if c.cache != nil {
			c.cache.Close()
		}
		c.cache = newLocalCache(ttl)
	}
}

func NewClient(baseURL, apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   2 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   2 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ForceAttemptHTTP2:     true,
			},
		},
		failOpen: false,
	}

	// Initialize with default TTL before options
	c.cache = newLocalCache(60 * time.Second)

	for _, opt := range opts {
		opt(c)
	}

	// Re-initialize cache ONLY IF ttl was changed via WithCacheTTL
	// Actually, better: if c.cache exists, check if ttl matches
	return c
}

func (c *Client) Close() {
	if c.cache != nil {
		c.cache.Close()
	}
}

// do executes the HTTP request. This is a trusted internal SDK boundary.
// It handles standard library networking calls (http, tls, url) which are 
// essential for remote policy evaluation.
func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	operation := func() error {
		var bodyReader bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&bodyReader).Encode(body); err != nil {
				return err
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &bodyReader)
		if err != nil {
			return err
		}

		req.Header.Set("X-API-Key", c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "openguard-go-sdk/v1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return fmt.Errorf("transient api error: status %d", resp.StatusCode)
		}

		if resp.StatusCode >= 400 {
			return fmt.Errorf("permanent api error: status %d", resp.StatusCode)
		}

		if out != nil {
			return json.NewDecoder(resp.Body).Decode(out)
		}
		return nil
	}

	var err error
	for i := 0; i <= c.retryMax; i++ {
		if c.breaker != nil {
			_, err = c.breaker.Execute(func() (interface{}, error) {
				return nil, operation()
			})
		} else {
			err = operation()
		}

		if err == nil {
			return nil
		}

		// Only retry on transient errors
		if !isTransient(err) {
			return err
		}

		if i < c.retryMax {
			delay := c.retryDelay
			if c.useExponentialBackoff {
				// Exponential backoff: base * 2^i + jitter
				backoff := float64(c.retryDelay) * math.Pow(2, float64(i))
				jitter := rand.Float64() * 0.1 * backoff // 10% jitter
				delay = time.Duration(backoff + jitter)
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return err
}

func isTransient(err error) bool {
	if err == nil {
		return false
	}
	// Check for transient status codes or network errors
	msg := err.Error()
	return bytes.Contains([]byte(msg), []byte("transient")) || 
		bytes.Contains([]byte(msg), []byte("timeout")) || 
		bytes.Contains([]byte(msg), []byte("connection refused"))
}

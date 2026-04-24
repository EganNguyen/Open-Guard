package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	cache      *localCache
	failOpen   bool
	breaker    *gobreaker.CircuitBreaker // nil = no circuit breaker
	retryMax   int                       // 0 = no retry
	retryDelay time.Duration
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
				return counts.ConsecutiveFailures >= uint32(threshold)
			},
		})
	}
}

func WithRetry(attempts int, delay time.Duration) ClientOption {
	return func(c *Client) {
		c.retryMax = attempts
		c.retryDelay = delay
	}
}

func WithCacheTTL(ttl time.Duration) ClientOption {
	return func(c *Client) {
		c.cache.ttl = ttl
	}
}

func NewClient(baseURL, apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 5 * time.Second},
		cache:      newLocalCache(60 * time.Second), // R-15 requirement
		failOpen:   false,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) Close() {
	if c.cache != nil {
		c.cache.Close()
	}
}

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

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("api error: status %d", resp.StatusCode)
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

		if i < c.retryMax {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryDelay):
			}
		}
	}

	return err
}

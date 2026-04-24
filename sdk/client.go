package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	cache      *localCache
	failOpen   bool
}

type ClientOption func(*Client)

func WithFailOpen(enabled bool) ClientOption {
	return func(c *Client) { c.failOpen = enabled }
}

func WithCircuitBreaker(threshold int, timeout time.Duration) ClientOption {
	return func(c *Client) {
		// Implementation will be added in Phase 3
	}
}

func WithRetry(attempts int, delay time.Duration) ClientOption {
	return func(c *Client) {
		// Implementation will be added in Phase 3
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

func (c *Client) do(ctx context.Context, method, path string, body interface{}, out interface{}) error {
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

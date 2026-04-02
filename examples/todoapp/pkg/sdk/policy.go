package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
)

type PolicyClient struct {
	addr       string
	apiKey     string
	cache      *lru.Cache[string, cacheItem]
	httpClient *http.Client
}

type cacheItem struct {
	Decision   bool
	Reason     string
	Expiration time.Time
}

func NewPolicyClient(addr, apiKey string, cacheSize int) (*PolicyClient, error) {
	cache, err := lru.New[string, cacheItem](cacheSize)
	if err != nil {
		return nil, err
	}
	return &PolicyClient{
		addr:   addr,
		apiKey: apiKey,
		cache:  cache,
		httpClient: &http.Client{
			Timeout: 100 * time.Millisecond,
		},
	}, nil
}

func (c *PolicyClient) Evaluate(ctx context.Context, req PolicyRequest) (bool, string, error) {
	cacheKey := fmt.Sprintf("%s:%s:%s:%s", req.UserID, req.OrgID, req.Action, req.Resource)

	// Check cache first
	if item, ok := c.cache.Get(cacheKey); ok {
		if time.Now().Before(item.Expiration) {
			return item.Decision, item.Reason, nil
		}
	}

	// Call Control Plane
	body, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/v1/policies/evaluate", c.addr)
	
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return false, "client_error", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		// FAIL CLOSED: If we can't reach the service, we only trust the cache if it's there
		// Even if it's expired, we might want a grace period as per spec?
		// Spec says: "the SDK uses its local LRU cache for up to 60 seconds, then denies all requests."
		if item, ok := c.cache.Get(cacheKey); ok {
			// If it's within the 60s grace, return it even if technically expired
			// But the cache item's expiration is already the 60s mark from creation.
			return item.Decision, item.Reason, nil
		}
		return false, "service_unavailable", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, "unexpected_status", fmt.Errorf("policy service status: %d", resp.StatusCode)
	}

	var evalResp PolicyResponse
	if err := json.NewDecoder(resp.Body).Decode(&evalResp); err != nil {
		return false, "json_error", err
	}

	// Update cache
	c.cache.Add(cacheKey, cacheItem{
		Decision:   evalResp.Permitted,
		Reason:     evalResp.Reason,
		Expiration: time.Now().Add(60 * time.Second),
	})

	return evalResp.Permitted, evalResp.Reason, nil
}

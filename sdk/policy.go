package sdk

import (
	"context"
	"fmt"
)

type EvaluationRequest struct {
	SubjectID string `json:"subject_id"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
}

type EvaluationResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

	func (c *Client) Allow(ctx context.Context, subjectID, action, resource string) (bool, error) {
		cacheKey := fmt.Sprintf("%s:%s:%s:%s", c.orgID, subjectID, action, resource)
	
		// Check cache — including grace period (stale-while-unavailable)
	if val, ok := c.cache.GetOrStale(cacheKey); ok {
		return val, nil
	}

	req := EvaluationRequest{
		SubjectID: subjectID,
		Action:    action,
		Resource:  resource,
	}

	var resp EvaluationResponse
	// Path /v1/policy/evaluate per spec §10
	err := c.do(ctx, "POST", "/v1/policy/evaluate", req, &resp)
	if err != nil {
		// Policy service unavailable — apply fail-closed behavior
		if c.failOpen {
			return true, nil // fail-open (dev mode)
		}
		return false, nil // fail-closed (production default) — deny, no error
	}

	c.cache.Set(cacheKey, resp.Allowed)
	return resp.Allowed, nil
}

package service

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/openguard/shared/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestApplyPolicy_Branches(t *testing.T) {
	// Bad JSON
	m, _, _ := applyPolicy(EvalRequest{}, &models.Policy{Rules: []byte("{bad")})
	assert.False(t, m)

	// Unknown Type
	m, _, _ = applyPolicy(EvalRequest{}, &models.Policy{Type: "unknown", Rules: []byte("{}")})
	assert.False(t, m)

	// Session Limit
	m, _, _ = applyPolicy(EvalRequest{}, &models.Policy{Type: "session_limit", Rules: []byte("{}")})
	assert.False(t, m)
}

func TestEvaluatorService_Evaluate_AsyncSettle(t *testing.T) {
	// Test the permitted branch, and sleep to allow the async logging to register coverage
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:65535"})

	repo := &evalMockRepo{p: []*models.Policy{
		{ID: "p2", Enabled: true, Type: "ip_allowlist", Rules: []byte(`{"allowed_ips":["10.0.0.1"]}`)},
	}}
	svc := New(repo, rdb, 30, logger)

	req := EvalRequest{OrgID: "org-1", IPAddress: "10.0.0.1"}
	resp, err := svc.Evaluate(context.Background(), req)
	
	assert.NoError(t, err)
	assert.True(t, resp.Permitted)

	// Sleep briefly so the go func() logging block executes and counts towards coverage
	time.Sleep(50 * time.Millisecond)
}

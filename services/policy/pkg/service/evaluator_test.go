package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/openguard/policy/pkg/repository"
	"github.com/openguard/shared/models"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestApplyIPAllowlist(t *testing.T) {
	rules := map[string]interface{}{
		"allowed_ips": []interface{}{"192.168.1.1", "10.0.0.1"},
	}

	tests := []struct {
		name      string
		ip        string
		matched   bool
		deny      bool
		expReason string
	}{
		{
			name:      "IP in allowlist",
			ip:        "192.168.1.1",
			matched:   true,
			deny:      false,
			expReason: "ip in allowlist",
		},
		{
			name:      "IP not in allowlist",
			ip:        "192.168.1.2",
			matched:   true,
			deny:      true,
			expReason: `ip "192.168.1.2" not in allowlist`,
		},
		{
			name:      "Empty IP in request",
			ip:        "",
			matched:   true,
			deny:      true,
			expReason: `ip "" not in allowlist`,
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, deny, reason := applyIPAllowlist(logger, EvalRequest{IPAddress: tt.ip}, rules)
			if matched != tt.matched || deny != tt.deny || reason != tt.expReason {
				t.Errorf("got (%v, %v, %q), want (%v, %v, %q)", matched, deny, reason, tt.matched, tt.deny, tt.expReason)
			}
		})
	}
}

func TestApplyDataExportPolicy(t *testing.T) {
	rules := map[string]interface{}{
		"allowed_roles": []interface{}{"admin", "compliance"},
	}

	tests := []struct {
		name      string
		action    string
		groups    []string
		matched   bool
		deny      bool
		expReason string
	}{
		{
			name:      "Permitted action and role",
			action:    "data.export",
			groups:    []string{"admin", "user"},
			matched:   true,
			deny:      false,
			expReason: "role permitted for export",
		},
		{
			name:      "Permitted action but wrong role",
			action:    "data.export",
			groups:    []string{"user"},
			matched:   true,
			deny:      true,
			expReason: "data export not permitted for user's roles",
		},
		{
			name:      "Different action",
			action:    "data.read",
			groups:    []string{"admin"},
			matched:   false,
			deny:      false,
			expReason: "",
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, deny, reason := applyDataExportPolicy(logger, EvalRequest{Action: tt.action, UserGroups: tt.groups}, rules)
			if matched != tt.matched || deny != tt.deny || reason != tt.expReason {
				t.Errorf("got (%v, %v, %q), want (%v, %v, %q)", matched, deny, reason, tt.matched, tt.deny, tt.expReason)
			}
		})
	}
}

func TestApplyAnonAccessPolicy(t *testing.T) {
	rulesAllow := map[string]interface{}{"allow_anonymous": true}
	rulesDeny := map[string]interface{}{"allow_anonymous": false}

	tests := []struct {
		name      string
		userID    string
		rules     map[string]interface{}
		matched   bool
		deny      bool
		expReason string
	}{
		{
			name:      "Authenticated user, policy allows anon",
			userID:    "user-123",
			rules:     rulesAllow,
			matched:   false,
			deny:      false,
			expReason: "",
		},
		{
			name:      "Anonymous user, policy allows anon",
			userID:    "",
			rules:     rulesAllow,
			matched:   false,
			deny:      false,
			expReason: "",
		},
		{
			name:      "Anonymous user, policy denies anon",
			userID:    "",
			rules:     rulesDeny,
			matched:   true,
			deny:      true,
			expReason: "anonymous access not permitted",
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, deny, reason := applyAnonAccessPolicy(logger, EvalRequest{UserID: tt.userID}, tt.rules)
			if matched != tt.matched || deny != tt.deny || reason != tt.expReason {
				t.Errorf("got (%v, %v, %q), want (%v, %v, %q)", matched, deny, reason, tt.matched, tt.deny, tt.expReason)
			}
		})
	}
}

func TestApplyRBAC(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rules := map[string]interface{}{
		"allowed_roles": []interface{}{"admin", "editor"},
	}

	tests := []struct {
		name    string
		groups  []string
		matched bool
		deny    bool
	}{
		{
			name:    "user has permitted role",
			groups:  []string{"editor", "viewer"},
			matched: true,
			deny:    false,
		},
		{
			name:    "user has no permitted role",
			groups:  []string{"viewer"},
			matched: true,
			deny:    true,
		},
		{
			name:    "user has no groups",
			groups:  []string{},
			matched: true,
			deny:    true,
		},
		{
			name:    "user has the admin role",
			groups:  []string{"admin"},
			matched: true,
			deny:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, deny, _ := applyRBAC(logger, EvalRequest{UserID: "u1", UserGroups: tt.groups}, rules)
			if matched != tt.matched || deny != tt.deny {
				t.Errorf("got (matched=%v, deny=%v), want (matched=%v, deny=%v)",
					matched, deny, tt.matched, tt.deny)
			}
		})
	}

	t.Run("empty allowed_roles is a no-op", func(t *testing.T) {
		emptyRules := map[string]interface{}{"allowed_roles": []interface{}{}}
		matched, deny, _ := applyRBAC(logger, EvalRequest{UserGroups: []string{"admin"}}, emptyRules)
		if matched || deny {
			t.Error("expected no-op (false, false) for empty allowed_roles")
		}
	})

	t.Run("missing allowed_roles key is a no-op", func(t *testing.T) {
		matched, deny, _ := applyRBAC(logger, EvalRequest{UserGroups: []string{"admin"}}, map[string]interface{}{})
		if matched || deny {
			t.Error("expected no-op (false, false) when allowed_roles key is missing")
		}
	})
}

func TestEvaluateLogic(t *testing.T) {
	svc := &Service{}

	ipRules, _ := json.Marshal(map[string]interface{}{
		"allowed_ips": []interface{}{"1.1.1.1"},
	})
	
	policies := []*models.Policy{
		{
			ID:      "p1",
			Enabled: true,
			Type:    models.PolicyTypeIPAllowlist,
			Rules:   ipRules,
		},
	}

	t.Run("permitted by IP", func(t *testing.T) {
		req := EvalRequest{IPAddress: "1.1.1.1"}
		resp := svc.evaluate(req, policies)
		if !resp.Permitted {
			t.Errorf("expected permitted, got denied: %s", resp.Reason)
		}
		if len(resp.MatchedPolicies) != 1 || resp.MatchedPolicies[0] != "p1" {
			t.Errorf("unexpected matched policies: %v", resp.MatchedPolicies)
		}
	})

	t.Run("denied by IP", func(t *testing.T) {
		req := EvalRequest{IPAddress: "2.2.2.2"}
		resp := svc.evaluate(req, policies)
		if resp.Permitted {
			t.Error("expected denied, got permitted")
		}
		if resp.Reason != `ip "2.2.2.2" not in allowlist` {
			t.Errorf("unexpected reason: %s", resp.Reason)
		}
	})

	t.Run("no policies configured - implicit allow", func(t *testing.T) {
		resp := svc.evaluate(EvalRequest{}, []*models.Policy{})
		if !resp.Permitted {
			t.Error("expected permitted for empty policies")
		}
	})
}

func TestEvalCacheKey(t *testing.T) {
	req1 := EvalRequest{
		OrgID:      "org1",
		UserID:     "user1",
		Action:     "read",
		Resource:   "file",
		UserGroups: []string{"g1", "g2"},
	}

	req2 := EvalRequest{
		OrgID:      "org1",
		UserID:     "user1",
		Action:     "read",
		Resource:   "file",
		UserGroups: []string{"g1", "g2"},
	}

	key1 := evalCacheKey(req1)
	key2 := evalCacheKey(req2)

	if key1 != key2 {
		t.Errorf("expected identical keys, got %q and %q", key1, key2)
	}

	req3 := req1
	req3.Action = "write"
	key3 := evalCacheKey(req3)
	if key1 == key3 {
		t.Error("expected different keys for different actions")
	}
}

type evalMockRepo struct {
	listErr error
	p       []*models.Policy
}

func (m *evalMockRepo) Create(ctx context.Context, p *models.Policy) error             { return nil }
func (m *evalMockRepo) GetByID(ctx context.Context, o, p string) (*models.Policy, error) { return nil, nil }
func (m *evalMockRepo) ListByOrg(ctx context.Context, o string) ([]*models.Policy, error) { return nil, nil }
func (m *evalMockRepo) ListEnabledForOrg(ctx context.Context, o string) ([]*models.Policy, error) {
	return m.p, m.listErr
}
func (m *evalMockRepo) Update(ctx context.Context, p *models.Policy) error           { return nil }
func (m *evalMockRepo) Delete(ctx context.Context, o, p string) error                { return nil }
func (m *evalMockRepo) LogEvaluation(ctx context.Context, l *repository.EvalLog) error { return nil }

func TestEvaluatorService_Evaluate(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	// Pass a bad redis client so we bypass cache aggressively
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:65535", DialTimeout: 100 * time.Millisecond})

	t.Run("db error fails closed", func(t *testing.T) {
		repo := &evalMockRepo{listErr: errors.New("db error")}
		svc := New(repo, rdb, 30, logger)

		req := EvalRequest{OrgID: "org-1", Action: "read"}
		resp, err := svc.Evaluate(context.Background(), req)
		assert.NoError(t, err) // Evaluate itself swallows and fails closed
		assert.False(t, resp.Permitted)
		assert.Contains(t, resp.Reason, "fail closed")
	})

	t.Run("no policies configured allows all", func(t *testing.T) {
		repo := &evalMockRepo{p: []*models.Policy{}}
		svc := New(repo, rdb, 30, logger)

		req := EvalRequest{OrgID: "org-1", Action: "read"}
		resp, err := svc.Evaluate(context.Background(), req)
		assert.NoError(t, err)
		assert.True(t, resp.Permitted)
	})

	t.Run("policy denies request", func(t *testing.T) {
		repo := &evalMockRepo{p: []*models.Policy{
			{ID: "p1", Enabled: true, Type: "ip_allowlist", Rules: []byte(`{"allowed_ips":["10.0.0.1"]}`)},
		}}
		svc := New(repo, rdb, 30, logger)

		req := EvalRequest{OrgID: "org-1", IPAddress: "192.168.1.1"}
		resp, err := svc.Evaluate(context.Background(), req)
		assert.NoError(t, err)
		assert.False(t, resp.Permitted)
		assert.Contains(t, resp.MatchedPolicies, "p1")
		
		time.Sleep(50 * time.Millisecond)
	})
}

func TestEvaluatorService_InvalidateCache(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:65535", DialTimeout: 10 * time.Millisecond})
	svc := New(&evalMockRepo{}, rdb, 30, logger)

	err := svc.InvalidateCacheForOrg(context.Background(), "org-1")
	// Since Redis is offline, SMEMBERS returns an error
	assert.ErrorContains(t, err, "redis smembers")
}

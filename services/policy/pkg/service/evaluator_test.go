package service

import (
	"encoding/json"
	"testing"

	"github.com/openguard/shared/models"
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, deny, reason := applyIPAllowlist(EvalRequest{IPAddress: tt.ip}, rules)
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, deny, reason := applyDataExportPolicy(EvalRequest{Action: tt.action, UserGroups: tt.groups}, rules)
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, deny, reason := applyAnonAccessPolicy(EvalRequest{UserID: tt.userID}, tt.rules)
			if matched != tt.matched || deny != tt.deny || reason != tt.expReason {
				t.Errorf("got (%v, %v, %q), want (%v, %v, %q)", matched, deny, reason, tt.matched, tt.deny, tt.expReason)
			}
		})
	}
}

func TestEvaluateLogic(t *testing.T) {
	svc := &EvaluatorService{}

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

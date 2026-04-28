package service_test

import (
	"encoding/json"
	"log/slog"
	"os"
	"testing"

	"github.com/openguard/services/policy/pkg/repository"
	"github.com/openguard/services/policy/pkg/service"
)

func TestEvaluate_CEL(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	s := service.NewService(nil, nil, nil, logger)

	tests := []struct {
		name       string
		req        service.EvaluateRequest
		expression string
		want       string
	}{
		{
			name:       "CEL Simple Match",
			req:        service.EvaluateRequest{SubjectID: "user:admin", Action: "read", Resource: "doc1"},
			expression: "subject == 'user:admin'",
			want:       "allow",
		},
		{
			name:       "CEL StartsWith",
			req:        service.EvaluateRequest{SubjectID: "user:admin", Action: "read", Resource: "doc1"},
			expression: "subject.startsWith('user:')",
			want:       "allow",
		},
		{
			name:       "CEL List Contains",
			req:        service.EvaluateRequest{SubjectID: "user1", UserGroups: []string{"admin", "editor"}, Action: "write", Resource: "doc1"},
			expression: "'admin' in user_groups",
			want:       "allow",
		},
		{
			name:       "CEL Complex Logic",
			req:        service.EvaluateRequest{SubjectID: "user1", Action: "delete", Resource: "prod:db"},
			expression: "subject == 'user1' && resource.startsWith('prod:') && action == 'delete'",
			want:       "allow",
		},
		{
			name:       "CEL No Match",
			req:        service.EvaluateRequest{SubjectID: "user1", Action: "read", Resource: "doc1"},
			expression: "subject == 'user2'",
			want:       "deny",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policies := []repository.Policy{
				{
					ID:    "p1",
					Logic: json.RawMessage(`{"type":"cel","expression":"` + tt.expression + `"}`),
				},
			}
			effect, _, _ := s.EvaluateInternal(tt.req, policies)
			if effect != tt.want {
				t.Errorf("%s: expected %s, got %s", tt.name, tt.want, effect)
			}
		})
	}
}

func TestEvaluate_AllowOnRBACMatch(t *testing.T) {
	s := &service.Service{}
	req := service.EvaluateRequest{SubjectID: "user1", Action: "read", Resource: "doc1"}
	policies := []repository.Policy{
		{
			ID:    "p1",
			Logic: json.RawMessage(`{"type":"rbac","subjects":["user1"],"actions":["read"],"resources":["doc1"]}`),
		},
	}
	effect, matchedIDs, _ := service.EvaluateInternal(s, req, policies)
	if effect != "allow" {
		t.Errorf("expected allow, got %s", effect)
	}
	if len(matchedIDs) != 1 || matchedIDs[0] != "p1" {
		t.Errorf("expected matched ID p1, got %v", matchedIDs)
	}
}

func TestEvaluate_DenyAllOverridesAllow(t *testing.T) {
	s := &service.Service{}
	req := service.EvaluateRequest{SubjectID: "user1", Action: "read", Resource: "doc1"}
	policies := []repository.Policy{
		{ID: "p1", Logic: json.RawMessage(`{"type":"rbac","subjects":["*"],"actions":["*"],"resources":["*"]}`)},
		{ID: "p2", Logic: json.RawMessage(`{"type":"deny_all"}`)},
	}
	effect, _, _ := service.EvaluateInternal(s, req, policies)
	if effect != "deny" {
		t.Errorf("expected deny (override), got %s", effect)
	}
}

func TestEvaluate_DefaultDenyWithNoPolicies(t *testing.T) {
	s := &service.Service{}
	req := service.EvaluateRequest{SubjectID: "user1", Action: "read", Resource: "doc1"}
	policies := []repository.Policy{}
	effect, _, _ := service.EvaluateInternal(s, req, policies)
	if effect != "deny" {
		t.Errorf("expected default deny, got %s", effect)
	}
}

func TestEvaluate_WildcardSubject(t *testing.T) {
	s := &service.Service{}
	req := service.EvaluateRequest{SubjectID: "user:123", Action: "read", Resource: "doc1"}
	policies := []repository.Policy{
		{ID: "p1", Logic: json.RawMessage(`{"type":"rbac","subjects":["user:*"],"actions":["read"],"resources":["*"]}`)},
	}
	effect, _, _ := service.EvaluateInternal(s, req, policies)
	if effect != "allow" {
		t.Errorf("expected allow via wildcard, got %s", effect)
	}
}

func TestEvaluate_GlobActionPrefix(t *testing.T) {
	s := &service.Service{}
	req := service.EvaluateRequest{SubjectID: "user1", Action: "read:all", Resource: "doc1"}
	policies := []repository.Policy{
		{ID: "p1", Logic: json.RawMessage(`{"type":"rbac","subjects":["user1"],"actions":["read:*"],"resources":["*"]}`)},
	}
	effect, _, _ := service.EvaluateInternal(s, req, policies)
	if effect != "allow" {
		t.Errorf("expected allow via action glob, got %s", effect)
	}
}

// Table-driven test for matchesGlob:
var globTests = []struct {
	patterns []string
	value    string
	want     bool
}{
	{[]string{"read:*"}, "read:documents", true},
	{[]string{"read:*"}, "write:documents", false},
	{[]string{"*"}, "anything", true},
	{[]string{"read:documents"}, "read:documents", true},
	{[]string{"read:documents"}, "read:documents:extra", false},
	{[]string{"user:*"}, "user:", true}, // boundary: prefix with empty suffix
	{[]string{}, "read:docs", false},    // empty patterns → no match
}

func TestMatchesGlob(t *testing.T) {
	for _, tt := range globTests {
		got := service.MatchesGlob(tt.patterns, tt.value)
		if got != tt.want {
			t.Errorf("MatchesGlob(%v, %q) = %v, want %v", tt.patterns, tt.value, got, tt.want)
		}
	}
}

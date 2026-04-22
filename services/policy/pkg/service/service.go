package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/openguard/services/policy/pkg/repository"
	"github.com/openguard/shared/kafka/outbox"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	cachePrefix   = "policy:eval:"
	cacheTTL      = 30 * time.Second // stale-while-revalidate window
	maxRetryDelay = 5 * time.Second
)

// EvaluateRequest is the input to policy evaluation.
type EvaluateRequest struct {
	OrgID     string `json:"org_id"`
	SubjectID string `json:"subject_id"`
	Action    string `json:"action"`
	Resource  string `json:"resource"`
}

// EvaluateResponse is the output of policy evaluation.
type EvaluateResponse struct {
	Effect           string   `json:"effect"` // "allow" or "deny"
	MatchedPolicyIDs []string `json:"matched_policy_ids"`
	CacheHit         bool     `json:"cache_hit"`
	LatencyMs        int      `json:"latency_ms"`
}

// cachedDecision is what we store in Redis.
type cachedDecision struct {
	Effect           string   `json:"effect"`
	MatchedPolicyIDs []string `json:"matched_policy_ids"`
}

// PolicyLogic is the JSONB structure stored in policies.logic
// It supports two rule types:
//
//	{ "type": "rbac", "subjects": ["user:*"], "actions": ["read:*"], "resources": ["document:*"] }
//	{ "type": "deny_all" }
//	{ "type": "allow_all" }
type PolicyLogic struct {
	Type      string   `json:"type"`
	Subjects  []string `json:"subjects,omitempty"`
	Actions   []string `json:"actions,omitempty"`
	Resources []string `json:"resources,omitempty"`
}

// Service implements the policy engine business logic.
type Service struct {
	repo        *repository.Repository
	rdb         *redis.Client
	outboxWriter *outbox.Writer
	sfGroup     singleflight.Group
	logger      *slog.Logger
}

// NewService creates a new policy service.
func NewService(repo *repository.Repository, rdb *redis.Client, outboxWriter *outbox.Writer, logger *slog.Logger) *Service {
	return &Service{
		repo:        repo,
		rdb:         rdb,
		outboxWriter: outboxWriter,
		logger:      logger,
	}
}

// cacheKey builds the Redis key for a given evaluation request.
// Uses sha256 of "org_id:subject_id:action:resource" per spec §11.3.
func cacheKey(req EvaluateRequest) string {
	raw := fmt.Sprintf("%s:%s:%s:%s", req.OrgID, req.SubjectID, req.Action, req.Resource)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%s%x", cachePrefix, sum)
}

// Evaluate runs the policy decision engine with two-tier caching and singleflight.
func (s *Service) Evaluate(ctx context.Context, req EvaluateRequest) (*EvaluateResponse, error) {
	start := time.Now()
	key := cacheKey(req)

	// Tier 1: Redis cache check
	if s.rdb != nil {
		if cached, err := s.rdb.Get(ctx, key).Bytes(); err == nil {
			var decision cachedDecision
			if json.Unmarshal(cached, &decision) == nil {
				latency := int(time.Since(start).Milliseconds())
				// Async background refresh (stale-while-revalidate)
				go s.backgroundRefresh(req, key)
				return &EvaluateResponse{
					Effect:           decision.Effect,
					MatchedPolicyIDs: decision.MatchedPolicyIDs,
					CacheHit:         true,
					LatencyMs:        latency,
				}, nil
			}
		}
	}

	// Tier 2: Postgres evaluation with singleflight (one DB query per unique cache key)
	type sfResult struct {
		resp *EvaluateResponse
		err  error
	}
	ch := s.sfGroup.DoChan(key, func() (interface{}, error) {
		resp, err := s.evaluateFromDB(ctx, req)
		return &sfResult{resp: resp, err: err}, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		r := res.Val.(*sfResult)
		if r.err != nil {
			return nil, r.err
		}
		r.resp.LatencyMs = int(time.Since(start).Milliseconds())

		// Write to eval log (non-blocking)
		go s.writeEvalLog(req, r.resp)

		return r.resp, nil
	}
}

// evaluateFromDB fetches policies from Postgres and evaluates them.
func (s *Service) evaluateFromDB(ctx context.Context, req EvaluateRequest) (*EvaluateResponse, error) {
	policies, err := s.repo.GetMatchingPolicies(ctx, req.OrgID)
	if err != nil {
		// Fail-closed: if we can't read policies, deny
		s.logger.Error("failed to fetch policies, failing closed", "error", err, "org_id", req.OrgID)
		return &EvaluateResponse{
			Effect:           "deny",
			MatchedPolicyIDs: []string{},
			CacheHit:         false,
		}, nil
	}

	effect, matchedIDs := s.evaluate(req, policies)

	resp := &EvaluateResponse{
		Effect:           effect,
		MatchedPolicyIDs: matchedIDs,
		CacheHit:         false,
	}

	// Cache the result in Redis
	if s.rdb != nil {
		decision := cachedDecision{Effect: effect, MatchedPolicyIDs: matchedIDs}
		if b, err := json.Marshal(decision); err == nil {
			s.rdb.Set(ctx, cacheKey(req), b, cacheTTL)
		}
	}

	return resp, nil
}

// evaluate applies RBAC logic against the policy set.
// Default is DENY — all requests are denied unless explicitly allowed.
// An explicit deny overrides any allow.
func (s *Service) evaluate(req EvaluateRequest, policies []repository.Policy) (string, []string) {
	var matchedIDs []string
	effect := "deny"
	hasExplicitAllow := false
	hasExplicitDeny := false

	for _, p := range policies {
		var logic PolicyLogic
		if err := json.Unmarshal(p.Logic, &logic); err != nil {
			s.logger.Warn("failed to parse policy logic, skipping", "policy_id", p.ID, "error", err)
			continue
		}

		switch logic.Type {
		case "deny_all":
			matchedIDs = append(matchedIDs, p.ID)
			hasExplicitDeny = true

		case "allow_all":
			matchedIDs = append(matchedIDs, p.ID)
			hasExplicitAllow = true

		case "rbac":
			subjectMatch := matchesGlob(logic.Subjects, req.SubjectID)
			actionMatch := matchesGlob(logic.Actions, req.Action)
			resourceMatch := matchesGlob(logic.Resources, req.Resource)

			if subjectMatch && actionMatch && resourceMatch {
				matchedIDs = append(matchedIDs, p.ID)
				hasExplicitAllow = true
			}
		}
	}

	// Explicit deny overrides any allow
	if hasExplicitDeny {
		return "deny", matchedIDs
	}
	if hasExplicitAllow {
		effect = "allow"
	}

	return effect, matchedIDs
}

// matchesGlob checks if value matches any pattern in patterns.
// Supports "*" wildcard suffix (e.g. "read:*" matches "read:documents").
func matchesGlob(patterns []string, value string) bool {
	for _, p := range patterns {
		if p == "*" || p == value {
			return true
		}
		if strings.HasSuffix(p, ":*") {
			prefix := strings.TrimSuffix(p, ":*")
			if strings.HasPrefix(value, prefix+":") || value == prefix {
				return true
			}
		}
		if strings.HasSuffix(p, "*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(value, prefix) {
				return true
			}
		}
	}
	return false
}

// backgroundRefresh asynchronously refreshes the cache after a cache hit.
func (s *Service) backgroundRefresh(req EvaluateRequest, key string) {
	ctx, cancel := context.WithTimeout(context.Background(), maxRetryDelay)
	defer cancel()
	s.evaluateFromDB(ctx, req)
}

// writeEvalLog writes a policy evaluation log entry asynchronously.
func (s *Service) writeEvalLog(req EvaluateRequest, resp *EvaluateResponse) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repo.WriteEvalLog(ctx, repository.EvalLog{
		OrgID:            req.OrgID,
		SubjectID:        req.SubjectID,
		Action:           req.Action,
		Resource:         req.Resource,
		Effect:           resp.Effect,
		MatchedPolicyIDs: resp.MatchedPolicyIDs,
		CacheHit:         resp.CacheHit,
		LatencyMs:        resp.LatencyMs,
	}); err != nil {
		s.logger.Error("failed to write eval log", "error", err)
	}
}

// PublishPolicyChange writes a policy.changes event to the outbox via the provided transaction.
// Call this within a policy mutation transaction so the event is published atomically.
// The outboxWriter field is used here for future wiring — currently a no-op stub.
func (s *Service) PublishPolicyChange(orgID, policyID, eventType string) {
	// TODO: wire to outbox.WriteTx within mutation transactions (Phase 3.1)
	// This requires the mutation to pass a pgx.Tx down to the service layer.
	_ = s.outboxWriter
}

// InvalidateCache removes the cached evaluation for all policy keys for an org.
// This is a best-effort operation triggered on policy.changes Kafka events.
func (s *Service) InvalidateOrgCache(ctx context.Context, orgID string) error {
	if s.rdb == nil {
		return nil
	}
	// Scan and delete all keys matching the org prefix
	// Note: In production use SCAN instead of KEYS for large keyspaces
	pattern := fmt.Sprintf("%s*", cachePrefix)
	var cursor uint64
	for {
		keys, nextCursor, err := s.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return fmt.Errorf("scan cache keys: %w", err)
		}
		if len(keys) > 0 {
			s.rdb.Del(ctx, keys...)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	return nil
}

package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/jackc/pgx/v5"
	"github.com/openguard/services/policy/pkg/repository"
	"github.com/openguard/services/policy/pkg/telemetry"
	"github.com/openguard/shared/kafka/outbox"
	"github.com/openguard/shared/resilience"
	"github.com/openguard/shared/rls"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
	"golang.org/x/sync/singleflight"
)

const (
	cachePrefix   = "policy:eval:"
	cacheTTL      = 60 * time.Second // total cache lifetime
	staleWindow   = 5 * time.Second  // stale-while-revalidate grace period per spec §11.2
	maxRetryDelay = 5 * time.Second
)

// EvaluateRequest is the input to policy evaluation.
type EvaluateRequest struct {
	OrgID      string   `json:"org_id"`
	SubjectID  string   `json:"subject_id"`
	UserGroups []string `json:"user_groups"`
	Action     string   `json:"action"`
	Resource   string   `json:"resource"`
}

// EvaluateResponse is the output of policy evaluation.
type EvaluateResponse struct {
	Effect           string   `json:"effect"` // "allow" or "deny"
	MatchedPolicyIDs []string `json:"matched_policy_ids"`
	MaxVersion       int      `json:"max_version"`
	CacheHit         string   `json:"cache_hit"` // "none", "redis", "sdk"
	LatencyMs        int      `json:"latency_ms"`
}

// cachedDecision is what we store in Redis.
type cachedDecision struct {
	Effect           string    `json:"effect"`
	MatchedPolicyIDs []string  `json:"matched_policy_ids"`
	MaxVersion       int       `json:"max_version"`
	ExpiresAt        time.Time `json:"expires_at"` // Used for Stale-While-Revalidate
}

// PolicyLogic is the JSONB structure stored in policies.logic
// It supports three rule types:
//
//	{ "type": "rbac", "subjects": ["user:*"], "actions": ["read:*"], "resources": ["document:*"] }
//	{ "type": "cel", "expression": "subject.startsWith('user:admin') && resource.contains('confidential')" }
//	{ "type": "deny_all" }
//	{ "type": "allow_all" }
type PolicyLogic struct {
	Type       string   `json:"type"`
	Subjects   []string `json:"subjects,omitempty"`
	Actions    []string `json:"actions,omitempty"`
	Resources  []string `json:"resources,omitempty"`
	Expression string   `json:"expression,omitempty"`
}

type evalLogEntry struct {
	req  EvaluateRequest
	resp *EvaluateResponse
}

// Service implements the policy engine business logic.
type Service struct {
	repo         PolicyRepository
	rdb          *redis.Client
	outboxWriter *outbox.Writer
	sfGroup      singleflight.Group
	logger       *slog.Logger
	dbBreaker    *gobreaker.CircuitBreaker
	refreshSem   chan struct{}
	logCh        chan evalLogEntry
	celEnv       *cel.Env
}

// NewService creates a new policy service.
func NewService(repo PolicyRepository, rdb *redis.Client, outboxWriter *outbox.Writer, logger *slog.Logger) *Service {
	env, err := cel.NewEnv(
		cel.Variable("subject", cel.StringType),
		cel.Variable("action", cel.StringType),
		cel.Variable("resource", cel.StringType),
		cel.Variable("user_groups", cel.ListType(cel.StringType)),
	)
	if err != nil {
		logger.Error("failed to initialize CEL environment", "error", err)
	}

	s := &Service{
		repo:         repo,
		rdb:          rdb,
		outboxWriter: outboxWriter,
		logger:       logger,
		celEnv:       env,
		dbBreaker: resilience.NewBreaker(resilience.BreakerConfig{
			Name:             "policy-db",
			MaxRequests:      5,
			Interval:         10 * time.Second,
			FailureThreshold: 10,
			OpenDuration:     30 * time.Second,
		}, logger),
		refreshSem: make(chan struct{}, 100),      // max 100 concurrent background refreshes
		logCh:      make(chan evalLogEntry, 1000), // drop logs if buffer full
	}

	// Start background log worker
	go s.logWorker()

	return s
}

func (s *Service) logWorker() {
	for entry := range s.logCh {
		s.processWriteEvalLog(entry.req, entry.resp)
	}
}

// cacheKey builds the Redis key for a given evaluation request.
// Uses sha256 of "org_id:subject_id:action:resource" per spec §11.3.
func cacheKey(req EvaluateRequest) string {
	// Build sorted input for hashing per spec §11.2
	input := map[string]interface{}{
		"org_id":      req.OrgID,
		"subject_id":  req.SubjectID,
		"user_groups": req.UserGroups,
		"action":      req.Action,
		"resource":    req.Resource,
	}
	b, _ := json.Marshal(input)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%s%s:%x", cachePrefix, req.OrgID, sum)
}

func orgIndexKey(orgID string) string {
	return fmt.Sprintf("policy:index:%s", orgID)
}

// Evaluate runs the policy decision engine with two-tier caching and singleflight.
func (s *Service) Evaluate(ctx context.Context, req EvaluateRequest) (*EvaluateResponse, error) {
	start := time.Now()
	key := cacheKey(req)

	// Tier 1: Redis cache check with Stale-While-Revalidate
	if s.rdb != nil {
		if cached, err := s.rdb.Get(ctx, key).Bytes(); err == nil {
			var decision cachedDecision
			if json.Unmarshal(cached, &decision) == nil {
				latency := int(time.Since(start).Milliseconds())

				// @AI-INTENT: [Pattern: Stale-While-Revalidate] Entry is within the grace period
				// (handled by Redis TTL) but past its ideal expiry. Return stale data
				// immediately and refresh in background.
				isStale := time.Now().After(decision.ExpiresAt)
				if isStale {
					// Entry is within the grace period (handled by Redis TTL) but past its ideal expiry
					telemetry.CacheHits.WithLabelValues("stale").Inc()
					select {
					case s.refreshSem <- struct{}{}:
						go s.backgroundRefresh(req, key)
					default:
						s.logger.Debug("background refresh skipped, semaphore full")
					}
				} else {
					telemetry.CacheHits.WithLabelValues("redis").Inc()
				}

				telemetry.EvaluateDuration.WithLabelValues(req.OrgID).Observe(time.Since(start).Seconds())
				return &EvaluateResponse{
					Effect:           decision.Effect,
					MatchedPolicyIDs: decision.MatchedPolicyIDs,
					MaxVersion:       decision.MaxVersion,
					CacheHit:         "redis",
					LatencyMs:        latency,
				}, nil
			}
		}
	}

	// @AI-INTENT: [Pattern: Singleflight] Tier 2: Postgres evaluation with singleflight
	// Ensures only one DB query per unique cache key if multiple concurrent requests
	// miss the cache at the same time.
	type sfResult struct {
		resp *EvaluateResponse
		err  error
	}
	ch := s.sfGroup.DoChan(key, func() (interface{}, error) {
		resp, err := s.evaluateFromDB(ctx, req, key)
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
		telemetry.EvaluateDuration.WithLabelValues(req.OrgID).Observe(time.Since(start).Seconds())
		telemetry.CacheHits.WithLabelValues("none").Inc()

		// Write to eval log (non-blocking, bounded)
		select {
		case s.logCh <- evalLogEntry{req, r.resp}:
		default:
			s.logger.Warn("eval log channel full, dropping entry")
		}

		return r.resp, nil
	}
}

// evaluateFromDB fetches policies from Postgres and evaluates them.
func (s *Service) evaluateFromDB(ctx context.Context, req EvaluateRequest, key string) (*EvaluateResponse, error) {
	policies, err := resilience.Call(ctx, s.dbBreaker, 50*time.Millisecond, func(ctx context.Context) ([]repository.Policy, error) {
		return s.repo.GetMatchingPolicies(ctx, req.OrgID, req.SubjectID, req.UserGroups)
	})
	if err != nil {
		// @AI-INTENT: [Pattern: Fail-Closed] If we can't read policies, deny access.
		s.logger.Error("failed to fetch policies, failing closed", "error", err, "org_id", req.OrgID)
		return &EvaluateResponse{
			Effect:           "deny",
			MatchedPolicyIDs: []string{},
			CacheHit:         "none",
		}, nil
	}

	effect, matchedIDs, maxVersion := s.evaluate(req, policies)

	resp := &EvaluateResponse{
		Effect:           effect,
		MatchedPolicyIDs: matchedIDs,
		MaxVersion:       maxVersion,
		CacheHit:         "none",
	}

	// Cache the result in Redis
	if s.rdb != nil {
		now := time.Now()
		decision := cachedDecision{
			Effect:           effect,
			MatchedPolicyIDs: matchedIDs,
			MaxVersion:       maxVersion,
			ExpiresAt:        now.Add(cacheTTL - staleWindow), // Actual ideal expiry
		}
		if b, err := json.Marshal(decision); err == nil {
			pipe := s.rdb.Pipeline()
			// Set Redis TTL to cacheTTL (which includes the stale window)
			pipe.Set(ctx, key, b, cacheTTL)
			pipe.SAdd(ctx, orgIndexKey(req.OrgID), key)
			pipe.Expire(ctx, orgIndexKey(req.OrgID), 24*time.Hour)
			_, _ = pipe.Exec(ctx)
		}
	}

	return resp, nil
}

// Evaluate applies RBAC logic against the policy set.
// Default is DENY — all requests are denied unless explicitly allowed.
// An explicit deny overrides any allow.
func (s *Service) EvaluateInternal(req EvaluateRequest, policies []repository.Policy) (string, []string, int) {
	return s.evaluate(req, policies)
}

func (s *Service) evaluate(req EvaluateRequest, policies []repository.Policy) (string, []string, int) {
	var matchedIDs []string
	effect := "deny"
	maxVersion := 0
	hasExplicitAllow := false
	hasExplicitDeny := false

	for _, p := range policies {
		// ... (I'll just replace the whole function signature and the return)
		var logic PolicyLogic
		if err := json.Unmarshal(p.Logic, &logic); err != nil {
			s.logger.Warn("failed to parse policy logic, skipping", "policy_id", p.ID, "error", err)
			continue
		}

		switch logic.Type {
		case "deny_all":
			matchedIDs = append(matchedIDs, p.ID)
			hasExplicitDeny = true
			if p.Version > maxVersion {
				maxVersion = p.Version
			}

		case "allow_all":
			matchedIDs = append(matchedIDs, p.ID)
			hasExplicitAllow = true
			if p.Version > maxVersion {
				maxVersion = p.Version
			}

		case "rbac":
			subjectMatch := matchesGlob(logic.Subjects, req.SubjectID)
			actionMatch := matchesGlob(logic.Actions, req.Action)
			resourceMatch := matchesGlob(logic.Resources, req.Resource)

			if subjectMatch && actionMatch && resourceMatch {
				matchedIDs = append(matchedIDs, p.ID)
				hasExplicitAllow = true
				if p.Version > maxVersion {
					maxVersion = p.Version
				}
			}

		case "cel":
			if s.celEnv == nil || logic.Expression == "" {
				continue
			}

			ast, issues := s.celEnv.Compile(logic.Expression)
			if issues != nil && issues.Err() != nil {
				s.logger.Warn("failed to compile CEL expression", "policy_id", p.ID, "error", issues.Err())
				continue
			}

			program, err := s.celEnv.Program(ast)
			if err != nil {
				s.logger.Warn("failed to create CEL program", "policy_id", p.ID, "error", err)
				continue
			}

			out, _, err := program.Eval(map[string]interface{}{
				"subject":     req.SubjectID,
				"action":      req.Action,
				"resource":    req.Resource,
				"user_groups": req.UserGroups,
			})
			if err != nil {
				s.logger.Warn("failed to evaluate CEL expression", "policy_id", p.ID, "error", err)
				continue
			}

			if out.Type() == types.BoolType && out.Value().(bool) {
				matchedIDs = append(matchedIDs, p.ID)
				hasExplicitAllow = true
				if p.Version > maxVersion {
					maxVersion = p.Version
				}
			}
		}
	}

	// Explicit deny overrides any allow
	if hasExplicitDeny {
		return "deny", matchedIDs, maxVersion
	}
	if hasExplicitAllow {
		effect = "allow"
	}

	return effect, matchedIDs, maxVersion
}

func (s *Service) MatchesGlob(patterns []string, value string) bool {
	return matchesGlob(patterns, value)
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
// It carries the org_id into the detached context so withOrgContext can set
// the RLS session variable on the background connection.
func (s *Service) backgroundRefresh(req EvaluateRequest, key string) {
	defer func() { <-s.refreshSem }()
	ctx, cancel := context.WithTimeout(
		rls.WithOrgID(context.Background(), req.OrgID),
		maxRetryDelay,
	)
	defer cancel()
	_, _ = s.evaluateFromDB(ctx, req, key)
}

// CreatePolicy creates a new policy and publishes a change event.
func (s *Service) CreatePolicy(ctx context.Context, orgID, name, description string, logic json.RawMessage) (*repository.Policy, error) {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	p, err := s.repo.CreatePolicyTx(ctx, tx, orgID, name, description, logic)
	if err != nil {
		return nil, err
	}

	if err := s.PublishPolicyChange(ctx, tx, orgID, p.ID, "policy.created"); err != nil {
		return nil, fmt.Errorf("publish change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return p, nil
}

// UpdatePolicy updates a policy and publishes a change event.
func (s *Service) UpdatePolicy(ctx context.Context, orgID, policyID, name, description string, logic json.RawMessage) (*repository.Policy, error) {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	p, err := s.repo.UpdatePolicyTx(ctx, tx, orgID, policyID, name, description, logic)
	if err != nil {
		return nil, err
	}

	if err := s.PublishPolicyChange(ctx, tx, orgID, p.ID, "policy.updated"); err != nil {
		return nil, fmt.Errorf("publish change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return p, nil
}

// DeletePolicy deletes a policy and publishes a change event.
func (s *Service) DeletePolicy(ctx context.Context, orgID, policyID string) error {
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := s.repo.DeletePolicyTx(ctx, tx, orgID, policyID); err != nil {
		return err
	}

	if err := s.PublishPolicyChange(ctx, tx, orgID, policyID, "policy.deleted"); err != nil {
		return fmt.Errorf("publish change: %w", err)
	}

	return tx.Commit(ctx)
}

// writeEvalLog writes a policy evaluation log entry asynchronously.
// It carries the org_id into the detached context so withOrgContext can set
// the RLS session variable on the background connection.
// writeEvalLog handles the actual DB write for a log entry.
func (s *Service) processWriteEvalLog(req EvaluateRequest, resp *EvaluateResponse) {
	ctx, cancel := context.WithTimeout(
		rls.WithOrgID(context.Background(), req.OrgID),
		5*time.Second,
	)
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
func (s *Service) PublishPolicyChange(ctx context.Context, tx pgx.Tx, orgID, policyID, eventType string) error {
	payload, _ := json.Marshal(map[string]string{
		"org_id":     orgID,
		"policy_id":  policyID,
		"event_type": eventType,
	})
	return s.outboxWriter.WriteTx(ctx, tx, orgID, "policy.changes", policyID, payload)
}

// InvalidateCache removes the cached evaluation for all policy keys for an org.
// This is a best-effort operation triggered on policy.changes Kafka events.
func (s *Service) InvalidateOrgCache(ctx context.Context, orgID string) error {
	if s.rdb == nil {
		return nil
	}

	indexKey := orgIndexKey(orgID)
	keys, err := s.rdb.SMembers(ctx, indexKey).Result()
	if err != nil {
		return fmt.Errorf("get indexed keys: %w", err)
	}

	if len(keys) > 0 {
		pipe := s.rdb.Pipeline()
		pipe.Del(ctx, keys...)
		pipe.Del(ctx, indexKey)
		_, err = pipe.Exec(ctx)
		if err != nil {
			return fmt.Errorf("invalidate keys: %w", err)
		}
	}

	return nil
}

func (s *Service) CreateAssignment(ctx context.Context, orgID, policyID, subjectID, subjectType string) (*repository.Assignment, error) {
	a, err := s.repo.CreateAssignment(ctx, orgID, policyID, subjectID, subjectType)
	if err != nil {
		return nil, err
	}

	// Invalidate cache for the org (R-14)
	go func() { _ = s.InvalidateOrgCache(context.Background(), orgID) }()

	return a, nil
}

func (s *Service) DeleteAssignment(ctx context.Context, orgID, assignmentID string) error {
	err := s.repo.DeleteAssignment(ctx, orgID, assignmentID)
	if err != nil {
		return err
	}

	// Invalidate cache for the org (R-14)
	go func() { _ = s.InvalidateOrgCache(context.Background(), orgID) }()

	return nil
}

func (s *Service) ListPolicies(ctx context.Context, orgID string) ([]repository.Policy, error) {
	return s.repo.ListPolicies(ctx, orgID)
}

func (s *Service) GetPolicy(ctx context.Context, orgID, policyID string) (*repository.Policy, error) {
	return s.repo.GetPolicy(ctx, orgID, policyID)
}

func (s *Service) ListEvalLogs(ctx context.Context, orgID string, limit int) ([]repository.EvalLog, error) {
	return s.repo.ListEvalLogs(ctx, orgID, limit)
}

func (s *Service) ListAssignments(ctx context.Context, orgID string) ([]repository.Assignment, error) {
	return s.repo.ListAssignments(ctx, orgID)
}

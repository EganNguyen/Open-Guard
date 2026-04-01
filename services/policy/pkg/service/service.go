package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/openguard/policy/pkg/repository"
	"github.com/openguard/shared/models"
	"github.com/redis/go-redis/v9"
)

// ErrBadRequest is returned for invalid input validation.
var ErrBadRequest = errors.New("bad request")

// ErrNotFound is returned when a resource is not found.
var ErrNotFound = errors.New("not found")

// PolicyRepository defines the persistence interface for policies.
type PolicyRepository interface {
	Create(ctx context.Context, p *models.Policy) error
	GetByID(ctx context.Context, orgID, policyID string) (*models.Policy, error)
	ListByOrg(ctx context.Context, orgID string) ([]*models.Policy, error)
	ListEnabledForOrg(ctx context.Context, orgID string) ([]*models.Policy, error)
	Update(ctx context.Context, p *models.Policy) error
	Delete(ctx context.Context, orgID, policyID string) error
	LogEvaluation(ctx context.Context, log *repository.EvalLog) error
}

// EvalRequest is the input to the policy evaluator.
type EvalRequest struct {
	OrgID       string   `json:"org_id"`
	UserID      string   `json:"user_id"`
	UserGroups  []string `json:"user_groups"`
	Action      string   `json:"action"`
	Resource    string   `json:"resource"`
	IPAddress   string   `json:"ip_address,omitempty"`
}

// EvalResponse is the result of policy evaluation.
type EvalResponse struct {
	Permitted      bool     `json:"permitted"`
	MatchedPolicies []string `json:"matched_policies"`
	Reason         string   `json:"reason"`
	Cached         bool     `json:"cached"`
}

// Service handles real-time RBAC policy evaluation with Redis caching.
// Fail closed: if evaluation fails due to DB error, access is denied.
type Service struct {
	repo         PolicyRepository
	redis        *redis.Client
	cacheTTL     time.Duration
	logger       *slog.Logger
}

func New(
	repo PolicyRepository,
	rdb *redis.Client,
	cacheTTLSeconds int,
	logger *slog.Logger,
) *Service {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return &Service{
		repo:     repo,
		redis:    rdb,
		cacheTTL: time.Duration(cacheTTLSeconds) * time.Second,
		logger:   logger,
	}
}

// ── CRUD Methods ─────────────────────────────────────────────────────────────

func (s *Service) Create(ctx context.Context, p *models.Policy) error {
	if p.Name == "" {
		return fmt.Errorf("%w: policy name is required", ErrBadRequest)
	}
	if p.OrgID == "" {
		return fmt.Errorf("%w: org_id is required", ErrBadRequest)
	}
	return s.repo.Create(ctx, p)
}

func (s *Service) Get(ctx context.Context, orgID, policyID string) (*models.Policy, error) {
	return s.repo.GetByID(ctx, orgID, policyID)
}

func (s *Service) List(ctx context.Context, orgID string) ([]*models.Policy, error) {
	return s.repo.ListByOrg(ctx, orgID)
}

func (s *Service) Update(ctx context.Context, p *models.Policy) error {
	if p.Name == "" {
		return fmt.Errorf("%w: policy name is required", ErrBadRequest)
	}
	return s.repo.Update(ctx, p)
}

func (s *Service) Delete(ctx context.Context, orgID, policyID string) error {
	return s.repo.Delete(ctx, orgID, policyID)
}

// ── Evaluation Methods ───────────────────────────────────────────────────────

// Evaluate checks whether the requested action is permitted for the user.
// Results are cached in Redis with the TTL from POLICY_CACHE_TTL_SECONDS.
// Fails closed: if DB or policy evaluation errors, returns denied.
func (s *Service) Evaluate(ctx context.Context, req EvalRequest) (*EvalResponse, error) {
	start := time.Now()

	cacheKey := evalCacheKey(req)

	// Check cache first
	cached, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var resp EvalResponse
		if jsonErr := json.Unmarshal([]byte(cached), &resp); jsonErr == nil {
			resp.Cached = true
			latency := int(time.Since(start).Milliseconds())
			// Log asynchronously to not block the response
			go func() {
				logCtx := context.WithoutCancel(ctx)
				ids := resp.MatchedPolicies
				_ = s.repo.LogEvaluation(logCtx, &repository.EvalLog{
					OrgID:     req.OrgID,
					UserID:    req.UserID,
					Action:    req.Action,
					Resource:  req.Resource,
					Result:    resp.Permitted,
					PolicyIDs: ids,
					LatencyMs: latency,
					Cached:    true,
				})
			}()
			return &resp, nil
		}
	} else if !errors.Is(err, redis.Nil) {
		s.logger.Warn("redis cache read error", "error", err)
	}

	// Cache miss — evaluate from DB
	policies, err := s.repo.ListEnabledForOrg(ctx, req.OrgID)
	if err != nil {
		// Fail closed: deny on DB error
		s.logger.Error("policy db error — failing closed", "org_id", req.OrgID, "error", err)
		return &EvalResponse{Permitted: false, Reason: "policy evaluation failed — fail closed"}, nil
	}

	resp := s.evaluate(req, policies)
	latency := int(time.Since(start).Milliseconds())

	// Write to cache — suppress errors (stale cache OK, fail closed on next miss)
	if data, marshalErr := json.Marshal(resp); marshalErr == nil {
		pipe := s.redis.Pipeline()
		pipe.Set(ctx, cacheKey, data, s.cacheTTL)
		// Maintain a set of keys per org for O(M) invalidation per spec §0.14
		orgKeysSet := fmt.Sprintf("policy:org_keys:%s", req.OrgID)
		pipe.SAdd(ctx, orgKeysSet, cacheKey)
		pipe.Expire(ctx, orgKeysSet, s.cacheTTL+1*time.Hour) // Set TTL slightly longer than entries

		if _, cacheErr := pipe.Exec(ctx); cacheErr != nil {
			s.logger.Warn("redis cache write error", "error", cacheErr)
		}
	}

	// Log evaluation asynchronously
	go func() {
		logCtx := context.WithoutCancel(ctx)
		_ = s.repo.LogEvaluation(logCtx, &repository.EvalLog{
			OrgID:     req.OrgID,
			UserID:    req.UserID,
			Action:    req.Action,
			Resource:  req.Resource,
			Result:    resp.Permitted,
			PolicyIDs: resp.MatchedPolicies,
			LatencyMs: latency,
			Cached:    false,
		})
	}()

	return resp, nil
}

// evaluate applies all policies and returns the combined result.
// Default is DENY if no policy explicitly permits.
func (s *Service) evaluate(req EvalRequest, policies []*models.Policy) *EvalResponse {
	if len(policies) == 0 {
		// No policies means implicit allow (no restrictions configured)
		return &EvalResponse{Permitted: true, Reason: "no policies configured"}
	}

	var matchedIDs []string
	denied := false
	denyReason := ""

	for _, p := range policies {
		if !p.Enabled {
			continue
		}

		matched, deny, reason := applyPolicy(req, p)
		if matched {
			matchedIDs = append(matchedIDs, p.ID)
			if deny {
				denied = true
				denyReason = reason
			}
		}
	}

	if denied {
		return &EvalResponse{
			Permitted:      false,
			MatchedPolicies: matchedIDs,
			Reason:         denyReason,
		}
	}

	return &EvalResponse{
		Permitted:      true,
		MatchedPolicies: matchedIDs,
		Reason:         "access permitted",
	}
}

// applyPolicy evaluates a single policy against the request.
// Returns (matched bool, deny bool, reason string).
func applyPolicy(req EvalRequest, p *models.Policy) (matched bool, deny bool, reason string) {
	var rules map[string]interface{}
	if err := json.Unmarshal(p.Rules, &rules); err != nil {
		return false, false, ""
	}

	switch models.PolicyType(p.Type) {
	case models.PolicyTypeIPAllowlist:
		return applyIPAllowlist(req, rules)

	case models.PolicyTypeRBAC:
		return applyRBAC(req, rules)

	case models.PolicyTypeSessionLimit:
		// Session limits are enforced at login time, not at evaluation time.
		// Return not matched so we don't block API calls.
		return false, false, ""

	case models.PolicyTypeDataExport:
		return applyDataExportPolicy(req, rules)

	case models.PolicyTypeAnonAccess:
		return applyAnonAccessPolicy(req, rules)

	default:
		return false, false, ""
	}
}

func applyIPAllowlist(req EvalRequest, rules map[string]interface{}) (bool, bool, string) {
	allowedIPs, ok := rules["allowed_ips"].([]interface{})
	if !ok || len(allowedIPs) == 0 {
		return false, false, ""
	}

	for _, ip := range allowedIPs {
		if ip == req.IPAddress {
			return true, false, "ip in allowlist"
		}
	}

	return true, true, fmt.Sprintf("ip %q not in allowlist", req.IPAddress)
}

func applyDataExportPolicy(req EvalRequest, rules map[string]interface{}) (bool, bool, string) {
	if req.Action != "data.export" {
		return false, false, ""
	}

	// Extract allowed roles/groups from rule
	allowedRoles, _ := rules["allowed_roles"].([]interface{})
	for _, role := range allowedRoles {
		for _, group := range req.UserGroups {
			if role == group {
				return true, false, "role permitted for export"
			}
		}
	}

	return true, true, "data export not permitted for user's roles"
}

func applyAnonAccessPolicy(req EvalRequest, rules map[string]interface{}) (bool, bool, string) {
	allowed, _ := rules["allow_anonymous"].(bool)
	if req.UserID == "" && !allowed {
		return true, true, "anonymous access not permitted"
	}
	return false, false, ""
}

// applyRBAC checks whether the user belongs to at least one of the allowed_roles
// defined in the RBAC policy rules. If the user has no matching role, access is denied.
// Rules shape: { "allowed_roles": ["admin", "editor"] }
func applyRBAC(req EvalRequest, rules map[string]interface{}) (bool, bool, string) {
	allowedRoles, ok := rules["allowed_roles"].([]interface{})
	if !ok || len(allowedRoles) == 0 {
		// No roles configured means RBAC policy is a no-op for this request.
		return false, false, ""
	}

	allowedSet := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		if role, ok := r.(string); ok {
			allowedSet[role] = struct{}{}
		}
	}

	for _, group := range req.UserGroups {
		if _, ok := allowedSet[group]; ok {
			return true, false, fmt.Sprintf("rbac: role %q permitted", group)
		}
	}

	return true, true, fmt.Sprintf("rbac: user %q has no permitted role (required one of: %v)", req.UserID, allowedRoles)
}

// InvalidateCacheForOrg deletes all cached eval results for an org using the org-key index (O(M)).
// Called by the Kafka consumer when a policy.changes event is received.
func (s *Service) InvalidateCacheForOrg(ctx context.Context, orgID string) error {
	orgKeysSet := fmt.Sprintf("policy:org_keys:%s", orgID)

	// Fetch all keys associated with this org (O(M))
	keys, err := s.redis.SMembers(ctx, orgKeysSet).Result()
	if err != nil {
		return fmt.Errorf("redis smembers: %w", err)
	}

	if len(keys) > 0 {
		pipe := s.redis.Pipeline()
		pipe.Del(ctx, keys...)
		pipe.Del(ctx, orgKeysSet)
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("redis pipeline del: %w", err)
		}
		s.logger.Info("invalidated policy cache", "org_id", orgID, "keys_deleted", len(keys))
	}

	return nil
}

// evalCacheKey computes the canonical Redis key for a policy evaluation request.
// Key format: "policy:eval:{org_id}:{sha256(action+resource+user_id+sorted(user_groups))}"
func evalCacheKey(req EvalRequest) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s", req.Action, req.Resource, req.UserID, req.IPAddress)
	for _, g := range req.UserGroups {
		fmt.Fprintf(h, "|%s", g)
	}
	return fmt.Sprintf("policy:eval:%s:%x", req.OrgID, h.Sum(nil))
}

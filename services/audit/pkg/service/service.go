package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/openguard/audit/pkg/models"
)

// Repository defines the interface needed by the Audit service.
type Repository interface {
	FindEvents(ctx context.Context, filter interface{}, limit, skip int64) ([]models.AuditEvent, error)
	GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error)
	GetLastChainState(ctx context.Context, orgID string) (int64, string, error)
}

type Service struct {
	repo   Repository
	logger *slog.Logger
	secret string
	isDev  bool
}

func New(repo Repository, secret string, logger *slog.Logger, isDev bool) *Service {
	return &Service{
		repo:   repo,
		secret: secret,
		logger: logger,
		isDev:  isDev,
	}
}

type IntegrityResult struct {
	Ok           bool     `json:"ok"`
	Gaps         int      `json:"gaps,omitempty"`
	Mismatches   []string `json:"mismatches,omitempty"`
	CheckedCount int64    `json:"checked_count"`
}

// VerifyIntegrity checks the HMAC-hash chain for a specific organization's audit trail.
func (s *Service) VerifyIntegrity(ctx context.Context, orgID string) (*IntegrityResult, error) {
	events, err := s.repo.GetIntegrityChain(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("fetch integrity chain: %w", err)
	}

	result := &IntegrityResult{
		Ok:         true,
		Mismatches: make([]string, 0),
	}

	if len(events) == 0 {
		return result, nil
	}

	expectedSeq := events[0].ChainSeq
	var prevHash string // starts empty for the first event in the chain

	for _, ev := range events {
		// 1. Check for Sequence Gaps
		if ev.ChainSeq != expectedSeq {
			result.Ok = false
			result.Gaps++
			expectedSeq = ev.ChainSeq
		}

		// 2. Check Previous Hash Link
		if ev.PrevChainHash != prevHash {
			result.Ok = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("event:%s (prev_hash link broken)", ev.EventID))
		}

		// 3. Verify HMAC Hash Integrity
		computedHash := models.ChainHash(s.secret, prevHash, ev)
		if computedHash != ev.ChainHash {
			result.Ok = false
			result.Mismatches = append(result.Mismatches, fmt.Sprintf("event:%s (content tampered or invalid hash)", ev.EventID))
		}

		prevHash = ev.ChainHash
		expectedSeq++
		result.CheckedCount++
	}

	return result, nil
}

// FindEvents proxies the repository find call with added logging/telemetry.
func (s *Service) FindEvents(ctx context.Context, filter interface{}, limit, skip int64) ([]models.AuditEvent, error) {
	return s.repo.FindEvents(ctx, filter, limit, skip)
}

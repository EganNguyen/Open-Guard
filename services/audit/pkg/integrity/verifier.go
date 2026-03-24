package integrity

import (
	"context"

	"github.com/openguard/audit/pkg/models"
)

type integrityReader interface {
	GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error)
}

type Verifier struct {
	repo   integrityReader
	secret string
}

func NewVerifier(repo integrityReader, secret string) *Verifier {
	return &Verifier{
		repo:   repo,
		secret: secret,
	}
}

type IntegrityResult struct {
	Ok           bool     `json:"ok"`
	Gaps         int      `json:"gaps,omitempty"`
	Mismatches   []string `json:"mismatches,omitempty"`
	CheckedCount int64    `json:"checked_count"`
}

// VerifyChain checks the hash chain for a specific organization
func (v *Verifier) VerifyChain(ctx context.Context, orgID string) (*IntegrityResult, error) {
	events, err := v.repo.GetIntegrityChain(ctx, orgID)
	if err != nil {
		return nil, err
	}

	result := &IntegrityResult{
		Ok:         true,
		Mismatches: make([]string, 0),
	}

	if len(events) == 0 {
		return result, nil
	}

	expectedSeq := events[0].ChainSeq
	var prevHash string // starts empty for the first event

	for _, ev := range events {
		// 1. Check for Sequence Gaps
		if ev.ChainSeq != expectedSeq {
			result.Ok = false
			result.Gaps++
			// Fast-forward expected seq
			expectedSeq = ev.ChainSeq
		}

		// 2. Check Previous Hash Link
		if ev.PrevChainHash != prevHash {
			result.Ok = false
			result.Mismatches = append(result.Mismatches, ev.EventID+" (invalid prev_hash linking)")
		}

		// 3. Check Current Event Hash
		computedHash := models.ChainHash(v.secret, prevHash, ev)
		if computedHash != ev.ChainHash {
			result.Ok = false
			result.Mismatches = append(result.Mismatches, ev.EventID+" (tampered contents or invalid hash)")
		}

		prevHash = ev.ChainHash
		expectedSeq++
		result.CheckedCount++
	}

	return result, nil
}

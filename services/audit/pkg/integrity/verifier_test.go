package integrity_test

import (
	"context"
	"testing"
	"time"

	"github.com/openguard/audit/pkg/integrity"
	"github.com/openguard/audit/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReadRepo struct {
	events []models.AuditEvent
}

func (m *mockReadRepo) GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error) {
	return m.events, nil
}

func TestVerifier_VerifyChain(t *testing.T) {
	secret := "my-secret"
	now := time.Now()
	
	t.Run("valid chain", func(t *testing.T) {
		ev0 := models.AuditEvent{
			EventID: "0", OrgID: "org1", Type: "t", OccurredAt: now,
			PrevChainHash: "", ChainSeq: 0,
		}
		ev0.ChainHash = models.ChainHash(secret, "", ev0)
		
		ev1 := models.AuditEvent{
			EventID: "1", OrgID: "org1", Type: "t", OccurredAt: now.Add(time.Second),
			PrevChainHash: ev0.ChainHash, ChainSeq: 1,
		}
		ev1.ChainHash = models.ChainHash(secret, ev0.ChainHash, ev1)
		
		repo := &mockReadRepo{events: []models.AuditEvent{ev0, ev1}}
		v := integrity.NewVerifier(repo, secret)
		
		res, err := v.VerifyChain(context.Background(), "org1")
		require.NoError(t, err)
		assert.True(t, res.Ok)
		assert.Equal(t, int64(2), res.CheckedCount)
	})
	
	t.Run("tampered hash", func(t *testing.T) {
		ev0 := models.AuditEvent{
			EventID: "0", OrgID: "org1", Type: "t", OccurredAt: now,
			PrevChainHash: "", ChainSeq: 0,
		}
		ev0.ChainHash = "garbage" // invalid
		
		repo := &mockReadRepo{events: []models.AuditEvent{ev0}}
		v := integrity.NewVerifier(repo, secret)
		
		res, err := v.VerifyChain(context.Background(), "org1")
		require.NoError(t, err)
		assert.False(t, res.Ok)
		assert.Contains(t, res.Mismatches[0], "tampered contents or invalid hash")
	})

	t.Run("sequence gap", func(t *testing.T) {
		ev0 := models.AuditEvent{
			EventID: "0", OrgID: "org1", Type: "t", OccurredAt: now,
			PrevChainHash: "", ChainSeq: 0,
		}
		ev0.ChainHash = models.ChainHash(secret, "", ev0)
		
		ev2 := models.AuditEvent{
			EventID: "2", OrgID: "org1", Type: "t", OccurredAt: now.Add(time.Second),
			PrevChainHash: ev0.ChainHash, ChainSeq: 2, // Missing seq 1
		}
		ev2.ChainHash = models.ChainHash(secret, ev0.ChainHash, ev2)
		
		repo := &mockReadRepo{events: []models.AuditEvent{ev0, ev2}}
		v := integrity.NewVerifier(repo, secret)
		
		res, err := v.VerifyChain(context.Background(), "org1")
		require.NoError(t, err)
		assert.False(t, res.Ok)
		assert.Greater(t, res.Gaps, 0)
	})

	t.Run("broken prev_hash link", func(t *testing.T) {
		ev0 := models.AuditEvent{
			EventID: "0", OrgID: "org1", Type: "t", OccurredAt: now,
			PrevChainHash: "", ChainSeq: 0,
		}
		ev0.ChainHash = models.ChainHash(secret, "", ev0)

		ev1 := models.AuditEvent{
			EventID: "1", OrgID: "org1", Type: "t", OccurredAt: now.Add(time.Second),
			PrevChainHash: "wrong-prev-hash", // broken link
			ChainSeq:      1,
		}
		ev1.ChainHash = models.ChainHash(secret, "wrong-prev-hash", ev1)

		repo := &mockReadRepo{events: []models.AuditEvent{ev0, ev1}}
		v := integrity.NewVerifier(repo, secret)

		res, err := v.VerifyChain(context.Background(), "org1")
		require.NoError(t, err)
		assert.False(t, res.Ok)
		assert.Contains(t, res.Mismatches[0], "invalid prev_hash linking")
	})

	t.Run("empty event list", func(t *testing.T) {
		repo := &mockReadRepo{events: []models.AuditEvent{}}
		v := integrity.NewVerifier(repo, secret)

		res, err := v.VerifyChain(context.Background(), "org1")
		require.NoError(t, err)
		assert.True(t, res.Ok)
		assert.Equal(t, int64(0), res.CheckedCount)
		assert.Empty(t, res.Mismatches)
	})
}


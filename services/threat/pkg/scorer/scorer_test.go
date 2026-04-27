package scorer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompositeScore(t *testing.T) {
	now := time.Now()

	t.Run("Empty scores", func(t *testing.T) {
		val, source := CompositeScore(nil)
		assert.Equal(t, 0.0, val)
		assert.Equal(t, "", source)
	})

	t.Run("Single score no decay", func(t *testing.T) {
		scores := []Score{
			{Value: 0.8, Source: "brute_force", OccurredAt: now},
		}
		val, source := CompositeScore(scores)
		// Small delta due to immediate time.Since calculation
		assert.InDelta(t, 0.8, val, 0.01)
		assert.Equal(t, "brute_force", source)
	})

	t.Run("Decay logic", func(t *testing.T) {
		scores := []Score{
			{Value: 1.0, Source: "old_threat", OccurredAt: now.Add(-60 * time.Minute)},
			{Value: 0.5, Source: "new_threat", OccurredAt: now},
		}
		val, source := CompositeScore(scores)
		
		// 1.0 * exp(-0.05 * 60) = 1.0 * exp(-3) approx 0.049
		// 0.5 * exp(-0.05 * 0) = 0.5
		// 0.5 should be higher than 0.049
		assert.InDelta(t, 0.5, val, 0.01)
		assert.Equal(t, "new_threat", source)
	})

	t.Run("Multiple threats picking highest", func(t *testing.T) {
		scores := []Score{
			{Value: 0.6, Source: "t1", OccurredAt: now},
			{Value: 0.9, Source: "t2", OccurredAt: now},
			{Value: 0.4, Source: "t3", OccurredAt: now},
		}
		val, source := CompositeScore(scores)
		assert.InDelta(t, 0.9, val, 0.01)
		assert.Equal(t, "t2", source)
	})
}

func TestSeverity(t *testing.T) {
	cases := []struct {
		score    float64
		expected string
	}{
		{0.96, "CRITICAL"},
		{0.95, "CRITICAL"},
		{0.85, "HIGH"},
		{0.80, "HIGH"},
		{0.60, "MEDIUM"},
		{0.50, "MEDIUM"},
		{0.40, "LOW"},
		{0.10, "LOW"},
	}

	for _, tc := range cases {
		assert.Equal(t, tc.expected, Severity(tc.score))
	}
}

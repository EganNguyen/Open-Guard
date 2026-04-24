package scorer

import (
	"math"
	"time"
)

type Score struct {
	Value      float64
	Source     string
	OccurredAt time.Time
}

func CompositeScore(scores []Score) (float64, string) {
	if len(scores) == 0 {
		return 0, ""
	}

	maxScore := 0.0
	topSource := ""
	lambda := 0.05 // Recency factor: score * exp(-λ * minutes_ago)

	for _, s := range scores {
		minutesAgo := time.Since(s.OccurredAt).Minutes()
		decayedValue := s.Value * math.Exp(-lambda*minutesAgo)

		if decayedValue > maxScore {
			maxScore = decayedValue
			topSource = s.Source
		}
	}

	return maxScore, topSource
}

func Severity(score float64) string {
	switch {
	case score >= 0.95:
		return "CRITICAL"
	case score >= 0.8:
		return "HIGH"
	case score >= 0.5:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EvaluateDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "policy_evaluate_duration_seconds",
			Help:    "Duration of policy evaluation requests",
			Buckets: []float64{.001, .002, .005, .01, .02, .05, .1, .2, .5, 1},
		},
		[]string{"org_id"},
	)

	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "policy_cache_hit_total",
			Help: "Total number of policy cache hits/misses",
		},
		[]string{"type"}, // none, redis, sdk
	)
)

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EvaluateDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "openguard_policy_evaluate_duration_seconds",
			Help:    "Duration of policy evaluation requests",
			Buckets: []float64{.001, .002, .005, .01, .02, .05, .1, .2, .5, 1},
		},
		[]string{"org_id"},
	)

	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "openguard_policy_cache_hits_total",
			Help: "Total number of policy cache hits/misses",
		},
		[]string{"layer"}, // none, redis, sdk
	)
)

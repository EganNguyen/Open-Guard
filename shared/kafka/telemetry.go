package kafka

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OffsetCommitDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "openguard_kafka_offset_commit_duration_seconds",
		Help:    "Duration of Kafka offset commits",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
	})
)

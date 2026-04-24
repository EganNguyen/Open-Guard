package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ThreatsDetected = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "threat_detections_total",
		Help: "Total threats detected",
	}, []string{"type", "severity"})

	ProcessingLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "threat_processing_duration_seconds",
		Help:    "Time spent processing a threat event",
		Buckets: prometheus.DefBuckets,
	}, []string{"type"})
)

package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
)

var RequestCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests",
	},
	[]string{"path", "method"},
)

var RequestLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"path", "method"},
)

func init() {
	prometheus.MustRegister(RequestCounter)
	prometheus.MustRegister(RequestLatency)
}

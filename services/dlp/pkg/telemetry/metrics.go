package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dlp_operations_total",
		Help: "Total operations for dlp",
	}, []string{"operation", "status"})
)

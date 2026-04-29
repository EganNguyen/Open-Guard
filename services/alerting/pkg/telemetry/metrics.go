package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openguard_alerting_operations_total",
		Help: "Total operations for alerting",
	}, []string{"operation", "status"})
)

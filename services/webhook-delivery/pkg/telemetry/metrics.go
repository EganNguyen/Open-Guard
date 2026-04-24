package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "webhook-delivery_operations_total",
		Help: "Total operations for webhook-delivery",
	}, []string{"operation", "status"})
)

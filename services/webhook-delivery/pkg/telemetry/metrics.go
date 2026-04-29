package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openguard_webhook_delivery_attempts_total",
		Help: "Total operations for webhook-delivery",
	}, []string{"operation", "status"})
)

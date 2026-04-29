package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	OperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "openguard_compliance_operations_total",
		Help: "Total operations for compliance",
	}, []string{"operation", "status"})

	BulkheadRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "openguard_report_bulkhead_rejected_total",
		Help: "Total number of reports rejected by the bulkhead",
	})
)

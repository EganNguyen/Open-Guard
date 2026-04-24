package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	ConnectorOperations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "connector_operations_total",
		Help: "Total operations on connectors",
	}, []string{"operation", "status"})
)

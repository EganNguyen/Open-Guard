package telemetry

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EventsIngested = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "audit_events_ingested_total",
		Help: "Total audit events ingested from Kafka",
	}, []string{"org_id", "topic"})

	BatchFlushDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "audit_batch_flush_duration_seconds",
		Help:    "Duration of audit batch flush to MongoDB",
		Buckets: prometheus.DefBuckets,
	}, []string{"status"})

	HashChainLength = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "audit_hash_chain_sequence",
		Help: "Current hash chain sequence number per org",
	}, []string{"org_id"})

	KafkaBulkInsertSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "openguard_kafka_bulk_insert_size",
		Help:    "Size of audit events batch inserted into MongoDB",
		Buckets: []float64{10, 50, 100, 200, 500, 1000},
	})
)

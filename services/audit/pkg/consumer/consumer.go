package consumer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/audit/pkg/repository"
	"github.com/openguard/services/audit/pkg/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type AuditConsumer struct {
	reader *kafka.Reader
	repo   *repository.AuditWriteRepository
	logger *slog.Logger
}

func NewAuditConsumer(brokers string, groupID string, topic string, repo *repository.AuditWriteRepository, logger *slog.Logger) (*AuditConsumer, error) {
	brokerList := strings.Split(brokers, ",")
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokerList,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: 10e3, // 10KB
		MaxBytes: 10e6, // 10MB
		CommitInterval: 0, // Manual commit (R-07)
	})

	return &AuditConsumer{
		reader: r,
		repo:   repo,
		logger: logger,
	}, nil
}

func (c *AuditConsumer) Start(ctx context.Context) error {
	batchSize := 100
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var batch []kafka.Message

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if len(batch) > 0 {
				c.flush(ctx, batch)
				batch = nil
			}
		default:
			// FetchMessage handles reading and preparing for commit
			m, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				c.logger.Error("failed to fetch kafka message", "error", err)
				continue
			}

			batch = append(batch, m)
			if len(batch) >= batchSize {
				c.flush(ctx, batch)
				batch = nil
			}
		}
	}
}

func (c *AuditConsumer) flush(ctx context.Context, batch []kafka.Message) {
	start := time.Now()
	c.logger.Info("flushing audit batch to mongodb", "size", len(batch))
	
	secretKey := os.Getenv("AUDIT_SECRET_KEY")
	if secretKey == "" {
		c.logger.Warn("AUDIT_SECRET_KEY not set, skipping hash chain")
	}

	var events []map[string]interface{}
	for _, m := range batch {
		// Extract tracing context from Kafka headers (INFRA-04)
		headerMap := make(map[string]string)
		for _, h := range m.Headers {
			headerMap[h.Key] = string(h.Value)
		}
		
		prop := otel.GetTextMapPropagator()
		msgCtx := prop.Extract(ctx, propagation.MapCarrier(headerMap))
		
		_, span := otel.Tracer("audit-consumer").Start(msgCtx, "consume-audit-event", trace.WithSpanKind(trace.SpanKindConsumer))
		defer span.End()

		var event map[string]interface{}
		if err := json.Unmarshal(m.Value, &event); err != nil {
			c.logger.Error("failed to unmarshal kafka message", "error", err)
			span.RecordError(err)
			continue
		}
		event["timestamp"] = time.Now()
		events = append(events, event)

		// Metrics
		orgID, _ := event["org_id"].(string)
		telemetry.EventsIngested.WithLabelValues(orgID, m.Topic).Inc()
	}

	if len(events) == 0 {
		return
	}

	// 1. Reserve sequence range atomically
	orgID, _ := events[0]["org_id"].(string)
	if orgID == "" {
		c.logger.Error("missing org_id in audit event")
		return
	}

	startSeq, prevHash, err := c.repo.ReserveSequence(ctx, orgID, int64(len(events)))
	if err != nil {
		c.logger.Error("failed to reserve sequence", "error", err)
		telemetry.BatchFlushDuration.WithLabelValues("error").Observe(time.Since(start).Seconds())
		return
	}

	// 2. Compute chain locally
	var currentHash string
	for i, event := range events {
		event["sequence"] = startSeq + int64(i)
		
		if secretKey != "" {
			eventData := fmt.Sprintf("%v|%s", event["event_id"], prevHash)
			mac := hmac.New(sha256.New, []byte(secretKey))
			mac.Write([]byte(eventData))
			currentHash = hex.EncodeToString(mac.Sum(nil))
			event["integrity_hash"] = currentHash
			prevHash = currentHash
		}
	}

	// 3. Update the latest hash in hash_chains
	if secretKey != "" && currentHash != "" {
		if err := c.repo.UpdateHashChain(ctx, orgID, currentHash); err != nil {
			c.logger.Error("failed to update hash chain head", "error", err)
		}
		telemetry.HashChainLength.WithLabelValues(orgID).Set(float64(events[len(events)-1]["sequence"].(int64)))
	}

	// 4. BulkWrite all events
	var interfaceEvents []interface{}
	for _, e := range events {
		interfaceEvents = append(interfaceEvents, e)
	}

	if err := c.repo.BulkWrite(ctx, interfaceEvents); err != nil {
		c.logger.Error("bulk write failed", "error", err)
		telemetry.BatchFlushDuration.WithLabelValues("error").Observe(time.Since(start).Seconds())
		return
	}

	// 5. Commit offsets after successful write
	if err := c.reader.CommitMessages(ctx, batch...); err != nil {
		c.logger.Error("offset commit failed", "error", err)
	}

	telemetry.BatchFlushDuration.WithLabelValues("success").Observe(time.Since(start).Seconds())
}

func (c *AuditConsumer) Close() {
	c.reader.Close()
}

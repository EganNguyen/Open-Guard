package consumer

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/openguard/audit/pkg/models"
	sharedmodels "github.com/openguard/shared/models"
	kafkago "github.com/segmentio/kafka-go"
)

type lastEventReader interface {
	GetLastChainState(ctx context.Context, orgID string) (int64, string, error)
}
type bulkAdder interface {
	Add(ctx context.Context, doc models.AuditEvent) error
}

type Consumer struct {
	reader  *kafkago.Reader
	repo    lastEventReader
	writer  bulkAdder
	logger  *slog.Logger
	secret  string
}

func NewConsumer(brokers []string, topics []string, repo lastEventReader, writer bulkAdder, logger *slog.Logger, secret string) *Consumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:     brokers,
		GroupTopics: topics,
		GroupID:     "audit-log-consumer-group",
		MinBytes:    10e3, // 10KB
		MaxBytes:    10e6, // 10MB
	})

	return &Consumer{
		reader:    reader,
		writer:    writer,
		repo:      repo,
		secret:    secret,
		logger:    logger,
	}
}

func (c *Consumer) Start(ctx context.Context) {
	c.logger.Info("Starting Audit Kafka Consumer")
	for {
		if ctx.Err() != nil {
			return
		}

		m, err := c.reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.logger.ErrorContext(ctx, "failed to read message from kafka", "error", err)
			continue
		}

		if err := c.HandleMessage(ctx, m); err != nil {
			c.logger.ErrorContext(ctx, "failed to handle message", "error", err)
		}
	}
}

func (c *Consumer) HandleMessage(ctx context.Context, m kafkago.Message) error {
	var envelope sharedmodels.EventEnvelope
	if err := json.Unmarshal(m.Value, &envelope); err != nil {
		return err
	}

	lastSeq, lastHash, err := c.repo.GetLastChainState(ctx, envelope.OrgID)
	if err != nil {
		return err
	}

	auditEv := models.AuditEvent{
		EventID:       envelope.ID,
		OrgID:         envelope.OrgID,
		Type:          envelope.Type,
		OccurredAt:    envelope.OccurredAt,
		ActorID:       envelope.ActorID,
		Payload:       envelope.Payload,
		TraceID:       envelope.TraceID,
		PrevChainHash: lastHash,
		ChainSeq:      lastSeq + 1,
	}

	auditEv.ChainHash = models.ChainHash(c.secret, lastHash, auditEv)

	return c.writer.Add(ctx, auditEv)
}


func (c *Consumer) Stop() error {
	return c.reader.Close()
}

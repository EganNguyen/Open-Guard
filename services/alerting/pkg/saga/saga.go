package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/alerting/pkg/repository"
	"github.com/openguard/services/alerting/pkg/webhook"
)

type AlertSaga struct {
	reader     *kafka.Reader
	publisher  KafkaPublisher
	repo       *repository.Repository
	siem       *webhook.SIEMDeliverer
	logger     *slog.Logger
}

type KafkaPublisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

func NewAlertSaga(brokers []string, groupID string, topic string, repo *repository.Repository, pub KafkaPublisher, siem *webhook.SIEMDeliverer, logger *slog.Logger) *AlertSaga {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokers,
		GroupID: groupID,
		Topic:   topic,
	})

	return &AlertSaga{
		reader:    r,
		publisher: pub,
		repo:      repo,
		siem:      siem,
		logger:    logger,
	}
}

func (s *AlertSaga) Start(ctx context.Context) error {
	sem := make(chan struct{}, 50) // max 50 concurrent alert goroutines
	var wg sync.WaitGroup

	for {
		m, err := s.reader.FetchMessage(ctx) // FetchMessage: manual commit
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return nil
			}
			s.logger.Error("failed to fetch message", "error", err)
			continue
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(msg kafka.Message) {
			defer wg.Done()
			defer func() { <-sem }()

			s.processMessage(ctx, msg)

			// Commit ONLY after processing
			if err := s.reader.CommitMessages(ctx, msg); err != nil {
				s.logger.Error("failed to commit offset", "error", err)
			}
		}(m)
	}
}

func (s *AlertSaga) processMessage(ctx context.Context, m kafka.Message) {
	var alert repository.Alert
	if err := json.Unmarshal(m.Value, &alert); err != nil {
		s.logger.Error("failed to unmarshal alert", "error", err)
		return
	}

	// Step 1: Persist to MongoDB
	if err := s.executeStep(ctx, alert.ID, "persist", func() error {
		return s.repo.Create(ctx, &alert)
	}); err != nil {
		s.logger.Error("saga step failed", "step", "persist", "alert_id", alert.ID, "error", err)
		return
	}

	// Step 2: Enqueue to notifications.outbound
	if err := s.executeStep(ctx, alert.ID, "notify", func() error {
		payload, _ := json.Marshal(alert)
		return s.publisher.Publish(ctx, "notifications.outbound", alert.ID, payload)
	}); err != nil {
		s.logger.Error("saga step failed", "step", "notify", "alert_id", alert.ID, "error", err)
	}

	// Step 3: Fire SIEM webhook (if configured)
	// For now, assume a mock check or environment variable
	siemURL := "" // This would come from org config in a real impl
	siemSecret := ""
	siemType := webhook.SIEMGeneric
	if siemURL != "" {
		if err := s.executeStep(ctx, alert.ID, "siem", func() error {
			payload, _ := json.Marshal(alert)
			return s.siem.Deliver(ctx, siemType, siemURL, siemSecret, payload)
		}); err != nil {
			s.logger.Error("saga step failed", "step", "siem", "alert_id", alert.ID, "error", err)
		}
	}

	// Step 4: Write to audit.trail
	if err := s.executeStep(ctx, alert.ID, "audit", func() error {
		auditEvent := map[string]interface{}{
			"event_id":   fmt.Sprintf("alert-%s", alert.ID),
			"type":       "threat.alert.created",
			"org_id":     alert.OrgID,
			"actor_id":   alert.DetectorID,
			"actor_type": "detector",
			"payload":    alert,
		}
		payload, _ := json.Marshal(auditEvent)
		return s.publisher.Publish(ctx, "audit.trail", alert.ID, payload)
	}); err != nil {
		s.logger.Error("saga step failed", "step", "audit", "alert_id", alert.ID, "error", err)
	}
}

func (s *AlertSaga) executeStep(ctx context.Context, alertID, stepName string, fn func() error) error {
	var lastErr error
	for i := 0; i < 5; i++ {
		err := fn()
		if err == nil {
			s.repo.UpdateSagaStep(ctx, alertID, repository.SagaStep{
				Step:    stepName,
				Status:  "completed",
				At:      time.Now(),
				Retries: i,
			})
			return nil
		}
		lastErr = err
		s.logger.Warn("saga step retry", "step", stepName, "attempt", i+1, "error", err)

		backoff := time.Duration(1<<i) * 100 * time.Millisecond
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	s.repo.UpdateSagaStep(ctx, alertID, repository.SagaStep{
		Step:   stepName,
		Status: "failed",
		Error:  lastErr.Error(),
		At:     time.Now(),
		Retries: 5,
	})
	return lastErr
}

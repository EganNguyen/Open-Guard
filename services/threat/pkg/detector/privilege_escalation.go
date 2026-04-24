package detector

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/threat/pkg/alert"
	sharedkafka "github.com/openguard/shared/kafka"
)

type PrivilegeEscalationDetector struct {
	rdb          *redis.Client
	authReader   *kafka.Reader // TopicAuthEvents
	policyReader *kafka.Reader // TopicPolicyChanges
	logger       *slog.Logger
	store        *alert.Store
	pub          *sharedkafka.Publisher
}

func NewPrivilegeEscalationDetector(redisAddr string, brokers string, groupID string, authTopic, policyTopic string, store *alert.Store, pub *sharedkafka.Publisher, logger *slog.Logger) *PrivilegeEscalationDetector {
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	brokerList := strings.Split(brokers, ",")
	
	authReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokerList,
		GroupID: groupID + "-auth",
		Topic:   authTopic,
	})

	policyReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: brokerList,
		GroupID: groupID + "-policy",
		Topic:   policyTopic,
	})

	return &PrivilegeEscalationDetector{
		rdb:          rdb,
		authReader:   authReader,
		policyReader: policyReader,
		logger:       logger,
		store:        store,
		pub:          pub,
	}
}

func (d *PrivilegeEscalationDetector) Run(ctx context.Context) error {
	d.logger.Info("Starting PrivilegeEscalationDetector")
	
	// We need to run two consumers. For simplicity in this implementation, we'll use goroutines.
	
	go d.consumeAuth(ctx)
	go d.consumePolicy(ctx)
	
	<-ctx.Done()
	return nil
}

func (d *PrivilegeEscalationDetector) consumeAuth(ctx context.Context) {
	for {
		m, err := d.authReader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d.logger.Error("failed to fetch auth message", "error", err)
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(m.Value, &event); err == nil {
			eventType, _ := event["event_type"].(string)
			if eventType == "auth.login.success" {
				userID, _ := event["user_id"].(string)
				if userID != "" {
					// SET privsec:login:{userID} "1" EX 3600
					d.rdb.Set(ctx, "privsec:login:"+userID, "1", time.Hour)
				}
			}
		}
		d.authReader.CommitMessages(ctx, m)
	}
}

func (d *PrivilegeEscalationDetector) consumePolicy(ctx context.Context) {
	for {
		m, err := d.policyReader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			d.logger.Error("failed to fetch policy message", "error", err)
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(m.Value, &event); err == nil {
			action, _ := event["action"].(string) // or event_type depending on schema
			if action == "role.grant" || strings.Contains(action, "policy.changed") {
				actorID, _ := event["actor_id"].(string)
				targetID, _ := event["target_id"].(string)
				
				if actorID != "" {
					exists, _ := d.rdb.Exists(ctx, "privsec:login:"+actorID).Result()
					if exists > 0 {
						d.logger.Warn("privilege escalation risk detected", 
							"actor_id", actorID, 
							"target_id", targetID, 
							"action", action)
						d.publishThreatEvent(ctx, actorID, targetID, action)
					}
				}
			}
		}
		d.policyReader.CommitMessages(ctx, m)
	}
}

func (d *PrivilegeEscalationDetector) publishThreatEvent(ctx context.Context, actorID, targetID, action string) {
	a := &alert.Alert{
		UserID:   actorID, // The one who performed the action
		Detector: "privilege_escalation",
		Score:    0.9,
		Severity: "HIGH",
		Metadata: map[string]interface{}{
			"target_id": targetID,
			"action":    action,
		},
	}

	if d.store != nil {
		if err := d.store.CreateAlert(ctx, a); err != nil {
			d.logger.Error("failed to persist alert", "error", err)
		}
	}

	payload, _ := json.Marshal(a)

	if d.pub != nil {
		alertID := a.ID.Hex()
		if err := d.pub.Publish(ctx, "threat.alerts", alertID, payload); err != nil {
			d.logger.Error("failed to publish to kafka", "error", err)
		}
	}

	d.rdb.Set(ctx, "threat:privesc:"+actorID, payload, 24*time.Hour)
}

func (d *PrivilegeEscalationDetector) Close() {
	d.authReader.Close()
	d.policyReader.Close()
	d.rdb.Close()
}

package detector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/openguard/services/threat/pkg/alert"
	sharedkafka "github.com/openguard/shared/kafka"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

type AccountTakeoverDetector struct {
	rdb    *redis.Client
	reader *kafka.Reader
	logger *slog.Logger
	store  alert.Persister
	pub    *sharedkafka.Publisher
}

func NewAccountTakeoverDetector(redisAddr string, brokers string, groupID string, topic string, store alert.Persister, pub *sharedkafka.Publisher, logger *slog.Logger) *AccountTakeoverDetector {
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	brokerList := strings.Split(brokers, ",")
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokerList,
		GroupID:  groupID,
		Topic:    topic,
		MinBytes: 10e3,
		MaxBytes: 10e6,
	})

	return &AccountTakeoverDetector{
		rdb:    rdb,
		reader: r,
		logger: logger,
		store:  store,
		pub:    pub,
	}
}

func (d *AccountTakeoverDetector) Run(ctx context.Context) error {
	d.logger.Info("Starting AccountTakeoverDetector")
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			m, err := d.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				d.logger.Error("failed to fetch kafka message", "error", err)
				continue
			}

			if err := d.processEvent(ctx, m); err != nil {
				d.logger.Error("processEvent failed, not committing offset", "error", err)
				continue
			}
			if err := d.reader.CommitMessages(ctx, m); err != nil {
				d.logger.Error("failed to commit kafka offset", "error", err)
			}
		}
	}
}

func (d *AccountTakeoverDetector) processEvent(ctx context.Context, m kafka.Message) error {
	var event map[string]interface{}
	if err := json.Unmarshal(m.Value, &event); err != nil {
		d.logger.Error("failed to unmarshal event", "error", err)
		return nil
	}

	eventType, _ := event["event_type"].(string)
	userID, _ := event["user_id"].(string)
	if userID == "" {
		return nil
	}

	if eventType == "password.changed" {
		// SET ato:pwchange:{userID} "1" EX 86400 (24h)
		if err := d.rdb.Set(ctx, "ato:pwchange:"+userID, "1", 24*time.Hour).Err(); err != nil {
			return err
		}
		return nil
	}

	if eventType == "auth.login.success" {
		// 1. Check if "ato:pwchange:{userID}" exists
		exists, err := d.rdb.Exists(ctx, "ato:pwchange:"+userID).Result()
		if err != nil {
			return err
		}

		// 2. Compute device fingerprint
		ua, _ := event["user_agent"].(string)
		al, _ := event["accept_language"].(string)
		pl, _ := event["platform"].(string)
		fingerprint := d.computeFingerprint(ua, al, pl)

		deviceKey := "ato:devices:" + userID
		isKnown, err := d.rdb.SIsMember(ctx, deviceKey, fingerprint).Result()
		if err != nil {
			return err
		}

		if exists > 0 && !isKnown {
			d.logger.Warn("account takeover suspect detected", "user_id", userID, "fingerprint", fingerprint)
			d.publishThreatEvent(ctx, userID, fingerprint)
		}

		// Update known devices
		pipe := d.rdb.Pipeline()
		pipe.SAdd(ctx, deviceKey, fingerprint)
		pipe.Expire(ctx, deviceKey, 30*24*time.Hour) // 30 days TTL
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (d *AccountTakeoverDetector) computeFingerprint(ua, al, pl string) string {
	h := sha256.New()
	h.Write([]byte(ua + "|" + al + "|" + pl))
	return hex.EncodeToString(h.Sum(nil))
}

func (d *AccountTakeoverDetector) publishThreatEvent(ctx context.Context, userID string, fingerprint string) {
	a := &alert.Alert{
		UserID:   userID,
		Detector: "account_takeover",
		Score:    0.7,
		Severity: "HIGH",
		Metadata: map[string]interface{}{
			"fingerprint": fingerprint,
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

	if err := d.rdb.Set(ctx, "threat:ato:"+userID, payload, 24*time.Hour).Err(); err != nil {
		d.logger.Error("failed to set threat cache", "error", err)
	}
}

func (d *AccountTakeoverDetector) Close() {
	_ = d.reader.Close()
	_ = d.rdb.Close()
}

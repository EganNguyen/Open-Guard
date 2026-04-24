package detector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/threat/pkg/alert"
	sharedkafka "github.com/openguard/shared/kafka"
)

const (
	WindowSize = 5 * time.Minute
	ThreatTTL  = 24 * time.Hour
)

type BruteForceDetector struct {
	rdb         *redis.Client
	reader      *kafka.Reader
	logger      *slog.Logger
	maxAttempts int64
	store       *alert.Store
	pub         *sharedkafka.Publisher
}

func NewBruteForceDetector(redisAddr string, brokers string, groupID string, topic string, store *alert.Store, pub *sharedkafka.Publisher, logger *slog.Logger) (*BruteForceDetector, error) {
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

	maxAttempts := int64(11) // Spec default
	if v := os.Getenv("THREAT_MAX_FAILED_LOGINS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxAttempts = n
		}
	}

	return &BruteForceDetector{
		rdb:         rdb,
		reader:      r,
		logger:      logger,
		maxAttempts: maxAttempts,
		store:       store,
		pub:         pub,
	}, nil
}

func (d *BruteForceDetector) Start(ctx context.Context) error {
	d.logger.Info("Starting BruteForceDetector", "max_attempts", d.maxAttempts)
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

func (d *BruteForceDetector) processEvent(ctx context.Context, m kafka.Message) error {
	var event map[string]interface{}
	if err := json.Unmarshal(m.Value, &event); err != nil {
		d.logger.Error("failed to unmarshal event", "error", err)
		return nil // Invalid JSON is not a retryable error
	}

	eventType, _ := event["event_type"].(string)
	if eventType != "login.failed" && eventType != "auth.failed" {
		return nil
	}

	ip, _ := event["ip"].(string)
	email, _ := event["email"].(string)

	if ip != "" {
		if err := d.trackFailedAttempt(ctx, "bruteforce:ip:"+ip); err != nil {
			return err
		}
	}
	if email != "" {
		if err := d.trackFailedAttempt(ctx, "bruteforce:user:"+email); err != nil {
			return err
		}
	}
	return nil
}

func (d *BruteForceDetector) trackFailedAttempt(ctx context.Context, key string) error {
	now := time.Now().UnixNano() / int64(time.Millisecond)
	windowStart := now - int64(WindowSize/time.Millisecond)

	pipe := d.rdb.Pipeline()
	// Use sorted set for sliding window
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now),
		Member: uuid.New().String(),
	})
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", windowStart))
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, WindowSize)

	_, err := pipe.Exec(ctx)
	if err != nil {
		d.logger.Error("failed to track failed attempt", "error", err, "key", key)
		return err
	}

	count := countCmd.Val()
	d.logger.Debug("failed attempt tracked", "key", key, "count", count)

	if count >= d.maxAttempts {
		d.logger.Warn("brute force attack detected", "key", key, "attempts", count)
		d.publishThreatEvent(ctx, key, count)
	}
	return nil
}

func (d *BruteForceDetector) publishThreatEvent(ctx context.Context, key string, count int64) {
	// Extract userID or IP from key
	parts := strings.Split(key, ":")
	userID := ""
	if len(parts) >= 3 {
		userID = parts[2]
	}

	a := &alert.Alert{
		UserID:   userID,
		Detector: "brute_force",
		Score:    0.9,
		Severity: "HIGH",
		Metadata: map[string]interface{}{
			"attempts": count,
			"key":      key,
		},
	}

	if d.store != nil {
		if err := d.store.CreateAlert(ctx, a); err != nil {
			d.logger.Error("failed to persist alert", "error", err)
		}
	}

	payload, _ := json.Marshal(a)
	d.logger.Info("threat detected", "event", string(payload))

	// Publish to Kafka for alerting saga
	if d.pub != nil {
		alertID := a.ID.Hex()
		if alertID == "" {
			alertID = uuid.New().String()
		}
		if err := d.pub.Publish(ctx, "threat.alerts", alertID, payload); err != nil {
			d.logger.Error("failed to publish to kafka", "error", err)
		}
	}

	// Store in Redis for legacy/quick check
	d.rdb.Set(ctx, "threat:"+key, payload, ThreatTTL)
}

func (d *BruteForceDetector) Close() {
	d.reader.Close()
	d.rdb.Close()
}

func (d *BruteForceDetector) CheckRateLimit(ctx context.Context, key string) (bool, int64) {
	// For backward compatibility or external checks
	count, err := d.rdb.ZCard(ctx, key).Result()
	if err != nil && err != redis.Nil {
		return false, 0
	}
	return count < d.maxAttempts, count
}

func (d *BruteForceDetector) GetThreats(ctx context.Context) ([]map[string]interface{}, error) {
	keys, err := d.rdb.Keys(ctx, "threat:*").Result()
	if err != nil {
		return nil, err
	}

	var threats []map[string]interface{}
	for _, key := range keys {
		val, err := d.rdb.Get(ctx, key).Result()
		if err != nil {
			continue
		}
		var threat map[string]interface{}
		if json.Unmarshal([]byte(val), &threat) == nil {
			threat["key"] = key
			threats = append(threats, threat)
		}
	}
	return threats, nil
}
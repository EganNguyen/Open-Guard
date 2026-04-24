package detector

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

const (
	WindowSize    = 5 * time.Minute
	MaxAttempts  = 5
	ThreatTTL    = 24 * time.Hour
)

type BruteForceDetector struct {
	rdb         *redis.Client
	reader      *kafka.Reader
	logger      *slog.Logger
	maxAttempts int64
}

func NewBruteForceDetector(redisAddr string, brokers string, groupID string, topic string, logger *slog.Logger) (*BruteForceDetector, error) {
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

	maxAttempts := int64(10)
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
	}, nil
}

func (d *BruteForceDetector) Start(ctx context.Context) error {
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

			d.processEvent(ctx, m)
			d.reader.CommitMessages(ctx, m)
		}
	}
}

func (d *BruteForceDetector) processEvent(ctx context.Context, m kafka.Message) {
	var event map[string]interface{}
	if err := json.Unmarshal(m.Value, &event); err != nil {
		d.logger.Error("failed to unmarshal event", "error", err)
		return
	}

	eventType, _ := event["event_type"].(string)
	if eventType != "login.failed" && eventType != "auth.failed" {
		return
	}

	ip, _ := event["ip"].(string)
	email, _ := event["email"].(string)

	if ip != "" {
		d.trackFailedAttempt(ctx, "ip:"+ip)
	}
	if email != "" {
		d.trackFailedAttempt(ctx, "user:"+email)
	}
}

func (d *BruteForceDetector) trackFailedAttempt(ctx context.Context, key string) {
	pipe := d.rdb.Pipeline()

	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, WindowSize)

	_, err := pipe.Exec(ctx)
	if err != nil {
		d.logger.Error("failed to track failed attempt", "error", err, "key", key)
		return
	}

	count := incr.Val()
	d.logger.Debug("failed attempt tracked", "key", key, "count", count)

	if count >= d.maxAttempts {
		d.logger.Warn("brute force attack detected", "key", key, "attempts", count)
		d.publishThreatEvent(ctx, key, count)
	}
}

func (d *BruteForceDetector) publishThreatEvent(ctx context.Context, key string, count int64) {
	threatEvent := map[string]interface{}{
		"threat_type": "brute_force",
		"key":         key,
		"attempts":    count,
		"window":      WindowSize.String(),
		"timestamp":  time.Now().Unix(),
	}

	payload, _ := json.Marshal(threatEvent)
	d.logger.Info("threat detected", "event", string(payload))

	d.rdb.Set(ctx, "threat:"+key, payload, ThreatTTL)
}

func (d *BruteForceDetector) Close() {
	d.reader.Close()
	d.rdb.Close()
}

func (d *BruteForceDetector) CheckRateLimit(ctx context.Context, key string) (bool, int64) {
	count, err := d.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return true, 0
	}
	if err != nil {
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
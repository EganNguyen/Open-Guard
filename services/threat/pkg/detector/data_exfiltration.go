package detector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/threat/pkg/alert"
	sharedkafka "github.com/openguard/shared/kafka"
)

type DataExfiltrationDetector struct {
	rdb    *redis.Client
	reader *kafka.Reader
	logger *slog.Logger
	store  *alert.Store
	pub    *sharedkafka.Publisher
}

func NewDataExfiltrationDetector(redisAddr string, brokers string, groupID string, topic string, store *alert.Store, pub *sharedkafka.Publisher, logger *slog.Logger) *DataExfiltrationDetector {
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

	return &DataExfiltrationDetector{
		rdb:    rdb,
		reader: r,
		logger: logger,
		store:  store,
		pub:    pub,
	}
}

func (d *DataExfiltrationDetector) Run(ctx context.Context) error {
	d.logger.Info("Starting DataExfiltrationDetector")
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

func (d *DataExfiltrationDetector) processEvent(ctx context.Context, m kafka.Message) error {
	var event map[string]interface{}
	if err := json.Unmarshal(m.Value, &event); err != nil {
		d.logger.Error("failed to unmarshal event", "error", err)
		return nil
	}

	// Spec: data.access count for single user exceeds org baseline by 3σ within 1hr
	userID, _ := event["user_id"].(string)
	orgID, _ := event["org_id"].(string)
	if userID == "" || orgID == "" {
		return nil
	}

	now := time.Now().UnixNano() / int64(time.Millisecond)
	hourAgo := now - int64(time.Hour/time.Millisecond)
	key := fmt.Sprintf("access:%s:%s", orgID, userID)

	pipe := d.rdb.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(now), Member: uuid.New().String()})
	pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(hourAgo, 10))
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, time.Hour)

	_, err := pipe.Exec(ctx)
	if err != nil {
		d.logger.Error("failed to update access counts", "error", err)
		return err
	}

	count := countCmd.Val()

	// Get org baseline
	meanKey := fmt.Sprintf("baseline:%s:access_mean", orgID)
	stddevKey := fmt.Sprintf("baseline:%s:access_stddev", orgID)

	mean, _ := d.rdb.Get(ctx, meanKey).Float64()
	stddev, _ := d.rdb.Get(ctx, stddevKey).Float64()

	// If no baseline, we can't detect anomaly yet, or we use a sensible default
	if mean == 0 {
		// Fallback: if count is very high (e.g. > 1000 in an hour)
		if count > 1000 {
			d.alert(ctx, orgID, userID, count, mean, stddev)
		}
		return nil
	}

	if float64(count) > mean+3*stddev {
		d.alert(ctx, orgID, userID, count, mean, stddev)
	}
	return nil
}

	count := countCmd.Val()

	// Get org baseline
	meanKey := fmt.Sprintf("baseline:%s:access_mean", orgID)
	stddevKey := fmt.Sprintf("baseline:%s:access_stddev", orgID)

	mean, _ := d.rdb.Get(ctx, meanKey).Float64()
	stddev, _ := d.rdb.Get(ctx, stddevKey).Float64()

	// If no baseline, we can't detect anomaly yet, or we use a sensible default
	if mean == 0 {
		// Fallback: if count is very high (e.g. > 1000 in an hour)
		if count > 1000 {
			d.alert(ctx, orgID, userID, count, mean, stddev)
		}
		return
	}

	if float64(count) > mean+3*stddev {
		d.alert(ctx, orgID, userID, count, mean, stddev)
	}
}

func (d *DataExfiltrationDetector) alert(ctx context.Context, orgID, userID string, count int64, mean, stddev float64) {
	d.logger.Warn("data exfiltration detected", "user_id", userID, "count", count, "mean", mean, "stddev", stddev)

	a := &alert.Alert{
		OrgID:    orgID,
		UserID:   userID,
		Detector: "data_exfiltration",
		Score:    0.7,
		Severity: "HIGH",
		Metadata: map[string]interface{}{
			"count":  count,
			"mean":   mean,
			"stddev": stddev,
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

	d.rdb.Set(ctx, fmt.Sprintf("threat:exfiltration:%s:%s", orgID, userID), payload, 24*time.Hour)
}

func (d *DataExfiltrationDetector) Close() {
	d.reader.Close()
	d.rdb.Close()
}

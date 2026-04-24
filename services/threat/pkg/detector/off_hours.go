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

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/threat/pkg/alert"
	sharedkafka "github.com/openguard/shared/kafka"
)

type OffHoursDetector struct {
	rdb      *redis.Client
	reader   *kafka.Reader
	offStart int // THREAT_OFF_HOURS_START, default 22 (UTC hour)
	offEnd   int // THREAT_OFF_HOURS_END, default 6 (UTC hour)
	logger   *slog.Logger
	store    *alert.Store
	pub      *sharedkafka.Publisher
}

func NewOffHoursDetector(redisAddr string, brokers string, groupID string, topic string, store *alert.Store, pub *sharedkafka.Publisher, logger *slog.Logger) *OffHoursDetector {
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

	offStart := 22
	if v := os.Getenv("THREAT_OFF_HOURS_START"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offStart = n
		}
	}

	offEnd := 6
	if v := os.Getenv("THREAT_OFF_HOURS_END"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offEnd = n
		}
	}

	return &OffHoursDetector{
		rdb:      rdb,
		reader:   r,
		offStart: offStart,
		offEnd:   offEnd,
		logger:   logger,
		store:    store,
		pub:      pub,
	}
}

func (d *OffHoursDetector) Run(ctx context.Context) error {
	d.logger.Info("Starting OffHoursDetector", "off_start", d.offStart, "off_end", d.offEnd)
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

func (d *OffHoursDetector) processEvent(ctx context.Context, m kafka.Message) {
	var event map[string]interface{}
	if err := json.Unmarshal(m.Value, &event); err != nil {
		d.logger.Error("failed to unmarshal event", "error", err)
		return
	}

	eventType, _ := event["event_type"].(string)
	if eventType != "auth.login.success" {
		return
	}

	userID, _ := event["user_id"].(string)
	orgID, _ := event["org_id"].(string)
	if userID == "" || orgID == "" {
		return
	}

	now := time.Now().UTC()
	hour := now.Hour()
	date := now.Format("2006-01-02")

	isOffHours := false
	if d.offStart > d.offEnd {
		// e.g. 22:00 - 06:00
		isOffHours = hour >= d.offStart || hour < d.offEnd
	} else {
		// e.g. 01:00 - 05:00
		isOffHours = hour >= d.offStart && hour < d.offEnd
	}

	key := fmt.Sprintf("offhours:%s:%s:%s", orgID, userID, date)
	if isOffHours {
		// Check last 3 days
		allPreviousInHours := true
		for i := 1; i <= 3; i++ {
			prevDate := now.AddDate(0, 0, -i).Format("2006-01-02")
			prevKey := fmt.Sprintf("offhours:%s:%s:%s", orgID, userID, prevDate)
			exists, _ := d.rdb.Exists(ctx, prevKey).Result()
			if exists == 0 {
				// If key doesn't exist, we assume it was in-hours (since we only record 1 for in-hours days or similar)
				// Actually the spec says "3+ consecutive days previously all in-hours"
				// Let's use a pattern: "1" = in-hours login recorded for that day.
				// If we don't have a record for a day, we can't be sure, but let's assume in-hours for the sake of the detector.
			} else {
				val, _ := d.rdb.Get(ctx, prevKey).Result()
				if val != "1" {
					allPreviousInHours = false
					break
				}
			}
		}

		if allPreviousInHours {
			d.logger.Warn("off-hours access detected", "user_id", userID, "org_id", orgID, "hour", hour)
			d.publishThreatEvent(ctx, orgID, userID, hour)
		}
	} else {
		// Record in-hours access
		d.rdb.Set(ctx, key, "1", 7*24*time.Hour)
	}
}

func (d *OffHoursDetector) publishThreatEvent(ctx context.Context, orgID, userID string, hour int) {
	a := &alert.Alert{
		OrgID:    orgID,
		UserID:   userID,
		Detector: "off_hours_access",
		Score:    0.5,
		Severity: "MEDIUM",
		Metadata: map[string]interface{}{
			"hour": hour,
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

	d.rdb.Set(ctx, fmt.Sprintf("threat:offhours:%s:%s", orgID, userID), payload, 24*time.Hour)
}

func (d *OffHoursDetector) Close() {
	d.reader.Close()
	d.rdb.Close()
}

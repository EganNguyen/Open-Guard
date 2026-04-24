package detector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/openguard/services/threat/pkg/alert"
	sharedkafka "github.com/openguard/shared/kafka"
)

type LastLogin struct {
	IP        string    `json:"ip"`
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Timestamp time.Time `json:"timestamp"`
}

type ImpossibleTravelDetector struct {
	db         *geoip2.Reader
	rdb        *redis.Client
	reader     *kafka.Reader
	threshold  float64 // THREAT_GEO_CHANGE_THRESHOLD_KM, default 500
	windowSecs int     // 3600 (1 hour, hardcoded per spec)
	logger     *slog.Logger
	store      *alert.Store
	pub        *sharedkafka.Publisher
}

func NewImpossibleTravelDetector(dbPath string, redisAddr string, brokers string, groupID string, topic string, store *alert.Store, pub *sharedkafka.Publisher, logger *slog.Logger) (*ImpossibleTravelDetector, error) {
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open GeoLite2 DB: %w", err)
	}

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

	threshold := 500.0
	if v := os.Getenv("THREAT_GEO_CHANGE_THRESHOLD_KM"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			threshold = f
		}
	}

	return &ImpossibleTravelDetector{
		db:         db,
		rdb:        rdb,
		reader:     r,
		threshold:  threshold,
		windowSecs: 3600,
		logger:     logger,
		store:      store,
		pub:        pub,
	}, nil
}

func (d *ImpossibleTravelDetector) Run(ctx context.Context) error {
	d.logger.Info("Starting ImpossibleTravelDetector", "threshold_km", d.threshold)
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

func (d *ImpossibleTravelDetector) processEvent(ctx context.Context, m kafka.Message) error {
	var event map[string]interface{}
	if err := json.Unmarshal(m.Value, &event); err != nil {
		d.logger.Error("failed to unmarshal event", "error", err)
		return nil
	}

	eventType, _ := event["event_type"].(string)
	if eventType != "auth.login.success" {
		return nil
	}

	userID, _ := event["user_id"].(string)
	ipStr, _ := event["ip"].(string)
	if userID == "" || ipStr == "" {
		return nil
	}

	ip := net.ParseIP(ipStr)
	record, err := d.db.City(ip)
	if err != nil {
		d.logger.Error("failed to geolocate IP", "ip", ipStr, "error", err)
		return nil // Not a retryable error usually, unless DB is down
	}

	current := LastLogin{
		IP:        ipStr,
		Lat:       record.Location.Latitude,
		Lon:       record.Location.Longitude,
		Timestamp: time.Now(),
	}

	// Check last login
	redisKey := "travel:" + userID
	val, err := d.rdb.Get(ctx, redisKey).Result()
	if err == nil {
		var last LastLogin
		if err := json.Unmarshal([]byte(val), &last); err == nil {
			d.detect(ctx, userID, last, current)
		}
	} else if err != redis.Nil {
		return err
	}

	// Store current login
	payload, _ := json.Marshal(current)
	if err := d.rdb.Set(ctx, redisKey, payload, time.Duration(d.windowSecs)*time.Second).Err(); err != nil {
		return err
	}
	return nil
}

func (d *ImpossibleTravelDetector) detect(ctx context.Context, userID string, last, current LastLogin) {
	if last.IP == current.IP {
		return
	}

	dist := haversine(last.Lat, last.Lon, current.Lat, current.Lon)
	timeDelta := current.Timestamp.Sub(last.Timestamp).Seconds()

	if dist > d.threshold && timeDelta < float64(d.windowSecs) {
		d.logger.Warn("impossible travel detected", 
			"user_id", userID, 
			"distance_km", dist, 
			"time_delta_sec", timeDelta,
			"last_ip", last.IP,
			"current_ip", current.IP)
		
		d.publishThreatEvent(ctx, userID, dist, timeDelta)
	}
}

func (d *ImpossibleTravelDetector) publishThreatEvent(ctx context.Context, userID string, dist, timeDelta float64) {
	a := &alert.Alert{
		UserID:   userID,
		Detector: "impossible_travel",
		Score:    0.9,
		Severity: "HIGH",
		Metadata: map[string]interface{}{
			"distance_km":    dist,
			"time_delta_sec": timeDelta,
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
	
	d.rdb.Set(ctx, "threat:travel:"+userID, payload, 24*time.Hour)
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // Earth radius in km
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

func (d *ImpossibleTravelDetector) Close() {
	d.db.Close()
	d.reader.Close()
	d.rdb.Close()
}

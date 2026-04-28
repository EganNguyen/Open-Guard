package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AlertStatus string

const (
	StatusOpen         AlertStatus = "open"
	StatusAcknowledged AlertStatus = "acknowledged"
	StatusResolved     AlertStatus = "resolved"
)

type AlertSeverity string

const (
	SeverityLow      AlertSeverity = "LOW"
	SeverityMedium   AlertSeverity = "MEDIUM"
	SeverityHigh     AlertSeverity = "HIGH"
	SeverityCritical AlertSeverity = "CRITICAL"
)

type Alert struct {
	ID          string        `bson:"_id" json:"id"`
	OrgID       string        `bson:"org_id" json:"org_id"`
	Type        string        `bson:"type" json:"type"`
	Severity    AlertSeverity `bson:"severity" json:"severity"`
	Status      AlertStatus   `bson:"status" json:"status"`
	RiskScore   float64       `bson:"risk_score" json:"risk_score"`
	DetectorID  string        `bson:"detector_id" json:"detector_id"`
	RawEvent    bson.M        `bson:"raw_event" json:"raw_event"`
	SagaSteps   []SagaStep    `bson:"saga_steps" json:"saga_steps"`
	CreatedAt   time.Time     `bson:"created_at" json:"created_at"`
	AckAt       *time.Time    `bson:"ack_at,omitempty" json:"ack_at,omitempty"`
	ResolvedAt  *time.Time    `bson:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	MTTRSeconds float64       `bson:"mttr_seconds" json:"mttr_seconds"`
}

type SagaStep struct {
	Step    string    `bson:"step" json:"step"`
	Status  string    `bson:"status" json:"status"`
	Error   string    `bson:"error,omitempty" json:"error,omitempty"`
	At      time.Time `bson:"at" json:"at"`
	Retries int       `bson:"retries" json:"retries"`
}

type Repository struct {
	DB *mongo.Database
}

func NewRepository(client *mongo.Client, dbName string) *Repository {
	return &Repository{
		DB: client.Database(dbName),
	}
}

func (r *Repository) Create(ctx context.Context, alert *Alert) error {
	coll := r.DB.Collection("alerts")
	_, err := coll.InsertOne(ctx, alert)
	return err
}

func (r *Repository) List(ctx context.Context, orgID string, status AlertStatus, severity AlertSeverity, cursor string, limit int64) ([]Alert, string, error) {
	coll := r.DB.Collection("alerts")

	filter := bson.M{"org_id": orgID}
	if status != "" {
		filter["status"] = status
	}
	if severity != "" {
		filter["severity"] = severity
	}
	if cursor != "" {
		filter["_id"] = bson.M{"$gt": cursor}
	}

	opts := options.Find().SetLimit(limit).SetSort(bson.M{"_id": 1})

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, "", err
	}
	defer cur.Close(ctx)

	var alerts []Alert
	if err := cur.All(ctx, &alerts); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(alerts) > 0 {
		nextCursor = alerts[len(alerts)-1].ID
	}

	return alerts, nextCursor, nil
}

func (r *Repository) GetByID(ctx context.Context, orgID, id string) (*Alert, error) {
	coll := r.DB.Collection("alerts")
	var alert Alert
	err := coll.FindOne(ctx, bson.M{"_id": id, "org_id": orgID}).Decode(&alert)
	if err != nil {
		return nil, err
	}
	return &alert, nil
}

func (r *Repository) Acknowledge(ctx context.Context, orgID, id string) error {
	coll := r.DB.Collection("alerts")
	now := time.Now()
	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": id, "org_id": orgID, "status": StatusOpen},
		bson.M{"$set": bson.M{"status": StatusAcknowledged, "ack_at": now}},
	)
	return err
}

func (r *Repository) Resolve(ctx context.Context, orgID, id string) error {
	coll := r.DB.Collection("alerts")

	// Need to get created_at to compute MTTR
	alert, err := r.GetByID(ctx, orgID, id)
	if err != nil {
		return err
	}

	now := time.Now()
	mttr := now.Sub(alert.CreatedAt).Seconds()

	_, err = coll.UpdateOne(ctx,
		bson.M{"_id": id, "org_id": orgID},
		bson.M{"$set": bson.M{
			"status":       StatusResolved,
			"resolved_at":  now,
			"mttr_seconds": mttr,
		}},
	)
	return err
}

func (r *Repository) GetStats(ctx context.Context, orgID string) (map[string]interface{}, error) {
	coll := r.DB.Collection("alerts")

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"org_id": orgID}}},
		{{Key: "$group", Value: bson.M{
			"_id":      "$severity",
			"count":    bson.M{"$sum": 1},
			"avg_mttr": bson.M{"$avg": "$mttr_seconds"},
		}}},
	}

	cur, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	results := make(map[string]interface{})
	var b []bson.M
	if err := cur.All(ctx, &b); err != nil {
		return nil, err
	}

	severityCounts := make(map[string]int32)
	var totalMTTR float64
	var mttrCount int

	for _, res := range b {
		sev := res["_id"].(string)
		count := res["count"].(int32)
		severityCounts[sev] = count

		if avgMTTR, ok := res["avg_mttr"].(float64); ok {
			totalMTTR += avgMTTR
			mttrCount++
		}
	}

	results["severity_counts"] = severityCounts
	if mttrCount > 0 {
		results["avg_mttr"] = totalMTTR / float64(mttrCount)
	} else {
		results["avg_mttr"] = 0
	}

	return results, nil
}

func (r *Repository) UpdateSagaStep(ctx context.Context, alertID string, step SagaStep) error {
	coll := r.DB.Collection("alerts")
	_, err := coll.UpdateOne(ctx,
		bson.M{"_id": alertID},
		bson.M{"$push": bson.M{"saga_steps": step}},
	)
	return err
}

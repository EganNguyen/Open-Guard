package alert

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Alert struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	OrgID      string             `bson:"org_id" json:"org_id"`
	UserID     string             `bson:"user_id" json:"user_id"`
	Detector   string             `bson:"detector" json:"type"`
	Score      float64            `bson:"score" json:"risk_score"`
	Severity   string             `bson:"severity" json:"severity"` // MEDIUM/HIGH/CRITICAL
	Status     string             `bson:"status" json:"status"`     // open/acknowledged/resolved
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
	ResolvedAt *time.Time         `bson:"resolved_at,omitempty" json:"resolved_at,omitempty"`
	MTTR       *int64             `bson:"mttr_seconds,omitempty" json:"mttr_seconds,omitempty"`
	Metadata   map[string]interface{} `bson:"metadata,omitempty" json:"metadata,omitempty"`
}

type Store struct {
	db *mongo.Database
}

func NewStore(uri string) (*Store, error) {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	return &Store{
		db: client.Database("threats"),
	}, nil
}

func (s *Store) CreateAlert(ctx context.Context, alert *Alert) error {
	if alert.CreatedAt.IsZero() {
		alert.CreatedAt = time.Now()
	}
	if alert.Status == "" {
		alert.Status = "open"
	}
	res, err := s.db.Collection("alerts").InsertOne(ctx, alert)
	if err != nil {
		return err
	}
	alert.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

func (s *Store) GetAlert(ctx context.Context, id string) (*Alert, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	var alert Alert
	err = s.db.Collection("alerts").FindOne(ctx, bson.M{"_id": oid}).Decode(&alert)
	return &alert, err
}

func (s *Store) ListAlerts(ctx context.Context, orgID string, status string, severity string, limit int64, cursor string) ([]Alert, string, error) {
	filter := bson.M{"org_id": orgID}
	if status != "" {
		filter["status"] = status
	}
	if severity != "" {
		filter["severity"] = severity
	}

	if cursor != "" {
		oid, err := primitive.ObjectIDFromHex(cursor)
		if err == nil {
			filter["_id"] = bson.M{"$lt": oid}
		}
	}

	opts := options.Find().SetSort(bson.M{"_id": -1}).SetLimit(limit)
	cursorRes, err := s.db.Collection("alerts").Find(ctx, filter, opts)
	if err != nil {
		return nil, "", err
	}
	defer cursorRes.Close(ctx)

	var alerts []Alert
	if err := cursorRes.All(ctx, &alerts); err != nil {
		return nil, "", err
	}

	nextCursor := ""
	if len(alerts) > 0 {
		nextCursor = alerts[len(alerts)-1].ID.Hex()
	}

	return alerts, nextCursor, nil
}

func (s *Store) AcknowledgeAlert(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = s.db.Collection("alerts").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{"status": "acknowledged"}})
	return err
}

func (s *Store) ResolveAlert(ctx context.Context, id string) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	
	var alert Alert
	err = s.db.Collection("alerts").FindOne(ctx, bson.M{"_id": oid}).Decode(&alert)
	if err != nil {
		return err
	}

	now := time.Now()
	mttr := int64(now.Sub(alert.CreatedAt).Seconds())

	_, err = s.db.Collection("alerts").UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": bson.M{
		"status":      "resolved",
		"resolved_at": now,
		"mttr_seconds": mttr,
	}})
	return err
}

func (s *Store) GetStats(ctx context.Context, orgID string) (map[string]interface{}, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"org_id": orgID}}},
		{{Key: "$group", Value: bson.M{
			"_id":           "$severity",
			"count":         bson.M{"$sum": 1},
			"avg_mttr_sec": bson.M{"$avg": "$mttr_seconds"},
		}}},
	}

	cursor, err := s.db.Collection("alerts").Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	for _, res := range results {
		sev := res["_id"].(string)
		stats[sev] = map[string]interface{}{
			"count":        res["count"],
			"avg_mttr_sec": res["avg_mttr_sec"],
		}
	}
	return stats, nil
}

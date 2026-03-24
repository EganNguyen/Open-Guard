package repository

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/openguard/audit/pkg/models"
)

type ReadRepository struct {
	db *mongo.Database
}

func NewReadRepository(client *mongo.Client) *ReadRepository {
	return &ReadRepository{db: client.Database("audit")}
}

// FindEvents fetches events from the secondary replica.
func (r *ReadRepository) FindEvents(ctx context.Context, filter bson.M, limit int64, skip int64) ([]models.AuditEvent, error) {
	coll := r.db.Collection("audit_events")

	findOptions := options.Find()
	findOptions.SetSort(bson.D{{Key: "occurred_at", Value: -1}})
	if limit > 0 {
		findOptions.SetLimit(limit)
	}
	if skip > 0 {
		findOptions.SetSkip(skip)
	}

	cursor, err := coll.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.AuditEvent
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetIntegrityChain fetches events ordered by chain sequence for integrity checks.
func (r *ReadRepository) GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error) {
	coll := r.db.Collection("audit_events")
	
	findOptions := options.Find().SetSort(bson.D{{Key: "chain_seq", Value: 1}})
	
	cursor, err := coll.Find(ctx, bson.M{"org_id": orgID}, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.AuditEvent
	if err = cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

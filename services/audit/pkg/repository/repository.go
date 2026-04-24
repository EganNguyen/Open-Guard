package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AuditRepository struct {
	db *mongo.Database
}

func NewAuditRepository(client *mongo.Client, dbName string) *AuditRepository {
	return &AuditRepository{
		db: client.Database(dbName),
	}
}

func (r *AuditRepository) BulkWrite(ctx context.Context, events []interface{}) error {
	coll := r.db.Collection("audit_events")
	
	var models []mongo.WriteModel
	for _, event := range events {
		models = append(models, mongo.NewInsertOneModel().SetDocument(event))
	}

	opts := options.BulkWrite().SetOrdered(false) // R-07 requirement
	_, err := coll.BulkWrite(ctx, models, opts)
	return err
}

func (r *AuditRepository) GetLatestHash(ctx context.Context, orgID string) (string, int64, error) {
	coll := r.db.Collection("hash_chains")
	
	var result struct {
		Hash     string `bson:"hash"`
		Sequence int64  `bson:"sequence"`
	}
	
	err := coll.FindOne(ctx, bson.M{"org_id": orgID}).Decode(&result)
	if err == mongo.ErrNoDocuments {
		return "", 0, nil
	}
	return result.Hash, result.Sequence, err
}

func (r *AuditRepository) UpdateHashChain(ctx context.Context, orgID, newHash string) error {
	coll := r.db.Collection("hash_chains")
	
	_, err := coll.UpdateOne(
		ctx,
		bson.M{"org_id": orgID},
		bson.M{
			"$set": bson.M{"hash": newHash},
			"$inc": bson.M{"sequence": 1},
			"$setOnInsert": bson.M{"created_at": time.Now()},
		},
		options.Update().SetUpsert(true),
	)
	return err
}

func (r *AuditRepository) FindEvents(ctx context.Context, filter bson.M, limit int64, offset int64) ([]map[string]interface{}, error) {
	coll := r.db.Collection("audit_events")
	
	opts := options.Find().
		SetLimit(limit).
		SetSkip(offset).
		SetSort(bson.M{"timestamp": -1})
	
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []map[string]interface{}
	if err = cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *AuditRepository) GetLatestEvent(ctx context.Context) (map[string]interface{}, error) {
	coll := r.db.Collection("audit_events")
	
	opts := options.FindOne().
		SetSort(bson.M{"timestamp": -1})
	
	var event map[string]interface{}
	err := coll.FindOne(ctx, bson.M{}, opts).Decode(&event)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	return event, err
}

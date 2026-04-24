package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AuditWriteRepository struct {
	DB *mongo.Database
}

type AuditReadRepository struct {
	DB *mongo.Database
}

func NewAuditWriteRepository(client *mongo.Client, dbName string) *AuditWriteRepository {
	return &AuditWriteRepository{
		DB: client.Database(dbName),
	}
}

func NewAuditReadRepository(client *mongo.Client, dbName string) *AuditReadRepository {
	return &AuditReadRepository{
		DB: client.Database(dbName),
	}
}

func (r *AuditWriteRepository) BulkWrite(ctx context.Context, events []interface{}) error {
	coll := r.DB.Collection("audit_events")
	
	var models []mongo.WriteModel
	for _, event := range events {
		models = append(models, mongo.NewInsertOneModel().SetDocument(event))
	}

	opts := options.BulkWrite().SetOrdered(true) // Ordered for chain integrity
	_, err := coll.BulkWrite(ctx, models, opts)
	return err
}

func (r *AuditWriteRepository) ReserveSequence(ctx context.Context, orgID string, count int64) (startSeq int64, prevHash string, err error) {
	coll := r.DB.Collection("hash_chains")
	var result struct {
		Hash     string `bson:"hash"`
		Sequence int64  `bson:"sequence"`
	}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	err = coll.FindOneAndUpdate(ctx,
		bson.M{"org_id": orgID},
		bson.M{
			"$inc": bson.M{"sequence": count},
			"$setOnInsert": bson.M{"hash": "", "created_at": time.Now()},
		},
		opts,
	).Decode(&result)
	if err != nil {
		return 0, "", err
	}
	return result.Sequence - count, result.Hash, nil // startSeq is BEFORE increment
}

func (r *AuditWriteRepository) UpdateHashChain(ctx context.Context, orgID, newHash string) error {
	coll := r.DB.Collection("hash_chains")
	
	_, err := coll.UpdateOne(
		ctx,
		bson.M{"org_id": orgID},
		bson.M{
			"$set": bson.M{"hash": newHash},
		},
	)
	return err
}

func (r *AuditReadRepository) FindEvents(ctx context.Context, filter bson.M, limit int64, offset int64) ([]map[string]interface{}, error) {
	coll := r.DB.Collection("audit_events")
	
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

func (r *AuditReadRepository) GetLatestHash(ctx context.Context, orgID string) (string, int64, error) {
	coll := r.DB.Collection("hash_chains")
	
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

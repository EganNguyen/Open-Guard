package repository

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/openguard/audit/pkg/models"
)

type Repository struct {
	db *mongo.Database
}

func New(client *mongo.Client) *Repository {
	return &Repository{db: client.Database("audit")}
}

// EnsureIndexes creates the necessary indexes on the primary.
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	coll := r.db.Collection("audit_events")

	models := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "occurred_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "type", Value: 1}, {Key: "occurred_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "actor_id", Value: 1}, {Key: "occurred_at", Value: -1}},
		},
		{
			Keys:    bson.D{{Key: "event_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "chain_seq", Value: 1}},
		},
	}

	_, err := coll.Indexes().CreateMany(ctx, models)
	return err
}

func (r *Repository) GetCollection() *mongo.Collection {
	return r.db.Collection("audit_events")
}

// FindEvents fetches events from the secondary replica.
func (r *Repository) FindEvents(ctx context.Context, filter interface{}, limit int64, skip int64) ([]models.AuditEvent, error) {
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
func (r *Repository) GetIntegrityChain(ctx context.Context, orgID string) ([]models.AuditEvent, error) {
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

// GetLastChainState fetches the last sequence number and hash for an org to continue the chain.
func (r *Repository) GetLastChainState(ctx context.Context, orgID string) (int64, string, error) {
	coll := r.db.Collection("audit_events")

	opts := options.FindOne().SetSort(bson.D{{Key: "chain_seq", Value: -1}})

	var lastEvent models.AuditEvent
	err := coll.FindOne(ctx, bson.M{"org_id": orgID}, opts).Decode(&lastEvent)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 0, "", nil
		}
		return 0, "", err
	}

	return lastEvent.ChainSeq, lastEvent.ChainHash, nil
}

// InsertMany inserts a batch of events (used by consumer).
func (r *Repository) InsertMany(ctx context.Context, events []interface{}) error {
	coll := r.db.Collection("audit_events")
	_, err := coll.InsertMany(ctx, events)
	return err
}

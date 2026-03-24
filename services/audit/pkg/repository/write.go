package repository

import (
	"context"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	
	"github.com/openguard/audit/pkg/models"
)

type WriteRepository struct {
	db *mongo.Database
}

func NewWriteRepository(client *mongo.Client) *WriteRepository {
	return &WriteRepository{db: client.Database("audit")}
}

// EnsureIndexes creates the necessary indexes on the primary.
func (r *WriteRepository) EnsureIndexes(ctx context.Context) error {
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

func (r *WriteRepository) GetCollection() *mongo.Collection {
	return r.db.Collection("audit_events")
}

// GetLastChainSeq fetches the last sequence number and hash for an org to continue the chain
func (r *WriteRepository) GetLastChainState(ctx context.Context, orgID string) (int64, string, error) {
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

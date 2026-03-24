//go:build integration
// +build integration

package consumer_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/openguard/audit/pkg/consumer"
	"github.com/openguard/audit/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestBulkWriterIntegration(t *testing.T) {
	ctx := context.Background()

	// Spin up MongoDB container
	mongodbContainer, err := mongodb.Run(ctx, "mongo:7")
	require.NoError(t, err, "failed to start mongodb container")
	defer func() {
		if err := mongodbContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate container: %s", err)
		}
	}()

	uri, err := mongodbContainer.ConnectionString(ctx)
	require.NoError(t, err)

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	require.NoError(t, err)
	defer client.Disconnect(ctx)

	db := client.Database("test_audit")
	coll := db.Collection("events")
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("Flush by MaxDocs limit", func(t *testing.T) {
		coll.Drop(ctx)
		maxDocs := 5
		writer := consumer.NewBulkWriter(coll, maxDocs, 5*time.Second, logger)
		writerCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go writer.Start(writerCtx)

		t.Log("Adding maxDocs events to trigger immediate flush")
		for i := 0; i < maxDocs; i++ {
			err := writer.Add(ctx, models.AuditEvent{
				EventID: fmt.Sprintf("evt-max-%d", i),
			})
			require.NoError(t, err)
		}

		time.Sleep(100 * time.Millisecond) // Give flush a moment to hit db

		count, err := coll.CountDocuments(ctx, bson.M{"event_id": bson.M{"$regex": "^evt-max-"}})
		require.NoError(t, err)
		assert.Equal(t, int64(maxDocs), count)
	})

	t.Run("Flush by Timeout", func(t *testing.T) {
		coll.Drop(ctx)
		flushAfter := 500 * time.Millisecond
		writer := consumer.NewBulkWriter(coll, 100, flushAfter, logger)
		writerCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go writer.Start(writerCtx)

		t.Log("Adding 1 event and waiting for timer flush")
		err := writer.Add(ctx, models.AuditEvent{
			EventID: "evt-time-1",
		})
		require.NoError(t, err)

		// Before timeout 
		count, _ := coll.CountDocuments(ctx, bson.M{})
		assert.Equal(t, int64(0), count, "should not be flushed yet")

		// Wait for timer
		time.Sleep(700 * time.Millisecond)

		// After timeout
		countAfter, _ := coll.CountDocuments(ctx, bson.M{})
		assert.Equal(t, int64(1), countAfter, "should be flushed by timer")
	})

	t.Run("Flush on Cancel", func(t *testing.T) {
		coll.Drop(ctx)
		writer := consumer.NewBulkWriter(coll, 100, 5*time.Second, logger)
		writerCtx, cancel := context.WithCancel(ctx)
		go writer.Start(writerCtx)

		err := writer.Add(ctx, models.AuditEvent{
			EventID: "evt-stop-1",
		})
		require.NoError(t, err)

		t.Log("Canceling context to trigger final flush")
		cancel()
		time.Sleep(100 * time.Millisecond)

		countAfter, _ := coll.CountDocuments(ctx, bson.M{})
		assert.Equal(t, int64(1), countAfter, "should be flushed on stop")
	})
}

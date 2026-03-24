package consumer

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openguard/audit/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MockCollection struct {
	BulkWriteFunc func(ctx context.Context, models []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error)
}

func (m *MockCollection) BulkWrite(ctx context.Context, models []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error) {
	return m.BulkWriteFunc(ctx, models, opts...)
}

func TestBulkWriter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	t.Run("flush by count", func(t *testing.T) {
		var callCount int32
		var docsCount int32
		
		mockColl := &MockCollection{
			BulkWriteFunc: func(ctx context.Context, models []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error) {
				atomic.AddInt32(&callCount, 1)
				atomic.AddInt32(&docsCount, int32(len(models)))
				return &mongo.BulkWriteResult{InsertedCount: int64(len(models))}, nil
			},
		}
		
		maxDocs := 5
		bw := NewBulkWriter(mockColl, maxDocs, 10*time.Second, logger)
		
		ctx := context.Background()
		for i := 0; i < 4; i++ {
			err := bw.Add(ctx, models.AuditEvent{EventID: "e"})
			require.NoError(t, err)
		}
		
		// Buffer has 4, should not have flushed yet
		assert.Equal(t, int32(0), atomic.LoadInt32(&callCount))
		
		// Add 5th
		err := bw.Add(ctx, models.AuditEvent{EventID: "e5"})
		require.NoError(t, err)
		
		// Should have flushed
		assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
		assert.Equal(t, int32(5), atomic.LoadInt32(&docsCount))
	})
	
	t.Run("flush by ticker", func(t *testing.T) {
		var callCount int32
		
		mockColl := &MockCollection{
			BulkWriteFunc: func(ctx context.Context, models []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error) {
				atomic.AddInt32(&callCount, 1)
				return &mongo.BulkWriteResult{InsertedCount: int64(len(models))}, nil
			},
		}
		
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		
		bw := NewBulkWriter(mockColl, 100, 50*time.Millisecond, logger)
		go bw.Start(ctx)
		
		bw.Add(ctx, models.AuditEvent{EventID: "e1"})
		
		// Wait for ticker
		time.Sleep(150 * time.Millisecond)
		
		assert.GreaterOrEqual(t, atomic.LoadInt32(&callCount), int32(1))
	})
}

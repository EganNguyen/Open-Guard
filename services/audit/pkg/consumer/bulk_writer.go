package consumer

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/openguard/audit/pkg/models"
)

type collection interface {
	BulkWrite(ctx context.Context, models []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error)
}

type BulkWriter struct {
	coll       collection
	buffer     []mongo.WriteModel
	mu         sync.Mutex
	maxDocs    int
	flushAfter time.Duration
	logger     *slog.Logger
	stopCh     chan struct{}
}

func NewBulkWriter(coll collection, maxDocs int, flushAfter time.Duration, logger *slog.Logger) *BulkWriter {
	b := &BulkWriter{
		coll:       coll,
		buffer:     make([]mongo.WriteModel, 0, maxDocs),
		maxDocs:    maxDocs,
		flushAfter: flushAfter,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
	return b
}

func (b *BulkWriter) Start(ctx context.Context) {
	ticker := time.NewTicker(b.flushAfter)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			b.flush(context.Background()) // final flush
			return
		case <-b.stopCh:
			b.flush(context.Background())
			return
		case <-ticker.C:
			b.flush(ctx)
		}
	}
}

func (b *BulkWriter) Stop() {
	close(b.stopCh)
}

func (b *BulkWriter) Add(ctx context.Context, doc models.AuditEvent) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	model := mongo.NewInsertOneModel().SetDocument(doc)
	b.buffer = append(b.buffer, model)

	if len(b.buffer) >= b.maxDocs {
		return b.flushLocked(ctx)
	}
	return nil
}

func (b *BulkWriter) flush(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked(ctx)
}

func (b *BulkWriter) flushLocked(ctx context.Context) error {
	if len(b.buffer) == 0 {
		return nil
	}

	batch := b.buffer
	b.buffer = make([]mongo.WriteModel, 0, b.maxDocs)

	opts := options.BulkWrite().SetOrdered(false)
	res, err := b.coll.BulkWrite(ctx, batch, opts)
	if err != nil {
		// Log errors gracefully since ordered=false, some docs might be duplicates (duplicate event_id)
		b.logger.WarnContext(ctx, "bulk write returned errors",
			"error", err,
			"inserted_count", func() int64 {
				if res != nil {
					return res.InsertedCount
				}
				return 0
			}(),
		)
		// We do not fail the whole batch. Duplicate event_ids will log errors but the rest will succeed.
		return nil
	}
	
	b.logger.DebugContext(ctx, "bulk write flushed", "inserted_count", res.InsertedCount)
	return nil
}

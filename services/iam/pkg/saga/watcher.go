package saga

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Watcher struct {
	rdb       *redis.Client
	publisher interface {
		Publish(ctx context.Context, topic, key string, payload []byte) error
	}
	logger *slog.Logger
}

func NewWatcher(rdb *redis.Client, publisher interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}, logger *slog.Logger) *Watcher {
	return &Watcher{rdb: rdb, publisher: publisher, logger: logger}
}

func (w *Watcher) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.checkExpired(ctx)
		}
	}
}

func (w *Watcher) checkExpired(ctx context.Context) {
	now := float64(time.Now().Unix())
	// ZRANGEBYSCORE saga:deadlines -inf <now> LIMIT 0 100
	sagaIDs, err := w.rdb.ZRangeByScore(ctx, "saga:deadlines", &redis.ZRangeBy{
		Min:    "-inf",
		Max:    fmt.Sprintf("%f", now),
		Offset: 0,
		Count:  100,
	}).Result()
	if err != nil || len(sagaIDs) == 0 {
		return
	}

	for _, sagaID := range sagaIDs {
		w.logger.Warn("saga timed out, publishing compensation", "saga_id", sagaID)
		payload, _ := json.Marshal(map[string]any{
			"event":        "user.provisioning.failed",
			"saga_id":      sagaID,
			"compensation": true,
			"reason":       "saga_timeout",
			"ts":           time.Now().Unix(),
		})
		if err := w.publisher.Publish(ctx, "saga.orchestration", sagaID, payload); err != nil {
			w.logger.Error("failed to publish saga timeout", "saga_id", sagaID, "error", err)
			continue
		}
		// Remove from sorted set after publishing
		w.rdb.ZRem(ctx, "saga:deadlines", sagaID)
	}
}

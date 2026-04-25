package saga

import (
	"context"
	"encoding/json"
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
	
	// Atomic: claim expired sagas using Lua script
	script := redis.NewScript(`
		local members = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', ARGV[1], 'LIMIT', 0, 100)
		if #members == 0 then return {} end
		redis.call('ZREM', KEYS[1], unpack(members))
		return members
	`)
	
	sagaIDs, err := script.Run(ctx, w.rdb, []string{"saga:deadlines"}, now).StringSlice()
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
			// Note: if publish fails, the saga is already removed from deadlines.
			// Ideally we should have a retry mechanism or DLQ for these.
			continue
		}
	}
}

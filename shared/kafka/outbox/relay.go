package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Relay polls the outbox table and publishes events to Kafka.
type Relay struct {
	pool      *pgxpool.Pool
	tableName string
	interval  time.Duration
	logger    *slog.Logger
}

// NewRelay creates a new outbox relay.
func NewRelay(pool *pgxpool.Pool, tableName string, interval time.Duration, logger *slog.Logger) *Relay {
	return &Relay{
		pool:      pool,
		tableName: tableName,
		interval:  interval,
		logger:    logger,
	}
}

// Run starts the relay polling loop.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("outbox relay started", "table", r.tableName)

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("outbox relay stopping")
			return
		case <-ticker.C:
			r.drain(ctx)
		}
	}
}

func (r *Relay) drain(ctx context.Context) {
	// Implementation will involve SELECT FOR UPDATE SKIP LOCKED
	// and publishing to Kafka. For now, it's a skeleton.
}

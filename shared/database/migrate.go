package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"time"
)

// RunWithLock executes the migration with a distributed Redis lock.
func RunWithLock(ctx context.Context, rdb *redis.Client, lockKey string, logger *slog.Logger, task func(ctx context.Context) error) error {
	lockTTL := 60 * time.Second

	// Acquire lock (SET NX with TTL)
	acquired, err := rdb.SetNX(ctx, lockKey, "1", lockTTL).Result()
	if err != nil {
		return fmt.Errorf("redis lock error: %w", err)
	}
	if !acquired {
		logger.Info("migration lock held by another instance, waiting...", "key", lockKey)
		// Wait and poll
		for i := 0; i < 30; i++ {
			time.Sleep(2 * time.Second)
			exists, _ := rdb.Exists(ctx, lockKey).Result()
			if exists == 0 {
				break
			}
		}
		return nil // Other instance ran migrations
	}

	// Heartbeat goroutine: extend TTL every 10s
	stopHeartbeat := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rdb.Expire(ctx, lockKey, lockTTL)
			case <-stopHeartbeat:
				return
			}
		}
	}()
	defer func() {
		close(stopHeartbeat)
		rdb.Del(ctx, lockKey)
	}()

	// Run migrations task
	return task(ctx)
}

// Migrate runs all .sql files in the specified directory against the pool.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	slog.Info("running migrations", "dir", migrationsDir)

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var sqlFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".up.sql") {
			sqlFiles = append(sqlFiles, f.Name())
		}
	}
	sort.Strings(sqlFiles)

	for _, f := range sqlFiles {
		slog.Info("applying migration", "file", f)
		content, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil {
			return fmt.Errorf("read migration file %s: %w", f, err)
		}

		_, err = pool.Exec(ctx, string(content))
		if err != nil {
			// If it's a "relation already exists" error, we might want to ignore it
			// for simple idempotent migrations, but since these are not managed by a tool,
			// we should be careful.
			if strings.Contains(err.Error(), "already exists") {
				slog.Warn("migration skipped (already applied?)", "file", f, "error", err)
				continue
			}
			return fmt.Errorf("execute migration %s: %w", f, err)
		}
	}

	slog.Info("migrations completed successfully")
	return nil
}

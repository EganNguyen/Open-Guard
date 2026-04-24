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
)

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

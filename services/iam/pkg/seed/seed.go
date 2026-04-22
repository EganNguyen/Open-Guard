package seed

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	systemOrgID = "00000000-0000-0000-0000-000000000000"
	acmeOrgID   = "11111111-1111-1111-1111-111111111111"
)

func Seed(ctx context.Context, pool *pgxpool.Pool) error {
	// 0. Ensure Tables Exist (Minimal for Phase 2)
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS orgs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT now()
		);
		
		CREATE TABLE IF NOT EXISTS users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id UUID REFERENCES orgs(id),
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'user',
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT now()
		);

		CREATE TABLE IF NOT EXISTS connectors (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			client_secret TEXT NOT NULL,
			redirect_uris TEXT[] NOT NULL,
			created_at TIMESTAMPTZ DEFAULT now()
		);

		ALTER TABLE connectors ADD COLUMN IF NOT EXISTS org_id UUID REFERENCES orgs(id);
	`)
	if err != nil {
		return fmt.Errorf("failed to ensure tables exist: %w", err)
	}

	// 1. Create Organizations
	orgs := []struct {
		id   string
		name string
		slug string
	}{
		{systemOrgID, "OpenGuard System", "openguard-system"},
		{acmeOrgID, "Acme Corp", "acme-corp"},
	}

	for _, org := range orgs {
		_, err = pool.Exec(ctx, `
			INSERT INTO orgs (id, name, slug, status)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (id) DO NOTHING
		`, org.id, org.name, org.slug, "active")
		if err != nil {
			return fmt.Errorf("failed to create org %s: %w", org.name, err)
		}
	}

	// 2. Create Users
	users := []struct {
		email   string
		pass    string
		name    string
		role    string
		orgID   string
	}{
		{"admin@openguard.io", "admin123", "System Admin", "admin", systemOrgID},
		{"test@openguard.io", "test123", "Test User", "user", systemOrgID},
		{"admin@task.io", "admin123", "Acme Admin", "admin", acmeOrgID},
	}

	for _, user := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.pass), 12)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO users (org_id, email, password_hash, display_name, role)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (email) DO UPDATE SET org_id = EXCLUDED.org_id
		`, user.orgID, user.email, string(hash), user.name, user.role)
		if err != nil {
			return fmt.Errorf("failed to create user %s: %w", user.email, err)
		}
	}

	// 3. Create Task App Connector
	_, err = pool.Exec(ctx, `
		INSERT INTO connectors (id, org_id, name, client_secret, redirect_uris)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET org_id = EXCLUDED.org_id
	`, "task-app", acmeOrgID, "Task Management App", "task-secret-123", []string{"http://localhost:3000/api/auth/callback"})
	if err != nil {
		return fmt.Errorf("failed to create task-app connector: %w", err)
	}

	// 4. Create Default Policies for Task App
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS policies (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			org_id UUID REFERENCES orgs(id),
			name TEXT NOT NULL,
			logic JSONB NOT NULL,
			version INT NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TIMESTAMPTZ DEFAULT now(),
			updated_at TIMESTAMPTZ DEFAULT now()
		);

		INSERT INTO policies (org_id, name, logic, version)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT DO NOTHING
	`, acmeOrgID, "Allow Task Management", `{"type": "allow_all"}`) // Simple allow all for the demo
	if err != nil {
		return fmt.Errorf("failed to seed policies: %w", err)
	}

	log.Println("Seeding completed successfully")
	return nil
}

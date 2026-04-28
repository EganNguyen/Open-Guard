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
	// 1. Create Organizations
	orgs := []struct {
		id   string
		name string
		slug string
	}{
		{systemOrgID, "OpenGuard System", "openguard-system"},
		{acmeOrgID, "Acme Corp", "acme-corp"},
	}
	var err error
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
		email string
		pass  string
		name  string
		role  string
		orgID string
	}{
		{"admin@openguard.io", "admin123", "System Admin", "admin", systemOrgID},
		{"test@openguard.io", "test123", "Test User", "user", systemOrgID},
		{"admin@acme.example", "changeme123!", "Acme Admin", "admin", acmeOrgID},
	}

	for _, user := range users {
		hash, err := bcrypt.GenerateFromPassword([]byte(user.pass), 12)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO users (org_id, email, password_hash, display_name, role)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (org_id, email) DO UPDATE SET 
				password_hash = EXCLUDED.password_hash,
				display_name = EXCLUDED.display_name,
				role = EXCLUDED.role,
				updated_at = NOW()
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

	log.Println("Seeding completed successfully")
	return nil
}

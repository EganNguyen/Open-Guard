package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	systemOrgID = "00000000-0000-0000-0000-000000000000"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable"
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer conn.Close(ctx)

	// 1. Create System Organization
	var orgID string
	err = conn.QueryRow(ctx, `
		INSERT INTO orgs (id, name, status)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, systemOrgID, "OpenGuard System", "active").Scan(&orgID)
	if err != nil {
		log.Fatalf("Failed to create system org: %v\n", err)
	}
	fmt.Printf("System Organization initialized: %s\n", orgID)

	// 2. Create System Admin User
	adminEmail := "admin@openguard.io"
	adminPass := "admin123" // Change this in production!

	// Hash password with cost 12 as per spec
	hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), 12)
	if err != nil {
		log.Fatalf("Failed to hash password: %v\n", err)
	}

	// Bypass RLS for seeding by using the superuser/owner role (openguard)
	// or by setting the org_id in session
	_, err = conn.Exec(ctx, fmt.Sprintf("SET app.org_id = '%s'", orgID))
	if err != nil {
		log.Fatalf("Failed to set RLS context: %v\n", err)
	}

	var userID string
	err = conn.QueryRow(ctx, `
		INSERT INTO users (org_id, email, password_hash, display_name, role, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (email) DO UPDATE SET 
			password_hash = EXCLUDED.password_hash,
			display_name = EXCLUDED.display_name,
			role = EXCLUDED.role
		RETURNING id
	`, orgID, adminEmail, string(hash), "System Administrator", "system_admin", "active").Scan(&userID)
	if err != nil {
		log.Fatalf("Failed to create admin user: %v\n", err)
	}

	fmt.Printf("System Admin user initialized: %s (%s)\n", adminEmail, userID)
}

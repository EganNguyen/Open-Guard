package repository

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// CreateOrg inserts a new organization.
func (r *Repository) CreateOrg(ctx context.Context, name string) (string, error) {
	var id string
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + uuid.New().String()[:8]
	err := r.pool.QueryRow(ctx, `
		INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id
	`, name, slug).Scan(&id)
	return id, err
}

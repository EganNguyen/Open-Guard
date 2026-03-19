package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Org represents an organization in the database.
type Org struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgRepository handles org CRUD operations.
type OrgRepository struct {
	pool *pgxpool.Pool
}

// NewOrgRepository creates a new OrgRepository.
func NewOrgRepository(pool *pgxpool.Pool) *OrgRepository {
	return &OrgRepository{pool: pool}
}

// Create inserts a new organization and returns it.
func (r *OrgRepository) Create(ctx context.Context, name, slug string) (*Org, error) {
	org := &Org{}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO orgs (name, slug) VALUES ($1, $2)
		 RETURNING id, name, slug, plan, created_at, updated_at`,
		name, slug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}
	return org, nil
}

// GetByID retrieves an org by its ID.
func (r *OrgRepository) GetByID(ctx context.Context, id string) (*Org, error) {
	org := &Org{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, slug, plan, created_at, updated_at FROM orgs WHERE id = $1`,
		id,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get org by id: %w", err)
	}
	return org, nil
}

// GetBySlug retrieves an org by its slug.
func (r *OrgRepository) GetBySlug(ctx context.Context, slug string) (*Org, error) {
	org := &Org{}
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, slug, plan, created_at, updated_at FROM orgs WHERE slug = $1`,
		slug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get org by slug: %w", err)
	}
	return org, nil
}

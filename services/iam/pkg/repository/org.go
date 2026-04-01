package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openguard/shared/rls"
)

type Org struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Plan      string    `json:"plan"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r *Repository) CreateOrg(ctx context.Context, tx pgx.Tx, name, slug string) (*Org, error) {
	if err := rls.SetSessionVar(ctx, tx, ""); err != nil { // system operation
		return nil, fmt.Errorf("rls config: %w", err)
	}

	org := &Org{}
	err := tx.QueryRow(ctx,
		`INSERT INTO orgs (name, slug) VALUES ($1, $2)
		 RETURNING id, name, slug, plan, created_at, updated_at`,
		name, slug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}
	return org, nil
}

func (r *Repository) GetOrgByID(ctx context.Context, tx pgx.Tx, id string) (*Org, error) {
	if err := rls.SetSessionVar(ctx, tx, ""); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	org := &Org{}
	err := tx.QueryRow(ctx,
		`SELECT id, name, slug, plan, created_at, updated_at FROM orgs WHERE id = $1`,
		id,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get org by id: %w", err)
	}
	return org, nil
}

func (r *Repository) GetOrgBySlug(ctx context.Context, tx pgx.Tx, slug string) (*Org, error) {
	if err := rls.SetSessionVar(ctx, tx, ""); err != nil {
		return nil, fmt.Errorf("rls config: %w", err)
	}

	org := &Org{}
	err := tx.QueryRow(ctx,
		`SELECT id, name, slug, plan, created_at, updated_at FROM orgs WHERE slug = $1`,
		slug,
	).Scan(&org.ID, &org.Name, &org.Slug, &org.Plan, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get org by slug: %w", err)
	}
	return org, nil
}

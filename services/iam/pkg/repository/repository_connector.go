package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/shared/rls"
)

// GetConnectorByID returns a connector by ID.
func (r *Repository) GetConnectorByID(ctx context.Context, id string) (*Connector, error) {
	orgID := rls.OrgID(ctx)
	var name, secret string
	var orgIDRes *string
	var uris []string
	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		return conn.QueryRow(ctx, `
			SELECT org_id, name, client_secret, redirect_uris FROM connectors WHERE id = $1
		`, id).Scan(&orgIDRes, &name, &secret, &uris)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Connector{
		ID:           id,
		OrgID:        orgIDRes,
		Name:         name,
		ClientSecret: secret,
		RedirectURIs: uris,
	}, nil
}

// ListConnectors lists all connectors visible in the current context.
func (r *Repository) ListConnectors(ctx context.Context) ([]Connector, error) {
	orgID := rls.OrgID(ctx)
	isSystem := orgID == "00000000-0000-0000-0000-000000000000"
	var connectors []Connector

	if isSystem {
		conn, err := r.pool.Acquire(ctx)
		if err != nil {
			return nil, err
		}
		defer conn.Release()

		_, err = conn.Exec(ctx, "SET ROLE openguard_login")
		if err != nil {
			return nil, fmt.Errorf("set login role: %w", err)
		}
		defer func() { _, _ = conn.Exec(ctx, "RESET ROLE") }()

		rows, err := conn.Query(ctx, `SELECT id, org_id, name, client_secret, redirect_uris FROM connectors`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var id, name, secret string
			var orgIDStr *string
			var uris []string
			if err := rows.Scan(&id, &orgIDStr, &name, &secret, &uris); err != nil {
				return nil, err
			}
			connectors = append(connectors, Connector{
				ID:           id,
				OrgID:        orgIDStr,
				Name:         name,
				ClientSecret: secret,
				RedirectURIs: uris,
			})
		}
		return connectors, nil
	}

	err := r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		rows, err := conn.Query(ctx, `
			SELECT id, org_id, name, client_secret, redirect_uris FROM connectors
		`)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var id, name, secret string
			var orgIDStr *string
			var uris []string
			if err := rows.Scan(&id, &orgIDStr, &name, &secret, &uris); err != nil {
				return err
			}
			connectors = append(connectors, Connector{
				ID:           id,
				OrgID:        orgIDStr,
				Name:         name,
				ClientSecret: secret,
				RedirectURIs: uris,
			})
		}
		return nil
	})
	return connectors, err
}

// CreateConnector creates a new connector and its associated org.
func (r *Repository) CreateConnector(ctx context.Context, id, name, secret string, uris []string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var orgID string
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "-" + id
	err = tx.QueryRow(ctx, `
		INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id
	`, name, slug).Scan(&orgID)
	if err != nil {
		return "", err
	}

	if err := rls.TxSetSessionVar(ctx, tx, orgID); err != nil {
		return "", err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO connectors (id, org_id, name, client_secret, redirect_uris)
		VALUES ($1, $2, $3, $4, $5)
	`, id, orgID, name, secret, uris)
	if err != nil {
		return "", err
	}

	return orgID, tx.Commit(ctx)
}

// UpdateConnector updates a connector's name and redirect URIs.
func (r *Repository) UpdateConnector(ctx context.Context, id, name string, uris []string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			UPDATE connectors SET name = $2, redirect_uris = $3 WHERE id = $1
		`, id, name, uris)
		return err
	})
}

// DeleteConnector deletes a connector by ID.
func (r *Repository) DeleteConnector(ctx context.Context, id string) error {
	orgID := rls.OrgID(ctx)
	return r.withOrgContext(ctx, orgID, func(ctx context.Context, conn *pgxpool.Conn) error {
		_, err := conn.Exec(ctx, `
			DELETE FROM connectors WHERE id = $1
		`, id)
		return err
	})
}

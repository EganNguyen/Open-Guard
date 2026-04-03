package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/openguard/todoapp/pkg/db"
)

type Todo struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	UserID      string    `json:"user_id"`
	Title       string    `json:"title"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"created_at"`
}

type Repository struct {
	db *db.DB
}

func NewRepository(db *db.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, orgID string, todo *Todo) error {
	return r.db.ExecuteWithRLS(ctx, orgID, func(tx pgx.Tx) error {
		query := `INSERT INTO todos (org_id, user_id, title, completed, created_at)
		          VALUES ($1, $2, $3, $4, $5) RETURNING id`
		return tx.QueryRow(ctx, query, orgID, todo.UserID, todo.Title, todo.Completed, todo.CreatedAt).Scan(&todo.ID)
	})
}

func (r *Repository) List(ctx context.Context, orgID string, userID string) ([]Todo, error) {
	todos := []Todo{} // initialize to empty slice so JSON encodes as [] not null
	err := r.db.ExecuteWithRLS(ctx, orgID, func(tx pgx.Tx) error {
		query := `SELECT id, org_id, user_id, title, completed, created_at FROM todos WHERE user_id = $1`
		rows, err := tx.Query(ctx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t Todo
			if err := rows.Scan(&t.ID, &t.OrgID, &t.UserID, &t.Title, &t.Completed, &t.CreatedAt); err != nil {
				return err
			}
			todos = append(todos, t)
		}
		return nil
	})
	return todos, err
}

func (r *Repository) Update(ctx context.Context, orgID string, todoID string, completed bool) error {
	return r.db.ExecuteWithRLS(ctx, orgID, func(tx pgx.Tx) error {
		query := `UPDATE todos SET completed = $1 WHERE id = $2`
		_, err := tx.Exec(ctx, query, completed, todoID)
		return err
	})
}

func (r *Repository) Delete(ctx context.Context, orgID string, todoID string) error {
	return r.db.ExecuteWithRLS(ctx, orgID, func(tx pgx.Tx) error {
		query := `DELETE FROM todos WHERE id = $1`
		_, err := tx.Exec(ctx, query, todoID)
		return err
	})
}

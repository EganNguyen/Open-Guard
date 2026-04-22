package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://openguard:openguard@localhost:5432/openguard?sslmode=disable"
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	if err := initDB(ctx, pool); err != nil {
		log.Fatalf("failed to init db: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Phase 2: Simple OIDC Auth Middleware
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			// Simplified token check for Phase 2 demo
			if token != "skeleton-token" {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), "user_id", "00000000-0000-0000-0000-000000000000")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	r.Route("/api/tasks", func(r chi.Router) {
		r.Use(authMiddleware)

		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value("user_id").(string)
			var tasks []map[string]interface{}
			rows, err := pool.Query(r.Context(), "SELECT id, title, status FROM tasks WHERE owner_id = $1", userID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			for rows.Next() {
				var id, title, status string
				rows.Scan(&id, &title, &status)
				tasks = append(tasks, map[string]interface{}{"id": id, "title": title, "status": status})
			}
			json.NewEncoder(w).Encode(tasks)
		})

		r.Post("/", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value("user_id").(string)
			var body struct {
				Title string `json:"title"`
			}
			json.NewDecoder(r.Body).Decode(&body)

			var id string
			err := pool.QueryRow(r.Context(), "INSERT INTO tasks (title, owner_id) VALUES ($1, $2) RETURNING id", body.Title, userID).Scan(&id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": id})
		})

		r.Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value("user_id").(string)
			taskID := chi.URLParam(r, "id")
			var body struct {
				Status string `json:"status"`
			}
			json.NewDecoder(r.Body).Decode(&body)

			_, err := pool.Exec(r.Context(), "UPDATE tasks SET status = $1, updated_at = now() WHERE id = $2 AND owner_id = $3", body.Status, taskID, userID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})

		r.Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value("user_id").(string)
			taskID := chi.URLParam(r, "id")

			_, err := pool.Exec(r.Context(), "DELETE FROM tasks WHERE id = $1 AND owner_id = $2", taskID, userID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3005"
	}
	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		log.Printf("Task Backend starting on port %s", port)
		srv.ListenAndServe()
	}()
	<-ctx.Done()
	srv.Shutdown(context.Background())
}

func initDB(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS tasks (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			owner_id UUID NOT NULL,
			created_at TIMESTAMPTZ DEFAULT now(),
			updated_at TIMESTAMPTZ DEFAULT now()
		);
	`)
	return err
}

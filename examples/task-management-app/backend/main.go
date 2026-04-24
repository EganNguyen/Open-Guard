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
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/sdk"
	"github.com/openguard/shared/crypto"
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

	// ── OpenGuard SDK Initialization ──────────────────────────────────────────
	policyURL := os.Getenv("OPENGUARD_POLICY_URL")
	if policyURL == "" {
		policyURL = "http://localhost:8083"
	}
	apiKey := os.Getenv("OPENGUARD_API_KEY")
	if apiKey == "" {
		apiKey = "ogk_dev_key_12345678" // Default for dev
	}
	og := sdk.NewClient(policyURL, apiKey)

	// Auth setup (still needed for token verification)
	keyringJSON := os.Getenv("OPENGUARD_JWT_KEYS")
	if keyringJSON == "" {
		keyringJSON = `[{"kid":"dev-key","secret":"dev-secret-at-least-32-chars-long-!!","algorithm":"HS256","status":"active"}]`
	}
	keyring, _ := crypto.LoadKeyring(keyringJSON)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

type contextKey string

const (
	userIDKey contextKey = "user_id"
	orgIDKey  contextKey = "org_id"
)

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			
			claims := &crypto.StandardClaims{}
			_, err := crypto.Verify(token, keyring, claims)
			if err != nil {
				go ingestEvent(context.Background(), og, "auth.login.failure", "", "", "", map[string]any{
					"ip": r.RemoteAddr, "error": err.Error(),
				})
				http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
				return
			}

			go ingestEvent(context.Background(), og, "auth.login.success", claims.UserID, claims.OrgID, "", map[string]any{
				"ip": r.RemoteAddr, "user_agent": r.UserAgent(),
			})

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			ctx = context.WithValue(ctx, orgIDKey, claims.OrgID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	policyMiddleware := func(action string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				userID, _ := r.Context().Value(userIDKey).(string)

				// Use SDK for policy evaluation (R-15)
				allowed, err := og.Allow(r.Context(), userID, action, "task:*")
				if err != nil {
					http.Error(w, "Policy evaluation failed", http.StatusServiceUnavailable)
					return
				}

				if !allowed {
					orgID, _ := r.Context().Value(orgIDKey).(string)
					taskID := chi.URLParam(r, "id")
					go ingestEvent(context.Background(), og, "access.denied", userID, orgID, taskID, map[string]any{
						"action": action,
					})
					http.Error(w, "Access Denied by OpenGuard Policy", http.StatusForbidden)
					return
				}

				next.ServeHTTP(w, r)
			})
		}
	}

	r.Route("/api/tasks", func(r chi.Router) {
		r.Use(authMiddleware)

		r.With(policyMiddleware("task:list")).Get("/", func(w http.ResponseWriter, r *http.Request) {
			userID, _ := r.Context().Value(userIDKey).(string)
			orgID, _ := r.Context().Value(orgIDKey).(string)
			var tasks []map[string]interface{}
			// In production, we'd filter by org_id too
			rows, err := pool.Query(r.Context(), "SELECT id, title, status FROM tasks WHERE owner_id = $1", userID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = orgID // use it in queries for multi-tenancy
			defer rows.Close()
			for rows.Next() {
				var id, title, status string
				rows.Scan(&id, &title, &status)
				tasks = append(tasks, map[string]interface{}{"id": id, "title": title, "status": status})
			}

			if len(tasks) > 50 {
				go ingestEvent(context.Background(), og, "data.bulk.read", userID, orgID, "tasks", map[string]any{
					"count":              len(tasks),
					"threshold_exceeded": true,
				})
			}

			json.NewEncoder(w).Encode(tasks)
		})

		r.With(policyMiddleware("task:create")).Post("/", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value(userIDKey).(string)
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

		r.With(policyMiddleware("task:update")).Put("/{id}", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value(userIDKey).(string)
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

		r.With(policyMiddleware("task:delete")).Delete("/{id}", func(w http.ResponseWriter, r *http.Request) {
			userID := r.Context().Value(userIDKey).(string)
			taskID := chi.URLParam(r, "id")

			_, err := pool.Exec(r.Context(), "DELETE FROM tasks WHERE id = $1 AND owner_id = $2", taskID, userID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			orgID, _ := r.Context().Value(orgIDKey).(string)
			go ingestEvent(context.Background(), og, "resource.delete", userID, orgID, taskID, nil)

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

func ingestEvent(ctx context.Context, og *sdk.Client, eventType, userID, orgID, resourceID string, metadata map[string]any) {
	event := sdk.AuditEvent{
		EventType:  eventType,
		UserID:     userID,
		OrgID:      orgID,
		ResourceID: resourceID,
		Metadata:   metadata,
		Timestamp:  time.Now().UTC(),
	}
	// IngestEvent must not block the main request path
	if err := og.IngestEvent(ctx, event); err != nil {
		log.Printf("WARN: failed to ingest security event type=%s error=%v", eventType, err)
	}
}

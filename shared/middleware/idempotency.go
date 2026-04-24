package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

const idempotencyTTL = 24 * time.Hour

// IdempotencyMiddleware deduplicates POST/PUT/PATCH requests that carry
// an `Idempotency-Key` header. Cached responses are stored in Redis for 24h.
// On replay: returns the cached status code + body without re-executing the handler.
func IdempotencyMiddleware(rdb *redis.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Namespace by org_id to prevent cross-tenant replay
			orgID := GetOrgID(r.Context())
			cacheKey := "idem:" + orgID + ":" + hashKey(key)

			ctx := r.Context()
			if cached, err := rdb.Get(ctx, cacheKey).Bytes(); err == nil {
				var entry idempotencyEntry
				if json.Unmarshal(cached, &entry) == nil {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Idempotency-Replayed", "true")
					w.WriteHeader(entry.StatusCode)
					w.Write(entry.Body)
					return
				}
			}

			// Capture response
			rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Cache only 2xx responses
			if rec.statusCode >= 200 && rec.statusCode < 300 {
				entry := idempotencyEntry{StatusCode: rec.statusCode, Body: rec.body}
				if b, err := json.Marshal(entry); err == nil {
					rdb.Set(ctx, cacheKey, b, idempotencyTTL)
				}
			}
		})
	}
}

type idempotencyEntry struct {
	StatusCode int    `json:"status_code"`
	Body       []byte `json:"body"`
}

func hashKey(k string) string {
	h := sha256.Sum256([]byte(k))
	return hex.EncodeToString(h[:])
}

// responseRecorder captures status + body for caching.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

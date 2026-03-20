package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/openguard/shared/models"
)

// PolicyClient handles evaluation requests to the Policy service.
type PolicyClient struct {
	addr   string
	client *http.Client
	logger *slog.Logger
}

func NewPolicyClient(addr string, logger *slog.Logger) *PolicyClient {
	return &PolicyClient{
		addr:   addr,
		client: &http.Client{Timeout: 2 * time.Second},
		logger: logger,
	}
}

type EvalRequest struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	Context  map[string]string `json:"context"`
}

type EvalResponse struct {
	Result bool `json:"result"`
}

// PolicyMiddleware returns a middleware that evaluates access via the Policy service.
func (pc *PolicyClient) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			
			// Assume JWTAuth has already run and populated context
			userData, ok := ctx.Value("user").(map[string]interface{})
			if !ok {
				// If not authenticated, skip policy check or fail based on config.
				// For public routes, this middleware shouldn't be reached.
				next.ServeHTTP(w, r)
				return
			}

			orgID, _ := userData["org_id"].(string)
			userID, _ := userData["user_id"].(string)

			if orgID == "" {
				pc.logger.Warn("missing org_id in context for policy check")
				http.Error(w, "Unauthorized: missing tenant context", http.StatusUnauthorized)
				return
			}

			// Prepare evaluation request
			evalReq := EvalRequest{
				Action:   r.Method,
				Resource: r.URL.Path,
				Context: map[string]string{
					"ip": r.RemoteAddr,
					"ua": r.UserAgent(),
				},
			}

			body, _ := json.Marshal(evalReq)
			req, err := http.NewRequestWithContext(ctx, "POST", pc.addr+"/policies/evaluate", bytes.NewBuffer(body))
			if err != nil {
				pc.logger.Error("failed to create policy evaluation request", "error", err)
				pc.failClosed(w, r)
				return
			}

			// Heatlhcheck-standard headers
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Org-ID", orgID)
			req.Header.Set("X-User-ID", userID)

			resp, err := pc.client.Do(req)
			if err != nil {
				pc.logger.Error("policy service unavailable - failing closed", "error", err)
				pc.failClosed(w, r)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				pc.logger.Warn("policy service returned error", "status", resp.StatusCode)
				pc.failClosed(w, r)
				return
			}

			var evalResp EvalResponse
			if err := json.NewDecoder(resp.Body).Decode(&evalResp); err != nil {
				pc.logger.Error("failed to decode policy response", "error", err)
				pc.failClosed(w, r)
				return
			}

			if !evalResp.Result {
				pc.logger.Info("access denied by policy engine", "user_id", userID, "org_id", orgID, "path", r.URL.Path)
				pc.deny(w, r)
				return
			}

			// Access granted
			next.ServeHTTP(w, r)
		})
	}
}

func (pc *PolicyClient) failClosed(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusForbidden, "POLICY_ERROR", "Security check failed: engine unavailable", r)
}

func (pc *PolicyClient) deny(w http.ResponseWriter, r *http.Request) {
	models.WriteError(w, http.StatusForbidden, "POLICY_DENIED", "Access denied by security policy", r)
}

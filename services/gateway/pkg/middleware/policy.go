package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/openguard/shared/models"
	"github.com/openguard/shared/resilience"
	"github.com/sony/gobreaker"
)

// PolicyClient handles evaluation requests to the Policy service.
type PolicyClient struct {
	addr    string
	client  *http.Client
	breaker *gobreaker.CircuitBreaker
	logger  *slog.Logger
}

func NewPolicyClient(addr string, logger *slog.Logger) *PolicyClient {
	return &PolicyClient{
		addr:   addr,
		client: &http.Client{Timeout: 2 * time.Second},
		breaker: resilience.NewBreaker(resilience.BreakerConfig{
			Name:             "policy-service",
			Timeout:          2 * time.Second,
			MaxRequests:      3,
			Interval:         10 * time.Second,
			FailureThreshold: 5,
			OpenDuration:     30 * time.Second,
		}),
		logger: logger,
	}
}

type EvalRequest struct {
	OrgID      string `json:"org_id"`
	UserID     string `json:"user_id"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	IPAddress  string `json:"ip_address,omitempty"`
}

type EvalResponse struct {
	Permitted bool `json:"permitted"`
}

// PolicyMiddleware returns a middleware that evaluates access via the Policy service.
func (pc *PolicyClient) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			
			// Assume JWTAuth has already run and populated headers
			orgID := r.Header.Get("X-Org-ID")
			userID := r.Header.Get("X-User-ID")

			pc.logger.Debug("PolicyMiddleware check", "org_id", orgID, "user_id", userID, "headers", r.Header)

			if orgID == "" {
				pc.logger.Warn("missing org_id in context for policy check")
				models.WriteError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing org context", r)
				return
			}

			// Prepare evaluation request
			evalReq := EvalRequest{
				OrgID:      orgID,
				UserID:     userID,
				Action:     r.Method,
				Resource:   r.URL.Path,
				IPAddress:  r.RemoteAddr,
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

			pc.logger.Debug("PolicyClient sending evaluation request", "org_id", orgID, "user_id", userID, "action", evalReq.Action, "resource", evalReq.Resource)

			resp, err := resilience.Call(ctx, pc.breaker, 2*time.Second, func(reqCtx context.Context) (*http.Response, error) {
				req = req.WithContext(reqCtx)
				return pc.client.Do(req)
			})
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

			if !evalResp.Permitted {
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

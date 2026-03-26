package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// In-memory data store for the Todo app
var (
	todosStore = make(map[string][]Todo) // Keyed by UserID
	storeMutex sync.RWMutex
	nextID     = 1
)

type Todo struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Completed bool      `json:"completed"`
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	CreatedAt time.Time `json:"created_at"`
}

// EvalRequest matches the Control Plane Policy Evaluation SDK signature
type EvalRequest struct {
	Action   string `json:"action"`
	Resource string `json:"resource"`
	OrgID    string `json:"org_id"`
	Context  struct {
		UserID    string   `json:"user_id"`
		UserRoles []string `json:"user_roles"`
	} `json:"context"`
}

type EvalResponse struct {
	Permitted bool   `json:"permitted"`
	Reason    string `json:"reason,omitempty"`
}

// EnsureOpenGuardAuth is a middleware for connected apps (like TodoApp).
// It extracts the user's JWT, parses the identity, and calls the Control Plane
// to verify the action against the dynamic policy engine.
func EnsureOpenGuardAuth(action string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error": "Missing Bearer token"}`, http.StatusUnauthorized)
			return
		}
		
		token := strings.TrimPrefix(authHeader, "Bearer ")
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			// For local testing, if it's not a real JWT, we parse it as a dummy string
			// but normally we expect a 3-part JWT.
			http.Error(w, `{"error": "Invalid JWT format"}`, http.StatusUnauthorized)
			return
		}

		// Decode JWT Payload without verification (IAM verifies signature in production, 
		// but since we are a connected app, we should either verify JWKS or trust our gateway.
		// For this example, we just parse the sub/org claims).
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			http.Error(w, `{"error": "Failed to decode JWT payload"}`, http.StatusUnauthorized)
			return
		}

		var claims map[string]interface{}
		json.Unmarshal(payload, &claims)

		userID, _ := claims["sub"].(string)
		orgID, _ := claims["org_id"].(string)
		
		if userID == "" {
			userID = "demo-user" // fallback for demo purposes
		}

		// Call Control Plane to evaluate Policy!
		evalReq := EvalRequest{
			Action:   action,
			Resource: "todos",
			OrgID:    orgID,
		}
		evalReq.Context.UserID = userID

		bodyBytes, _ := json.Marshal(evalReq)
		req, _ := http.NewRequest("POST", "http://localhost:8080/v1/policy/evaluate", bytes.NewReader(bodyBytes))
		
		// The Todo App authenticates itself to the Control Plane using its Connector API Key
		req.Header.Set("Authorization", "Bearer my-connector-api-key")
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(req)
		
		if err != nil || resp.StatusCode != 200 {
			log.Printf("Control Plane lookup failed or denied: %v", err)
			http.Error(w, `{"error": "Access Denied by Control Plane Policy"}`, http.StatusForbidden)
			return
		}

		var evalResp EvalResponse
		json.NewDecoder(resp.Body).Decode(&evalResp)
		if !evalResp.Permitted {
			http.Error(w, fmt.Sprintf(`{"error": "Denied by OpenGuard Policy: %s"}`, evalResp.Reason), http.StatusForbidden)
			return
		}

		r.Header.Set("Authenticated-User", userID)
		r.Header.Set("Authenticated-Org", orgID)

		next.ServeHTTP(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func getTodos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	userID := r.Header.Get("Authenticated-User")

	storeMutex.RLock()
	todos := todosStore[userID]
	storeMutex.RUnlock()

	if todos == nil {
		todos = []Todo{}
	}
	writeJSON(w, http.StatusOK, todos)
}

func addTodo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	userID := r.Header.Get("Authenticated-User")
	orgID := r.Header.Get("Authenticated-Org")

	storeMutex.Lock()
	t := Todo{
		ID:        nextID,
		Title:     payload.Title,
		Completed: false,
		UserID:    userID,
		OrgID:     orgID,
		CreatedAt: time.Now(),
	}
	nextID++
	todosStore[userID] = append(todosStore[userID], t)
	storeMutex.Unlock()

	writeJSON(w, http.StatusCreated, t)
}

func main() {
	mux := http.NewServeMux()

	// Serve the static HTML UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	// Wrap endpoints with OpenGuard Control Plane Middleware
	// Action depends on the method (GET = read, POST = write)
	mux.HandleFunc("/api/v1/todos", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			EnsureOpenGuardAuth("read", getTodos)(w, r)
		} else if r.Method == http.MethodPost {
			EnsureOpenGuardAuth("write", addTodo)(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	log.Println("[TodoApp] Starting on http://localhost:8082")
	log.Println("[TodoApp] Ready. Using OpenGuard SDK pattern to call Control Plane (port 8080).")

	if err := http.ListenAndServe(":8082", mux); err != nil {
		log.Fatal(err)
	}
}

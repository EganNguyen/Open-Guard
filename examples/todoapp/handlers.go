package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

func handleTodos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		getTodos(w, r)
	case http.MethodPost:
		createTodo(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func getTodos(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Authenticated-User")

	storeMutex.RLock()
	userTodos, exists := todosStore[userID]
	storeMutex.RUnlock()

	if !exists {
		userTodos = []Todo{}
	}

	writeJSON(w, http.StatusOK, userTodos)
}

func createTodo(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Authenticated-User")
	orgID := r.Header.Get("Authenticated-Org")

	var input struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(input.Title) == "" {
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}

	newTodo := Todo{
		Title:     input.Title,
		Completed: false,
		UserID:    userID,
		OrgID:     orgID,
		CreatedAt: time.Now(),
	}

	storeMutex.Lock()
	newTodo.ID = nextID
	nextID++
	todosStore[userID] = append(todosStore[userID], newTodo)
	storeMutex.Unlock()

	log.Printf("[TodoApp] Created new Todo for User: %s (Org: %s) -> %s", userID, orgID, input.Title)

	writeJSON(w, http.StatusCreated, newTodo)
}

func main() {
	mux := http.NewServeMux()

	// Serve the Simple Web UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})

	// Wrap our routes with the OpenGuard Enforcer Middleware
	mux.HandleFunc("/api/v1/todos", RequireOpenGuardHeaders(handleTodos))

	port := ":8081"
	log.Printf("[TodoApp] Starting on http://localhost%s (Protected Mode)", port)
	log.Printf("[TodoApp] Awaiting requests proxied by OpenGuard Gateway...")
	
	server := &http.Server{
		Addr:    port,
		Handler: mux,
	}
	
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

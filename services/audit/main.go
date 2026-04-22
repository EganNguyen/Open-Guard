package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type Event struct {
	ID        string                 `json:"id"`
	Source    string                 `json:"source"`
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"`
	Target    string                 `json:"target"`
	Data      map[string]interface{} `json:"data"`
	Timestamp string                 `json:"timestamp"`
}

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "OK"})
	})

	http.HandleFunc("/v1/events/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var event Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// For now, we just log the event. In Phase 3, we'll store it in ClickHouse/Kafka.
		fmt.Printf("[AUDIT] Ingested event: %s | Source: %s | Action: %s | Actor: %s\n", 
			event.ID, event.Source, event.Action, event.Actor)

		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Audit Service starting on port %s\n", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		os.Exit(1)
	}
}

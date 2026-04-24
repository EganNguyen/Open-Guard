package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"golang.org/x/sync/semaphore"
	"github.com/openguard/services/compliance/pkg/repository"
	"github.com/openguard/shared/middleware"
)

type ComplianceHandler struct {
	repo     *repository.Repository
	bulkhead *semaphore.Weighted
}

func NewComplianceHandler(repo *repository.Repository, concurrency int64) *ComplianceHandler {
	if concurrency == 0 {
		concurrency = 5
	}
	return &ComplianceHandler{
		repo:     repo,
		bulkhead: semaphore.NewWeighted(concurrency),
	}
}

func (h *ComplianceHandler) GetPosture(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	posture, err := h.repo.GetPosture(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(posture)
}

func (h *ComplianceHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	reports, err := h.repo.ListReports(r.Context(), orgID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reports)
}

func (h *ComplianceHandler) CreateReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	
	var req struct {
		Framework string `json:"framework"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	jobID := uuid.New().String()
	report := repository.ComplianceReport{
		ID:        jobID,
		OrgID:     orgID,
		Framework: req.Framework,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	if err := h.repo.CreateReportJob(r.Context(), report); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Start async generation
	go h.generateReport(context.Background(), jobID, orgID, req.Framework)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"job_id": jobID})
}

func (h *ComplianceHandler) GetReportStatus(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id := mux.Vars(r)["id"]

	report, err := h.repo.GetReport(r.Context(), orgID, id)
	if err != nil {
		http.Error(w, "Report not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (h *ComplianceHandler) DownloadReport(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	id := mux.Vars(r)["id"]

	report, err := h.repo.GetReport(r.Context(), orgID, id)
	if err != nil || report.Status != "ready" {
		http.Error(w, "Report not ready or not found", http.StatusNotFound)
		return
	}

	// In a real implementation, this would stream the PDF from S3/MinIO
	// For this stub, we'll return a mock PDF stream
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"compliance-report-%s.pdf\"", id))
	w.Write([]byte("%PDF-1.4\n%...mock pdf content..."))
}

func (h *ComplianceHandler) generateReport(ctx context.Context, jobID, orgID, framework string) {
	// Bulkhead check
	if err := h.bulkhead.Acquire(ctx, 1); err != nil {
		fmt.Printf("Bulkhead full, cannot generate report %s\n", jobID)
		return
	}
	defer h.bulkhead.Release(1)

	// Update status to generating
	h.repo.UpdateReportStatus(ctx, jobID, "generating", "")

	// Simulate heavy PDF generation
	time.Sleep(10 * time.Second)

	downloadURL := fmt.Sprintf("/v1/compliance/reports/%s/download", jobID)
	h.repo.UpdateReportStatus(ctx, jobID, "ready", downloadURL)
}

package handlers

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jung-kurt/gofpdf"

	"github.com/openguard/services/compliance/pkg/repository"
	"github.com/openguard/services/compliance/pkg/storage"
	"github.com/openguard/shared/middleware"
	"github.com/openguard/shared/resilience"
)

type ComplianceHandler struct {
	repo       *repository.Repository
	bulkhead   *resilience.Bulkhead
	storage    *storage.S3Storage
	signingKey *rsa.PrivateKey
}

func NewComplianceHandler(repo *repository.Repository, bulkhead *resilience.Bulkhead, storage *storage.S3Storage) *ComplianceHandler {
	h := &ComplianceHandler{
		repo:     repo,
		bulkhead: bulkhead,
		storage:  storage,
	}

	// Load signing key from environment
	keyPath := os.Getenv("COMPLIANCE_SIGNING_KEY_PATH")
	if keyPath != "" {
		keyBytes, err := os.ReadFile(keyPath)
		if err == nil {
			block, _ := pem.Decode(keyBytes)
			if block != nil {
				key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
				if err == nil {
					h.signingKey = key
				}
			}
		}
	}

	return h
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

func (h *ComplianceHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	to := time.Now()
	from := to.AddDate(0, 0, -30)

	if fromStr != "" {
		if f, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = f
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t
		}
	}

	stats, err := h.repo.GetStats(r.Context(), orgID, from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
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
		UpdatedAt: time.Now(),
	}

	if err := h.repo.CreateReportJob(r.Context(), report); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Try to execute via bulkhead
	err := h.bulkhead.Execute(context.Background(), func() error {
		go h.generateReport(context.Background(), jobID, orgID, req.Framework)
		return nil
	})

	if err != nil {
		if err == resilience.ErrBulkheadFull {
			w.Header().Set("Retry-After", "30")
			http.Error(w, "Bulkhead full, try again later", http.StatusTooManyRequests)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

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

	url, err := h.storage.GetPresignedURL(r.Context(), report.S3Key, 1*time.Hour)
	if err != nil {
		http.Error(w, "Failed to generate download URL", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, url, http.StatusFound)
}

func (h *ComplianceHandler) generateReport(ctx context.Context, jobID, orgID, framework string) {
	// Update status to generating
	h.repo.UpdateReportStatus(ctx, jobID, "generating", "", "", "")

	// 1. Generate real PDF
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 16)
	pdf.Cell(40, 10, fmt.Sprintf("%s Compliance Report", framework))
	pdf.Ln(12)
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(40, 10, fmt.Sprintf("Organization: %s", orgID))
	pdf.Ln(8)
	pdf.Cell(40, 10, fmt.Sprintf("Report ID: %s", jobID))
	pdf.Ln(8)
	pdf.Cell(40, 10, fmt.Sprintf("Generated At: %s", time.Now().Format(time.RFC3339)))
	pdf.Ln(12)

	// Section 1: Executive Summary
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "1. Executive Summary")
	pdf.Ln(10)
	pdf.SetFont("Arial", "", 12)
	pdf.MultiCell(0, 5, "This report provides an overview of compliance status and audit trails...", "", "", false)
	pdf.Ln(10)

	// Section 2: Control Compliance
	pdf.SetFont("Arial", "B", 14)
	pdf.Cell(40, 10, "2. Control Compliance")
	pdf.Ln(10)
	posture, _ := h.repo.GetPosture(ctx, orgID)
	for k, v := range posture {
		pdf.SetFont("Arial", "", 12)
		pdf.Cell(40, 10, fmt.Sprintf("%s Score: %.2f%%", k, v))
		pdf.Ln(8)
	}
	pdf.Ln(10)

	// ... other sections would be added here

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		h.repo.UpdateReportStatus(ctx, jobID, "failed", "", "", err.Error())
		return
	}

	pdfBytes := buf.Bytes()

	// 2. RSA-PSS Signing
	var sig []byte
	if h.signingKey != nil {
		hash := sha256.Sum256(pdfBytes)
		var err error
		sig, err = rsa.SignPSS(rand.Reader, h.signingKey, crypto.SHA256, hash[:], nil)
		if err != nil {
			fmt.Printf("Signing failed: %v\n", err)
		}
	}

	// 3. Upload to S3
	s3Key := fmt.Sprintf("reports/%s/%s.pdf", orgID, jobID)
	if err := h.storage.Upload(ctx, s3Key, pdfBytes); err != nil {
		h.repo.UpdateReportStatus(ctx, jobID, "failed", "", "", fmt.Sprintf("Upload failed: %v", err))
		return
	}

	s3SigKey := ""
	if sig != nil {
		s3SigKey = s3Key + ".sig"
		h.storage.Upload(ctx, s3SigKey, sig)
	}

	// 4. Update status to ready
	h.repo.UpdateReportStatus(ctx, jobID, "ready", s3Key, s3SigKey, "")
}
func (h *ComplianceHandler) StartWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reports, err := h.repo.GetPendingReports(ctx)
			if err != nil {
				fmt.Printf("Worker failed to list pending reports: %v\n", err)
				continue
			}

			for _, r := range reports {
				// Process via bulkhead to avoid overloading
				err := h.bulkhead.Execute(ctx, func() error {
					h.generateReport(ctx, r.ID, r.OrgID, r.Framework)
					return nil
				})
				if err != nil {
					fmt.Printf("Worker failed to dispatch report %s: %v\n", r.ID, err)
				}
			}
		}
	}
}

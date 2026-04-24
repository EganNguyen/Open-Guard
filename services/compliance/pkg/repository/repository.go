package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

type Event struct {
	EventID    string    `ch:"event_id"`
	Type       string    `ch:"type"`
	OrgID      string    `ch:"org_id"`
	ActorID    string    `ch:"actor_id"`
	ActorType  string    `ch:"actor_type"`
	OccurredAt time.Time `ch:"occurred_at"`
	Source     string    `ch:"source"`
	Payload    string    `ch:"payload"`
}

type ComplianceReport struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Framework   string    `json:"framework"` // GDPR, SOC2, HIPAA
	Status      string    `json:"status"`    // pending, generating, ready, failed
	DownloadURL string    `json:"download_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Repository struct {
	conn clickhouse.Conn
}

func NewRepository(addr string) (*Repository, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Database: "default",
			Username: "default",
			Password: "",
		},
	})
	if err != nil {
		return nil, err
	}
	return &Repository{conn: conn}, nil
}

func (r *Repository) InitSchema(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS events (
			event_id     String        CODEC(ZSTD(3)),
			type         LowCardinality(String),
			org_id       String        CODEC(ZSTD(3)),
			actor_id     String        CODEC(ZSTD(3)),
			actor_type   LowCardinality(String),
			occurred_at  DateTime64(3, 'UTC'),
			source       LowCardinality(String),
			payload      String        CODEC(ZSTD(3))
		) ENGINE = ReplacingMergeTree(occurred_at)
		PARTITION BY toYYYYMMDD(occurred_at)
		ORDER BY (org_id, type, occurred_at, event_id)
		TTL occurred_at + INTERVAL 2 YEAR
		SETTINGS index_granularity = 8192;`,
		
		`CREATE TABLE IF NOT EXISTS compliance_reports (
			id           String,
			org_id       String,
			framework    String,
			status       String,
			download_url String,
			created_at   DateTime64(3, 'UTC')
		) ENGINE = ReplacingMergeTree()
		ORDER BY (org_id, created_at, id);`,
	}

	for _, q := range queries {
		if err := r.conn.Exec(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) IngestEvents(ctx context.Context, events []Event) error {
	batch, err := r.conn.PrepareBatch(ctx, "INSERT INTO events")
	if err != nil {
		return err
	}

	for _, e := range events {
		if err := batch.Append(
			e.EventID,
			e.Type,
			e.OrgID,
			e.ActorID,
			e.ActorType,
			e.OccurredAt,
			e.Source,
			e.Payload,
		); err != nil {
			return err
		}
	}

	return batch.Send()
}

func (r *Repository) GetPosture(ctx context.Context, orgID string) (map[string]float64, error) {
	// Mock posture logic: calculate scores based on event types present
	// In reality, this would be complex SQL aggregations
	posture := map[string]float64{
		"GDPR":  85.5,
		"SOC2":  72.0,
		"HIPAA": 91.2,
	}
	return posture, nil
}

func (r *Repository) CreateReportJob(ctx context.Context, report ComplianceReport) error {
	return r.conn.Exec(ctx, 
		"INSERT INTO compliance_reports (id, org_id, framework, status, download_url, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		report.ID, report.OrgID, report.Framework, report.Status, report.DownloadURL, report.CreatedAt,
	)
}

func (r *Repository) UpdateReportStatus(ctx context.Context, id, status, downloadURL string) error {
	// ClickHouse ReplacingMergeTree handles updates by inserting same PK with newer values
	// But we need to fetch the existing record first or use ALTER TABLE UPDATE which is slow.
	// For this stub implementation, we'll just insert a new record with the same ID.
	var report ComplianceReport
	err := r.conn.QueryRow(ctx, "SELECT framework, org_id, created_at FROM compliance_reports WHERE id = ?", id).Scan(&report.Framework, &report.OrgID, &report.CreatedAt)
	if err != nil {
		return err
	}
	
	return r.conn.Exec(ctx, 
		"INSERT INTO compliance_reports (id, org_id, framework, status, download_url, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, report.OrgID, report.Framework, status, downloadURL, report.CreatedAt,
	)
}

func (r *Repository) GetReport(ctx context.Context, orgID, id string) (*ComplianceReport, error) {
	var report ComplianceReport
	err := r.conn.QueryRow(ctx, 
		"SELECT id, org_id, framework, status, download_url, created_at FROM compliance_reports WHERE id = ? AND org_id = ? LIMIT 1",
		id, orgID,
	).Scan(&report.ID, &report.OrgID, &report.Framework, &report.Status, &report.DownloadURL, &report.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *Repository) ListReports(ctx context.Context, orgID string) ([]ComplianceReport, error) {
	rows, err := r.conn.Query(ctx, 
		"SELECT id, org_id, framework, status, download_url, created_at FROM compliance_reports WHERE org_id = ? ORDER BY created_at DESC",
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ComplianceReport
	for rows.Next() {
		var report ComplianceReport
		if err := rows.Scan(&report.ID, &report.OrgID, &report.Framework, &report.Status, &report.DownloadURL, &report.CreatedAt); err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

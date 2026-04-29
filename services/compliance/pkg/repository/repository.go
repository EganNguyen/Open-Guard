package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/jackc/pgx/v5/pgxpool"
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
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	Framework string    `json:"framework"` // gdpr, soc2, hipaa
	Status    string    `json:"status"`    // pending, generating, ready, failed
	S3Key     string    `json:"s3_key,omitempty"`
	S3SigKey  string    `json:"s3_sig_key,omitempty"`
	ErrorMsg  string    `json:"error_msg,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Stats struct {
	Type  string `json:"type"`
	Total uint64 `json:"total"`
}

type Repository struct {
	chConn clickhouse.Conn
	pgPool *pgxpool.Pool
}

func NewRepository(chAddr string, pgPool *pgxpool.Pool) (*Repository, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{chAddr},
		Auth: clickhouse.Auth{
			Database: getEnvOrDefault("CLICKHOUSE_DB", "default"),
			Username: getEnvOrDefault("CLICKHOUSE_USER", "default"),
			Password: os.Getenv("CLICKHOUSE_PASSWORD"), // Must be non-empty in production
		},
	})
	if err != nil {
		return nil, err
	}
	return &Repository{
		chConn: conn,
		pgPool: pgPool,
	}, nil
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
		TTL toDateTime(occurred_at) + INTERVAL 2 YEAR
		SETTINGS index_granularity = 8192;`,

		`CREATE MATERIALIZED VIEW IF NOT EXISTS event_counts_daily
		ENGINE = SummingMergeTree()
		PARTITION BY toYYYYMM(day)
		ORDER BY (org_id, type, day)
		AS SELECT org_id, type, toDate(occurred_at) AS day, count() AS cnt
		FROM events
		GROUP BY org_id, type, day;`,

		`CREATE TABLE IF NOT EXISTS alert_stats (
			org_id       String,
			day          Date,
			severity     LowCardinality(String),
			count        UInt64,
			mttr_seconds UInt64
		) ENGINE = SummingMergeTree()
		ORDER BY (org_id, day, severity);`,
	}

	for _, q := range queries {
		if err := r.chConn.Exec(ctx, q); err != nil {
			return fmt.Errorf("clickhouse init failed: %w", err)
		}
	}
	return nil
}

func (r *Repository) IngestEvents(ctx context.Context, events []Event) error {
	batch, err := r.chConn.PrepareBatch(ctx, "INSERT INTO events")
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

func (r *Repository) GetStats(ctx context.Context, orgID string, from, to time.Time) ([]Stats, error) {
	rows, err := r.chConn.Query(ctx,
		`SELECT type, sum(cnt) as total
		 FROM event_counts_daily
		 WHERE org_id = ? AND day BETWEEN ? AND ?
		 GROUP BY type ORDER BY total DESC`,
		orgID, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []Stats
	for rows.Next() {
		var s Stats
		if err := rows.Scan(&s.Type, &s.Total); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, nil
}

func (r *Repository) GetPosture(ctx context.Context, orgID string) (map[string]float64, error) {
	// Real posture calculation: presence of audit events over the last 30 days
	// This is a simplified version of control compliance scoring
	query := `
		SELECT
			countIf(type LIKE 'auth.%') as auth_events,
			countIf(type LIKE 'policy.%') as policy_events,
			countIf(type LIKE 'data.access%') as access_events,
			countIf(type LIKE 'threat.%') as threat_events
		FROM events FINAL
		WHERE org_id = ? AND occurred_at > now() - INTERVAL 30 DAY
	`
	var auth, policy, access, threat uint64
	err := r.chConn.QueryRow(ctx, query, orgID).Scan(&auth, &policy, &access, &threat)
	if err != nil {
		return nil, err
	}

	// Calculate scores (normalized to 100)
	score := func(count uint64) float64 {
		if count > 100 {
			return 100.0
		}
		return float64(count)
	}

	posture := map[string]float64{
		"GDPR":  (score(auth)*0.3 + score(access)*0.7),
		"SOC2":  (score(auth)*0.2 + score(policy)*0.4 + score(threat)*0.4),
		"HIPAA": (score(auth)*0.2 + score(access)*0.6 + score(threat)*0.2),
	}

	return posture, nil
}

func (r *Repository) CreateReportJob(ctx context.Context, report ComplianceReport) error {
	_, err := r.pgPool.Exec(ctx,
		"INSERT INTO reports (id, org_id, framework, status, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)",
		report.ID, report.OrgID, report.Framework, report.Status, report.CreatedAt, report.UpdatedAt,
	)
	return err
}

func (r *Repository) UpdateReportStatus(ctx context.Context, id, status, s3Key, s3SigKey, errMsg string) error {
	_, err := r.pgPool.Exec(ctx,
		"UPDATE reports SET status = $1, s3_key = $2, s3_sig_key = $3, error_msg = $4, updated_at = $5 WHERE id = $6",
		status, s3Key, s3SigKey, errMsg, time.Now(), id,
	)
	return err
}

func (r *Repository) GetReport(ctx context.Context, orgID, id string) (*ComplianceReport, error) {
	var report ComplianceReport
	err := r.pgPool.QueryRow(ctx,
		"SELECT id, org_id, framework, status, s3_key, s3_sig_key, error_msg, created_at, updated_at FROM reports WHERE id = $1 AND org_id = $2",
		id, orgID,
	).Scan(&report.ID, &report.OrgID, &report.Framework, &report.Status, &report.S3Key, &report.S3SigKey, &report.ErrorMsg, &report.CreatedAt, &report.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (r *Repository) ListReports(ctx context.Context, orgID string) ([]ComplianceReport, error) {
	rows, err := r.pgPool.Query(ctx,
		"SELECT id, org_id, framework, status, s3_key, s3_sig_key, error_msg, created_at, updated_at FROM reports WHERE org_id = $1 ORDER BY created_at DESC",
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ComplianceReport
	for rows.Next() {
		var report ComplianceReport
		if err := rows.Scan(&report.ID, &report.OrgID, &report.Framework, &report.Status, &report.S3Key, &report.S3SigKey, &report.ErrorMsg, &report.CreatedAt, &report.UpdatedAt); err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}
func (r *Repository) GetPendingReports(ctx context.Context) ([]ComplianceReport, error) {
	rows, err := r.pgPool.Query(ctx,
		"SELECT id, org_id, framework, status, s3_key, s3_sig_key, error_msg, created_at, updated_at FROM reports WHERE status = 'pending' ORDER BY created_at ASC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reports []ComplianceReport
	for rows.Next() {
		var report ComplianceReport
		if err := rows.Scan(&report.ID, &report.OrgID, &report.Framework, &report.Status, &report.S3Key, &report.S3SigKey, &report.ErrorMsg, &report.CreatedAt, &report.UpdatedAt); err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

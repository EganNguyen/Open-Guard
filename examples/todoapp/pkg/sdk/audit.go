package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type AuditClient struct {
	addr        string
	apiKey      string
	channel     chan AuditEvent
	batchSize   int
	interval    time.Duration
	httpClient  *http.Client
	logger      *slog.Logger
	stopChannel chan struct{}
}

func NewAuditClient(addr, apiKey string, batchSize int, interval time.Duration, logger *slog.Logger) *AuditClient {
	return &AuditClient{
		addr:        addr,
		apiKey:      apiKey,
		channel:     make(chan AuditEvent, 1000), // Internal buffer
		batchSize:   batchSize,
		interval:    interval,
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		logger:      logger,
		stopChannel: make(chan struct{}),
	}
}

func (c *AuditClient) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	var batch []AuditEvent

	for {
		select {
		case <-ctx.Done():
			c.flush(batch)
			return
		case <-c.stopChannel:
			c.flush(batch)
			return
		case event := <-c.channel:
			batch = append(batch, event)
			if len(batch) >= c.batchSize {
				c.sendBatch(batch)
				batch = nil
				ticker.Reset(c.interval)
			}
		case <-ticker.C:
			if len(batch) > 0 {
				c.sendBatch(batch)
				batch = nil
			}
		}
	}
}

func (c *AuditClient) Stop() {
	close(c.stopChannel)
}

func (c *AuditClient) Ingest(event AuditEvent) {
	c.channel <- event
}

func (c *AuditClient) flush(batch []AuditEvent) {
	// Drain channel
	for {
		select {
		case event := <-c.channel:
			batch = append(batch, event)
		default:
			if len(batch) > 0 {
				c.sendBatch(batch)
			}
			return
		}
	}
}

func (c *AuditClient) sendBatch(batch []AuditEvent) {
	url := fmt.Sprintf("%s/v1/events/ingest", c.addr)
	body, _ := json.Marshal(batch)

	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("audit_batch_failed", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 422 {
		// DLP POLICY VIOLATION (as per spec)
		// Usually we'd handle it, but since this is async ingest, we just log it.
		// The caller should ideally handle it synchronously if they want to surface it.
		c.logger.Warn("dlp_violation_in_audit_batch", "statusCode", resp.StatusCode)
	} else if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		c.logger.Error("audit_batch_unsuccessful", "statusCode", resp.StatusCode)
	}
}

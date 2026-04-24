package deliverer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/shared/middleware"
)

type Deliverer struct {
	client *http.Client
	logger *slog.Logger
}

func NewDeliverer(logger *slog.Logger) *Deliverer {
	return &Deliverer{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

func (d *Deliverer) Deliver(ctx context.Context, target, payload, secret string) error {
	// 1. SSRF check
	if err := middleware.ValidateOutboundURL(target); err != nil {
		return fmt.Errorf("SSRF blocked: %w", err)
	}

	// 2. Sign payload (reuse alerting HMAC pattern)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// 3. POST with timeout
	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader([]byte(payload)))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OpenGuard-Signature", sig)
	req.Header.Set("X-OpenGuard-Timestamp", ts)
	req.Header.Set("X-OpenGuard-Delivery", uuid.New().String())

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("target server error: %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("target client error: %d", resp.StatusCode)
	}

	return nil
}

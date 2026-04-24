package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/shared/middleware"
)

type SIEMDeliverer struct {
	client          *http.Client
	replayTolerance int64
}

func NewSIEMDeliverer() *SIEMDeliverer {
	tolerance := int64(300) // Default 5 minutes
	if v := os.Getenv("ALERTING_SIEM_REPLAY_TOLERANCE_SECONDS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			tolerance = n
		}
	}

	return &SIEMDeliverer{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		replayTolerance: tolerance,
	}
}

func Sign(payload []byte, secret string) (sig, delivery, ts string) {
	ts = strconv.FormatInt(time.Now().Unix(), 10)
	delivery = uuid.New().String()
	mac := hmac.New(sha256.New, []byte(secret))
	// Sign over "<timestamp>.<body_bytes>" per spec §13.3
	mac.Write([]byte(ts + "." + string(payload)))
	sig = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return
}

func Verify(payload []byte, secret, sig, ts string, tolerance int64) error {
	// 1. Replay protection
	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	if time.Now().Unix()-tsInt > tolerance {
		return fmt.Errorf("request too old (replay protection)")
	}

	// 2. Signature verification
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func (d *SIEMDeliverer) Deliver(ctx context.Context, webhookURL string, secret string, payload []byte) error {
	// 1. SSRF validation (runtime)
	if err := middleware.ValidateOutboundURL(webhookURL); err != nil {
		return fmt.Errorf("SSRF blocked: %w", err)
	}

	// 2. Sign payload
	sig, delivery, ts := Sign(payload, secret)

	// 3. Prepare request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-OpenGuard-Signature", sig)
	req.Header.Set("X-OpenGuard-Delivery", delivery)
	req.Header.Set("X-OpenGuard-Timestamp", ts)

	// 4. Execute
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SIEM delivery failed with status: %d", resp.StatusCode)
	}

	return nil
}

func ValidateConfig(webhookURL string) error {
	if webhookURL == "" {
		return nil
	}
	return middleware.ValidateOutboundURL(webhookURL)
}

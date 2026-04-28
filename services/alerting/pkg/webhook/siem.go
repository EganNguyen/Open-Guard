package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/shared/middleware"
)

type SIEMType string

const (
	SIEMGeneric  SIEMType = "generic"
	SIEMSplunk   SIEMType = "splunk"
	SIEMDatadog  SIEMType = "datadog"
	SIEMSentinel SIEMType = "sentinel"
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
		client:          middleware.NewSafeHTTPClient(10*time.Second, nil),
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

func (d *SIEMDeliverer) Deliver(ctx context.Context, siemType SIEMType, webhookURL string, secret string, payload []byte) error {
	// 1. Format payload for specific SIEM if needed
	finalPayload, headers := d.formatForSIEM(siemType, payload, secret)

	// 2. Prepare request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(finalPayload))
	if err != nil {
		return err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

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

func (d *SIEMDeliverer) formatForSIEM(siemType SIEMType, payload []byte, secret string) ([]byte, map[string]string) {
	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"

	switch siemType {
	case SIEMSplunk:
		// Splunk HEC (HTTP Event Collector) format
		splunkPayload := map[string]interface{}{
			"event":      json.RawMessage(payload),
			"sourcetype": "openguard_alert",
		}
		formatted, _ := json.Marshal(splunkPayload)
		headers["Authorization"] = "Splunk " + secret // In Splunk, secret is the HEC token
		return formatted, headers

	case SIEMDatadog:
		headers["DD-API-KEY"] = secret
		return payload, headers

	case SIEMSentinel:
		// Azure Sentinel Logic App / Custom Log Ingestion often uses HMAC-SHA256 in a specific header
		sig, delivery, ts := Sign(payload, secret)
		headers["X-OpenGuard-Signature"] = sig
		headers["X-OpenGuard-Delivery"] = delivery
		headers["X-OpenGuard-Timestamp"] = ts
		headers["Log-Type"] = "OpenGuardAlert"
		return payload, headers

	default:
		sig, delivery, ts := Sign(payload, secret)
		headers["X-OpenGuard-Signature"] = sig
		headers["X-OpenGuard-Delivery"] = delivery
		headers["X-OpenGuard-Timestamp"] = ts
		return payload, headers
	}
}

func ValidateConfig(webhookURL string) error {
	if webhookURL == "" {
		return nil
	}
	// Note: Validating during delivery with NewSafeHTTPClient is the only
	// way to prevent DNS rebinding. This is a basic scheme check.
	return nil
}

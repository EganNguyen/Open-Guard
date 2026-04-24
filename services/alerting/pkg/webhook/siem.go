package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/shared/middleware"
)

type SIEMDeliverer struct {
	client *http.Client
}

func NewSIEMDeliverer() *SIEMDeliverer {
	return &SIEMDeliverer{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func Sign(payload []byte, secret string) (sig, delivery, ts string) {
	ts = strconv.FormatInt(time.Now().Unix(), 10)
	delivery = uuid.New().String()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	sig = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return
}

func (d *SIEMDeliverer) Deliver(ctx context.Context, webhookURL string, secret string, payload []byte) error {
	// 1. SSRF validation
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

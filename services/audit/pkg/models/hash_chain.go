package models

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// ChainHash computes HMAC-SHA256 of: prev_hash + event_id + org_id + type + occurred_at
func ChainHash(secret, prevHash string, event AuditEvent) string {
	mac := hmac.New(sha256.New, []byte(secret))
	
	// Create payload exactly as specified
	payload := fmt.Sprintf("%s%s%s%s%d",
		prevHash,
		event.EventID,
		event.OrgID,
		event.Type,
		event.OccurredAt.UnixNano(),
	)
	
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

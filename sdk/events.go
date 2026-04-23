package sdk

import (
	"context"
	"time"
)

type Event struct {
	ID        string                 `json:"id"`
	Source    string                 `json:"source"`
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"`
	Target    string                 `json:"target"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

func (c *Client) PushEvent(ctx context.Context, event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	// Path /v1/events/ingest per spec §12
	return c.do(ctx, "POST", "/v1/events/ingest", event, nil)
}

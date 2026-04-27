package rls

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContextHelpers(t *testing.T) {
	ctx := context.Background()
	orgID := "org-123"

	ctx = WithOrgID(ctx, orgID)
	assert.Equal(t, orgID, OrgID(ctx))
}

// Integration tests for SetSessionVar would require a real Postgres instance.
// In a real environment, we would use testcontainers-go to spin up a DB
// and verify that:
// 1. set_config is called correctly.
// 2. A table with "ENABLE ROW LEVEL SECURITY" correctly filters rows based on the 'app.org_id' setting.

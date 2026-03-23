package tenant

import (
	"context"
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestTenantContext(t *testing.T) {
	ctx := context.Background()
	assert.Empty(t, OrgIDFromContext(ctx))
	assert.Empty(t, UserIDFromContext(ctx))

	ctx1 := context.WithValue(ctx, OrgIDKey, "org-123")
	assert.Equal(t, "org-123", OrgIDFromContext(ctx1))
	assert.Empty(t, UserIDFromContext(ctx1))

	ctx2 := context.WithValue(ctx1, UserIDKey, "user-456")
	assert.Equal(t, "user-456", UserIDFromContext(ctx2))
}

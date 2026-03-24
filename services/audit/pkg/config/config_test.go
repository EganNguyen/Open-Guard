package config_test

import (
	"os"
	"testing"

	"github.com/openguard/audit/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	os.Setenv("MONGO_URI_PRIMARY", "mongodb://p")
	os.Setenv("MONGO_URI_SECONDARY", "mongodb://s")
	os.Setenv("AUDIT_HASH_CHAIN_SECRET", "secret")
	
	cfg := config.Load()
	
	assert.Equal(t, "mongodb://p", cfg.MongoURIPrimary)
	assert.Equal(t, "mongodb://s", cfg.MongoURISecondary)
	assert.Equal(t, "secret", cfg.HashChainSecret)
	assert.Equal(t, 500, cfg.BulkInsertDocs) // default
}

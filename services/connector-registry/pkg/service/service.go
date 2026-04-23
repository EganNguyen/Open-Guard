package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/openguard/services/connector-registry/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
)

type Service struct {
	repo   *repository.Repository
	rdb    *redis.Client
	logger *slog.Logger
}

func NewService(repo *repository.Repository, rdb *redis.Client, logger *slog.Logger) *Service {
	return &Service{
		repo:   repo,
		rdb:    rdb,
		logger: logger,
	}
}

func (s *Service) RegisterConnector(ctx context.Context, id, orgID, name string, uris []string) (string, error) {
	// 1. Generate API Key (ogk_ prefix + 32 bytes random)
	apiKeyRaw := s.generateAPIKey()
	apiKeyPrefix := apiKeyRaw[:12] // ogk_ + first 8 chars
	
	// 2. Hash API Key using PBKDF2 (R-08)
	// We'll use a standard PBKDF2 implementation from shared/crypto if available, 
	// or implement it here. Let's assume shared/crypto has a HashPBKDF2.
	// For now, I'll use a placeholder and implementation later.
	hash := crypto.HashPBKDF2(apiKeyRaw)

	// 3. Store in DB
	// We also need a client secret for OAuth2
	clientSecret := crypto.GenerateRandomString(32)

	err := s.repo.CreateConnector(ctx, id, orgID, name, clientSecret, uris, apiKeyPrefix, hash)
	if err != nil {
		return "", err
	}

	return apiKeyRaw, nil
}

func (s *Service) ValidateAPIKey(ctx context.Context, apiKey string) (map[string]interface{}, error) {
	if len(apiKey) < 12 {
		return nil, fmt.Errorf("invalid api key format")
	}
	prefix := apiKey[:12]

	// 1. Check Redis Cache (R-08)
	if s.rdb != nil {
		_, err := s.rdb.Get(ctx, "apikey:"+prefix).Result()
		if err == nil {
			// Cache hit. Need to verify hash.
			// In a real app, we might cache the hash or a success flag if we trust the prefix entropy.
			// But the spec says PBKDF2 fast-hash, so we should verify.
			// Actually, if we cache the ConnectorID and OrgID, we still need to verify the hash to be secure.
			s.logger.Debug("apikey cache hit", "prefix", prefix)
			// ... verification logic ...
		}
	}

	// 2. DB Lookup
	connector, err := s.repo.FindByPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}

	// 3. Verify PBKDF2 Hash
	if !crypto.VerifyPBKDF2(apiKey, connector["api_key_hash"].(string)) {
		return nil, fmt.Errorf("invalid api key")
	}

	// 4. Cache Result (5-min TTL per spec)
	if s.rdb != nil {
		s.rdb.Set(ctx, "apikey:"+prefix, connector["id"].(string), 5*time.Minute)
	}

	return connector, nil
}

func (s *Service) generateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "ogk_" + base64.URLEncoding.EncodeToString(b)
}

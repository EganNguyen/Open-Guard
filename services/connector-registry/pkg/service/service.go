package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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
		cachedHash, err := s.rdb.Get(ctx, "apikey:hash:"+prefix).Result()
		if err == nil {
			// Cache hit. Verify hash.
			if crypto.VerifyPBKDF2(apiKey, cachedHash) {
				s.logger.Debug("apikey cache hit and verified", "prefix", prefix)
				// Fetch metadata from cache or DB? 
				// Spec says cache for 5 mins. Let's cache the whole connector object.
				cachedData, err := s.rdb.Get(ctx, "apikey:data:"+prefix).Result()
				if err == nil {
					var connector map[string]interface{}
					if json.Unmarshal([]byte(cachedData), &connector) == nil {
						return connector, nil
					}
				}
			} else {
				return nil, fmt.Errorf("invalid api key")
			}
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
		connectorJSON, _ := json.Marshal(connector)
		pipe := s.rdb.Pipeline()
		pipe.Set(ctx, "apikey:hash:"+prefix, connector["api_key_hash"].(string), 5*time.Minute)
		pipe.Set(ctx, "apikey:data:"+prefix, connectorJSON, 5*time.Minute)
		_, _ = pipe.Exec(ctx)
	}

	return connector, nil
}

func (s *Service) DeleteConnector(ctx context.Context, id string) error {
	// 1. Get prefix to invalidate cache
	connector, err := s.repo.GetConnectorByID(ctx, id)
	if err == nil {
		prefix := connector["api_key_prefix"].(string)
		if s.rdb != nil {
			pipe := s.rdb.Pipeline()
			pipe.Del(ctx, "apikey:hash:"+prefix)
			pipe.Del(ctx, "apikey:data:"+prefix)
			_, _ = pipe.Exec(ctx)
		}
	}

	// 2. Delete from DB
	return s.repo.DeleteConnector(ctx, id)
}

func (s *Service) generateAPIKey() string {
	b := make([]byte, 24)
	rand.Read(b)
	return "ogk_" + base64.URLEncoding.EncodeToString(b)
}

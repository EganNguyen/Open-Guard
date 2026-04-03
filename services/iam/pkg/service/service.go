package service

import (
	"log/slog"
	"time"

	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/outbox"
)

// Service is the canonical implementation of the IAM service layer.
// It consolidates Auth and User operations into a single service type
// per the OpenGuard System Specification v2.0 (§0.3).
type Service struct {
	pool                DBPool
	repo                *repository.Repository
	outbox              *outbox.Writer
	logger              *slog.Logger
	jwtKeyring          *crypto.JWTKeyring
	aesKeyring          *crypto.AESKeyring
	jwtExpiry           time.Duration
	sessionIdleTimeout  time.Duration
	isDev               bool
}

// New creates a new instance of the unified IAM Service.
func New(
	pool DBPool,
	repo *repository.Repository,
	outbox *outbox.Writer,
	logger *slog.Logger,
	jwtKeyring *crypto.JWTKeyring,
	aesKeyring *crypto.AESKeyring,
	jwtExpiry time.Duration,
	sessionIdleTimeout time.Duration,
	isDev bool,
) *Service {
	return &Service{
		pool:               pool,
		repo:               repo,
		outbox:             outbox,
		logger:             logger,
		jwtKeyring:         jwtKeyring,
		aesKeyring:         aesKeyring,
		jwtExpiry:          jwtExpiry,
		sessionIdleTimeout: sessionIdleTimeout,
		isDev:              isDev,
	}
}
func (s *Service) GetJWTKeys() []crypto.JWTKey {
	return s.jwtKeyring.GetKeys()
}

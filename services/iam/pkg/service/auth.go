package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/models"
	"github.com/openguard/shared/outbox"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	pool       *pgxpool.Pool
	users      *repository.UserRepository
	orgs       *repository.OrgRepository
	sessions   *repository.SessionRepository
	mfa        *repository.MFARepository
	outbox     *outbox.Writer
	logger     *slog.Logger
	jwtKeyring *crypto.JWTKeyring
	aesKeyring *crypto.AESKeyring
	jwtExpiry  time.Duration
}

func NewAuthService(
	pool *pgxpool.Pool,
	users *repository.UserRepository,
	orgs *repository.OrgRepository,
	sessions *repository.SessionRepository,
	mfa *repository.MFARepository,
	outboxWriter *outbox.Writer,
	logger *slog.Logger,
	jwtKeyring *crypto.JWTKeyring,
	aesKeyring *crypto.AESKeyring,
	jwtExpiry int,
) *AuthService {
	return &AuthService{
		pool:       pool,
		users:      users,
		orgs:       orgs,
		sessions:   sessions,
		mfa:        mfa,
		outbox:     outboxWriter,
		logger:     logger,
		jwtKeyring: jwtKeyring,
		aesKeyring: aesKeyring,
		jwtExpiry:  time.Duration(jwtExpiry) * time.Second,
	}
}

type RegisterRequest struct {
	OrgName     string `json:"org_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type RegisterResponse struct {
	User  *repository.User `json:"user"`
	Org   *repository.Org  `json:"org"`
	Token string           `json:"token"`
}

func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	if req.OrgName == "" || req.Email == "" || len(req.Password) < 8 {
		return nil, fmt.Errorf("invalid inputs")
	}

	slug := slugify(req.OrgName)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	org, err := s.orgs.Create(ctx, tx, req.OrgName, slug)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	hashStr := string(hash)

	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Email
	}

	user, err := s.users.Create(ctx, tx, org.ID, req.Email, displayName, &hashStr)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	token, err := s.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("generate jwt: %w", err)
	}

	s.publishEvent(ctx, tx, kafka.TopicAuditTrail, "user.created", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &RegisterResponse{
		User:  user,
		Org:   org,
		Token: token,
	}, nil
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token        string           `json:"token"`
	RefreshToken string           `json:"refresh_token"`
	ExpiresIn    int              `json:"expires_in"`
	User         *repository.User `json:"user"`
}

func (s *AuthService) Login(ctx context.Context, req LoginRequest, ipAddress, userAgent *string) (*LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("email and password required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	user, err := s.users.GetByEmailGlobal(ctx, tx, req.Email)
	if err != nil || user.PasswordHash == nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if user.Status != string(models.UserStatusActive) {
		return nil, fmt.Errorf("user account is %s", user.Status)
	}

	token, err := s.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("generate jwt: %w", err)
	}

	refreshToken := uuid.New().String()
	hashSum := sha256.Sum256([]byte(refreshToken))
	refreshHash := hex.EncodeToString(hashSum[:])

	expiresAt := time.Now().Add(s.jwtExpiry)
	_, err = s.sessions.Create(ctx, tx, user.ID, user.OrgID, refreshHash, ipAddress, userAgent, nil, expiresAt)
	if err != nil {
		s.logger.Error("failed to create session", "error", err)
	}

	s.publishEvent(ctx, tx, kafka.TopicAuthEvents, "auth.login.success", user.OrgID, user.ID)

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &LoginResponse{
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.jwtExpiry.Seconds()),
		User:         user,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, sessionID, orgID, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.sessions.Revoke(ctx, tx, orgID, sessionID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	s.publishEvent(ctx, tx, kafka.TopicAuthEvents, "auth.logout", orgID, userID)

	return tx.Commit(ctx)
}

func (s *AuthService) generateJWT(user *repository.User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":    user.ID,
		"email":  user.Email,
		"org_id": user.OrgID,
		"tier":   user.TierIsolation,
		"iat":    now.Unix(),
		"exp":    now.Add(s.jwtExpiry).Unix(),
		"iss":    "openguard",
	}
	return s.jwtKeyring.Sign(claims)
}

func (s *AuthService) publishEvent(ctx context.Context, tx pgx.Tx, topic, eventType, orgID, actorID string) {
	if s.outbox == nil {
		return
	}

	payload, _ := json.Marshal(map[string]string{})
	envelope := models.EventEnvelope{
		ID:        uuid.New().String(),
		Type:      eventType,
		OrgID:     orgID,
		ActorID:   actorID,
		ActorType: "user",
		Source:    "iam",
		SchemaVer: "2.0",
		Payload:   payload,
	}

	if err := s.outbox.Write(ctx, tx, topic, actorID, envelope); err != nil {
		s.logger.Error("outbox write failed", "error", err)
	}
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	slug := strings.ToLower(strings.TrimSpace(s))
	slug = nonAlphanumeric.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "org-" + uuid.New().String()[:8]
	}
	return slug
}

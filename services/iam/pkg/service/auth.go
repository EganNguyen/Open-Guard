package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/models"
	"golang.org/x/crypto/bcrypt"
)

// AuthService handles registration, login, logout, and token refresh.
type AuthService struct {
	users    *repository.UserRepository
	orgs     *repository.OrgRepository
	sessions *repository.SessionRepository
	producer *kafka.Producer
	logger   *slog.Logger

	jwtSecret string
	jwtExpiry time.Duration
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	users *repository.UserRepository,
	orgs *repository.OrgRepository,
	sessions *repository.SessionRepository,
	producer *kafka.Producer,
	logger *slog.Logger,
	jwtSecret string,
	jwtExpiry int,
) *AuthService {
	return &AuthService{
		users:     users,
		orgs:      orgs,
		sessions:  sessions,
		producer:  producer,
		logger:    logger,
		jwtSecret: jwtSecret,
		jwtExpiry: time.Duration(jwtExpiry) * time.Second,
	}
}

// RegisterRequest is the input for user registration.
type RegisterRequest struct {
	OrgName     string `json:"org_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// RegisterResponse is the output for user registration.
type RegisterResponse struct {
	User  *repository.User `json:"user"`
	Org   *repository.Org  `json:"org"`
	Token string           `json:"token"`
}

// Register creates an org and an admin user, returns a JWT.
func (s *AuthService) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	// Validate inputs
	if req.OrgName == "" {
		return nil, fmt.Errorf("org_name is required")
	}
	if req.Email == "" {
		return nil, fmt.Errorf("email is required")
	}
	if req.Password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if len(req.Password) < 8 {
		return nil, fmt.Errorf("password must be at least 8 characters")
	}

	// Generate slug from org name
	slug := slugify(req.OrgName)

	// Create org
	org, err := s.orgs.Create(ctx, req.OrgName, slug)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}

	// Hash password with bcrypt cost 12
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	hashStr := string(hash)

	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Email
	}

	// Create user
	user, err := s.users.Create(ctx, org.ID, req.Email, displayName, &hashStr)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Generate JWT
	token, err := s.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("generate jwt: %w", err)
	}

	// Publish event (best-effort)
	s.publishEvent(ctx, kafka.TopicAuditTrail, "user.created", user.OrgID, user.ID)

	return &RegisterResponse{
		User:  user,
		Org:   org,
		Token: token,
	}, nil
}

// LoginRequest is the input for user login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse is the output for user login.
type LoginResponse struct {
	Token        string           `json:"token"`
	RefreshToken string           `json:"refresh_token"`
	ExpiresIn    int              `json:"expires_in"`
	User         *repository.User `json:"user"`
}

// Login authenticates a user with email/password and returns a JWT.
func (s *AuthService) Login(ctx context.Context, req LoginRequest, ipAddress, userAgent *string) (*LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	// Find user by email (globally)
	user, err := s.users.GetByEmailGlobal(ctx, req.Email)
	if err != nil {
		s.publishEvent(ctx, kafka.TopicAuthEvents, "auth.login.failure", "", "")
		return nil, fmt.Errorf("invalid credentials")
	}

	if user.PasswordHash == nil {
		s.publishEvent(ctx, kafka.TopicAuthEvents, "auth.login.failure", user.OrgID, user.ID)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Compare password
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		s.publishEvent(ctx, kafka.TopicAuthEvents, "auth.login.failure", user.OrgID, user.ID)
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check user status
	if user.Status != string(models.UserStatusActive) {
		return nil, fmt.Errorf("user account is %s", user.Status)
	}

	// Generate JWT
	token, err := s.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("generate jwt: %w", err)
	}

	// Create session
	expiresAt := time.Now().Add(s.jwtExpiry)
	session, err := s.sessions.Create(ctx, user.ID, user.OrgID, ipAddress, userAgent, expiresAt)
	if err != nil {
		s.logger.Error("failed to create session", "error", err)
	}

	// Generate refresh token (session ID)
	refreshToken := ""
	if session != nil {
		refreshToken = session.ID
	}

	// Publish login success event
	s.publishEvent(ctx, kafka.TopicAuthEvents, "auth.login.success", user.OrgID, user.ID)

	return &LoginResponse{
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.jwtExpiry.Seconds()),
		User:         user,
	}, nil
}

// Logout revokes a session.
func (s *AuthService) Logout(ctx context.Context, sessionID, orgID, userID string) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if err := s.sessions.Revoke(ctx, sessionID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	s.publishEvent(ctx, kafka.TopicAuthEvents, "auth.logout", orgID, userID)
	return nil
}

// generateJWT creates a signed JWT for the given user.
func (s *AuthService) generateJWT(user *repository.User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":    user.ID,
		"org_id": user.OrgID,
		"email":  user.Email,
		"iat":    now.Unix(),
		"exp":    now.Add(s.jwtExpiry).Unix(),
		"iss":    "openguard",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}

// publishEvent publishes an EventEnvelope to Kafka (best-effort, logs errors).
func (s *AuthService) publishEvent(ctx context.Context, topic, eventType, orgID, actorID string) {
	if s.producer == nil {
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
		SchemaVer: "1.0",
		Payload:   payload,
	}

	if err := s.producer.PublishEvent(ctx, topic, envelope); err != nil {
		s.logger.Error("failed to publish event",
			"topic", topic,
			"event_type", eventType,
			"error", err,
		)
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

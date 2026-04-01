package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/openguard/iam/pkg/repository"
	"github.com/openguard/shared/kafka"
	"github.com/openguard/shared/models"
	"github.com/openguard/shared/telemetry"
	"golang.org/x/crypto/bcrypt"
)

// Methods are now part of the unified Service struct in service.go

type RegisterInput struct {
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

func (s *Service) Register(ctx context.Context, req RegisterInput) (*RegisterResponse, error) {
	if req.OrgName == "" || req.Email == "" || len(req.Password) < 8 {
		return nil, fmt.Errorf("invalid inputs")
	}

	slug := slugify(req.OrgName)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	org, err := s.repo.CreateOrg(ctx, tx, req.OrgName, slug)
	if err != nil {
		return nil, fmt.Errorf("create org: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 4) // bcrypt.MinCost
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	hashStr := string(hash)

	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Email
	}

	user, err := s.repo.CreateUser(ctx, tx, org.ID, req.Email, displayName, &hashStr)
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

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token        string           `json:"token"`
	RefreshToken string           `json:"refresh_token"`
	ExpiresIn    int              `json:"expires_in"`
	User         *repository.User `json:"user"`
	Org          *repository.Org  `json:"org"`
}

func (s *Service) Login(ctx context.Context, req LoginInput, ipAddress, userAgent *string) (*LoginResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("email and password required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	user, err := s.repo.GetUserByEmailGlobal(ctx, tx, req.Email)
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

	expiresAt := time.Now().Add(s.sessionIdleTimeout)
	_, err = s.repo.CreateSession(ctx, tx, user.ID, user.OrgID, refreshHash, ipAddress, userAgent, nil, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	s.publishEvent(ctx, tx, kafka.TopicAuthEvents, "auth.login.success", user.OrgID, user.ID)

	org, err := s.repo.GetOrgByID(ctx, tx, user.OrgID)
	if err != nil {
		return nil, fmt.Errorf("get org: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &LoginResponse{
		Token:        token,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.jwtExpiry.Seconds()),
		User:         user,
		Org:          org,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken, orgID string, currentIP, currentUA *string) (*LoginResponse, error) {
	hashSum := sha256.Sum256([]byte(refreshToken))
	refreshHash := hex.EncodeToString(hashSum[:])

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	session, err := s.repo.GetActiveSessionByHashGlobal(ctx, tx, refreshHash)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired session: %w", err)
	}

	if orgID != "" && session.OrgID != orgID {
		return nil, fmt.Errorf("invalid session for org")
	}
	activeOrgID := session.OrgID

	riskScore := s.calculateRiskScore(session, currentIP, currentUA)
	if riskScore >= 80 {
		s.logger.Warn("suspicious session refresh attempt, revoking",
			"session_id", session.ID,
			"risk_score", riskScore,
			telemetry.SafeAttr("ip", *currentIP, s.isDev),
			telemetry.SafeAttr("ua", *currentUA, s.isDev),
		)
		if err := s.repo.RevokeSession(ctx, tx, activeOrgID, session.ID); err != nil {
			return nil, fmt.Errorf("revoke suspicious session: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
		return nil, fmt.Errorf("session revoked due to suspicious activity")
	}

	user, err := s.repo.GetUserByID(ctx, tx, activeOrgID, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	token, err := s.generateJWT(user)
	if err != nil {
		return nil, fmt.Errorf("generate jwt: %w", err)
	}

	// Rotate Refresh Token
	newRefreshToken := uuid.New().String()
	newHashSum := sha256.Sum256([]byte(newRefreshToken))
	newRefreshHash := hex.EncodeToString(newHashSum[:])

	// Reset idle clock (extend session) and update credentials
	newExpiresAt := time.Now().Add(s.sessionIdleTimeout)
	if err := s.repo.UpdateSessionCredentials(ctx, tx, activeOrgID, session.ID, newRefreshHash, currentIP, currentUA, newExpiresAt); err != nil {
		return nil, fmt.Errorf("update session credentials: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	org, err := s.repo.GetOrgByID(ctx, tx, activeOrgID)
	if err != nil {
		return nil, fmt.Errorf("get org: %w", err)
	}

	return &LoginResponse{
		Token:        token,
		RefreshToken: newRefreshToken,
		ExpiresIn:    int(s.jwtExpiry.Seconds()),
		User:         user,
		Org:          org,
	}, nil
}

func (s *Service) calculateRiskScore(session *repository.Session, currentIP, currentUA *string) int {
	score := 0

	getStr := func(sp *string) string {
		if sp == nil {
			return ""
		}
		return *sp
	}

	oldIP := getStr(session.IPAddress)
	newIP := getStr(currentIP)
	oldUA := getStr(session.UserAgent)
	newUA := getStr(currentUA)

	if oldIP != "" && newIP != "" && oldIP != newIP {
		oldParts := strings.Split(oldIP, ".")
		newParts := strings.Split(newIP, ".")
		if len(oldParts) == 4 && len(newParts) == 4 {
			if oldParts[0] != newParts[0] || oldParts[1] != newParts[1] {
				score += 40
			} else if oldParts[2] != newParts[2] {
				score += 15
			}
		} else {
			score += 40
		}
	} else if oldIP != "" && newIP == "" {
		score += 10
	} else if oldIP == "" && newIP != "" {
		score += 10
	}

	if oldUA != "" && newUA != "" && oldUA != newUA {
		families := []string{"Firefox", "Chrome", "Safari", "Edge", "PostmanRuntime", "curl"}

		oldFamily := "Unknown"
		for _, f := range families {
			if strings.Contains(oldUA, f) {
				oldFamily = f
				break 
			}
		}

		newFamily := "Unknown"
		for _, f := range families {
			if strings.Contains(newUA, f) {
				newFamily = f
				break
			}
		}

		if oldFamily != newFamily {
			score += 60
		} else {
			score += 20
		}
	}

	return score
}

func (s *Service) Logout(ctx context.Context, sessionID, orgID, userID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.repo.RevokeSession(ctx, tx, orgID, sessionID); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	s.publishEvent(ctx, tx, kafka.TopicAuthEvents, "auth.logout", orgID, userID)

	return tx.Commit(ctx)
}

func (s *Service) generateJWT(user *repository.User) (string, error) {
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

func (s *Service) publishEvent(ctx context.Context, tx pgx.Tx, topic, eventType, orgID, actorID string) {
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

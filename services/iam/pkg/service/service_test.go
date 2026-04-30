package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type MockRepository struct {
	service.Repository
	Users           map[string]*iam_repo.User
	FailedLogins    map[string]int
	LockedUntil     map[string]time.Time
	Sessions        []iam_repo.Session
	RefreshTokens   map[string]*iam_repo.RefreshToken
	RevokedFamilies map[uuid.UUID]bool
	MFAConfigs      map[string][]iam_repo.MFAConfig
}

func (m *MockRepository) GetUserByEmail(ctx context.Context, email string) (*iam_repo.User, error) {
	for _, u := range m.Users {
		if u.Email == email {
			user := *u
			if until, ok := m.LockedUntil[email]; ok {
				user.LockedUntil = &until
			}
			user.FailedLoginCount = m.FailedLogins[email]
			return &user, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockRepository) GetUserByID(ctx context.Context, id string) (*iam_repo.User, error) {
	if u, ok := m.Users[id]; ok {
		return u, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockRepository) IncrementFailedLogin(ctx context.Context, email string) (int, error) {
	m.FailedLogins[email]++
	return m.FailedLogins[email], nil
}

func (m *MockRepository) LockAccount(ctx context.Context, email string, until time.Time) error {
	m.LockedUntil[email] = until
	return nil
}

func (m *MockRepository) ResetFailedLogin(ctx context.Context, email string) error {
	m.FailedLogins[email] = 0
	delete(m.LockedUntil, email)
	return nil
}

func (m *MockRepository) ListMFAConfigs(ctx context.Context, userID string) ([]iam_repo.MFAConfig, error) {
	return m.MFAConfigs[userID], nil
}

func (m *MockRepository) CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ip string, expiresAt time.Time) error {
	m.Sessions = append(m.Sessions, iam_repo.Session{
		JTI: jti, UserAgent: userAgent, IPAddress: ip,
	})
	return nil
}

func (m *MockRepository) CreateRefreshToken(ctx context.Context, orgID, userID, tokenHash string, familyID uuid.UUID, expiresAt time.Time) error {
	if m.RefreshTokens == nil {
		m.RefreshTokens = make(map[string]*iam_repo.RefreshToken)
	}
	m.RefreshTokens[tokenHash] = &iam_repo.RefreshToken{
		ID: "rt-" + tokenHash[:8], OrgID: orgID, UserID: userID, FamilyID: familyID, ExpiresAt: expiresAt, Revoked: false,
	}
	return nil
}

func (m *MockRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*iam_repo.RefreshToken, error) {
	if rt, ok := m.RefreshTokens[tokenHash]; ok {
		res := *rt
		if m.RevokedFamilies[rt.FamilyID] {
			res.Revoked = true
		}
		return &res, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockRepository) RevokeRefreshTokenFamily(ctx context.Context, familyID uuid.UUID) error {
	if m.RevokedFamilies == nil {
		m.RevokedFamilies = make(map[uuid.UUID]bool)
	}
	m.RevokedFamilies[familyID] = true
	return nil
}

func (m *MockRepository) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	delete(m.RefreshTokens, tokenHash)
	return nil
}

func (m *MockRepository) ClaimRefreshToken(ctx context.Context, tokenHash string) (*iam_repo.RefreshToken, error) {
	rt, err := m.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if rt.Revoked || time.Now().After(rt.ExpiresAt) {
		return nil, fmt.Errorf("revoked or expired")
	}
	delete(m.RefreshTokens, tokenHash)
	rt.Revoked = true
	return rt, nil
}

func (m *MockRepository) RevokeRefreshTokenFamilyByHash(ctx context.Context, tokenHash string) error {
	if m.RevokedFamilies == nil {
		m.RevokedFamilies = make(map[uuid.UUID]bool)
	}
	if rt, ok := m.RefreshTokens[tokenHash]; ok {
		familyID := rt.FamilyID
		m.RevokedFamilies[familyID] = true
	}
	return nil
}

func (m *MockRepository) GetSessionByUserID(ctx context.Context, userID string) (*iam_repo.Session, error) {
	// Simple mock: return the last session
	if len(m.Sessions) > 0 {
		return &m.Sessions[len(m.Sessions)-1], nil
	}
	return nil, nil
}

func setup(_ *testing.T) (*service.Service, *MockRepository, *miniredis.Miniredis) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := &MockRepository{
		Users:           make(map[string]*iam_repo.User),
		FailedLogins:    make(map[string]int),
		LockedUntil:     make(map[string]time.Time),
		RefreshTokens:   make(map[string]*iam_repo.RefreshToken),
		RevokedFamilies: make(map[uuid.UUID]bool),
		MFAConfigs:      make(map[string][]iam_repo.MFAConfig),
	}
	pool := service.NewAuthWorkerPool(1, context.Background())
	keyring := []crypto.JWTKey{{Kid: "k1", Secret: "test-secret-at-least-32-bytes!!", Algorithm: "HS256", Status: "active"}}
	s := service.NewService(repo, pool, keyring, nil, rdb)
	return s, repo, mr
}

func TestLogin_SuccessWithoutMFA(t *testing.T) {
	s, repo, _ := setup(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 10)
	repo.Users["1"] = &iam_repo.User{
		ID: "1", OrgID: "org1", Email: "test@example.com", PasswordHash: string(hash), Status: "active",
	}

	user, token, err := s.Login(context.Background(), "test@example.com", "password", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "1" || token == "" {
		t.Errorf("login failed: user=%v, token=%s", user, token)
	}
}

func TestLogin_LockedAccount(t *testing.T) {
	s, repo, _ := setup(t)
	repo.Users["1"] = &iam_repo.User{
		ID: "1", OrgID: "org1", Email: "test@example.com", PasswordHash: "hash", Status: "active",
	}
	repo.LockedUntil["test@example.com"] = time.Now().Add(1 * time.Hour)

	_, _, err := s.Login(context.Background(), "test@example.com", "any", "ua", "127.0.0.1")
	// Constant-time login returns INVALID_CREDENTIALS for locked accounts too
	if err == nil || !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Errorf("expected INVALID_CREDENTIALS error, got %v", err)
	}
}

func TestLogin_LockAfterTenFailures(t *testing.T) {
	s, repo, _ := setup(t)
	repo.Users["1"] = &iam_repo.User{
		ID: "1", OrgID: "org1", Email: "test@example.com", PasswordHash: "hash", Status: "active",
	}
	repo.FailedLogins["test@example.com"] = 9

	_, _, err := s.Login(context.Background(), "test@example.com", "wrong", "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Errorf("expected INVALID_CREDENTIALS error after 10 attempts, got %v", err)
	}
	if repo.LockedUntil["test@example.com"].IsZero() {
		t.Error("account should be locked")
	}
}

func TestLogin_MFARequired_ReturnsChallengeToken(t *testing.T) {
	s, repo, mr := setup(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 10)
	repo.Users["1"] = &iam_repo.User{
		ID: "1", OrgID: "org1", Email: "test@example.com", PasswordHash: string(hash), Status: "active",
	}
	repo.MFAConfigs["1"] = []iam_repo.MFAConfig{
		{MFAType: "totp", SecretEncrypted: "enc"},
	}

	user, token, err := s.Login(context.Background(), "test@example.com", "password", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	// Our refactored Login returns a dummy user and the challenge token in the 'token' string
	if token == "" || user.ID != "1" {
		t.Errorf("expected MFA required, got user=%v, token=%s", user, token)
	}

	challengeToken := token
	if !mr.Exists("mfa_challenge:" + challengeToken) {
		t.Error("challenge token should be in redis")
	}
}

func TestRefreshToken_RevokesFamilyOnReuse(t *testing.T) {
	s, repo, _ := setup(t)
	familyID := uuid.New()
	token := "token123"
	rtHash := crypto.HashSHA256(token)

	repo.RefreshTokens[rtHash] = &iam_repo.RefreshToken{
		OrgID: "org1", UserID: "user1", FamilyID: familyID, ExpiresAt: time.Now().Add(1 * time.Hour), Revoked: true,
	}

	_, err := s.RefreshToken(context.Background(), token, "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "SESSION_COMPROMISED") {
		t.Errorf("expected SESSION_COMPROMISED error, got %v", err)
	}
	if !repo.RevokedFamilies[familyID] {
		t.Error("family should be revoked")
	}
}

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

func (m *MockRepository) GetUserByExternalID(ctx context.Context, orgID, externalID string) (*iam_repo.User, error) {
	for _, u := range m.Users {
		if u.OrgID == orgID && u.SCIMExternalID != nil && *u.SCIMExternalID == externalID {
			return u, nil
		}
	}
	return nil, fmt.Errorf("not found")
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

func TestLogin_AccountLockoutFlow(t *testing.T) {
	s, repo, _ := setup(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 10)
	repo.Users["1"] = &iam_repo.User{
		ID: "1", OrgID: "org1", Email: "lockout@example.com", PasswordHash: string(hash), Status: "active",
	}

	// 1. First 9 failures
	for i := 0; i < 9; i++ {
		_, _, err := s.Login(context.Background(), "lockout@example.com", "wrong", "ua", "127.0.0.1")
		if err == nil || !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
			t.Fatalf("expected INVALID_CREDENTIALS, got %v", err)
		}
	}
	if repo.FailedLogins["lockout@example.com"] != 9 {
		t.Errorf("expected 9 failures, got %d", repo.FailedLogins["lockout@example.com"])
	}

	// 2. 10th failure triggers lockout
	_, _, err := s.Login(context.Background(), "lockout@example.com", "wrong", "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Fatalf("expected INVALID_CREDENTIALS, got %v", err)
	}
	if repo.LockedUntil["lockout@example.com"].IsZero() {
		t.Error("account should be locked")
	}

	// 3. Login with correct password while locked should still fail
	_, _, err = s.Login(context.Background(), "lockout@example.com", "password", "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "INVALID_CREDENTIALS") {
		t.Errorf("expected INVALID_CREDENTIALS while locked, got %v", err)
	}

	// 4. Reset failures should allow login
	repo.ResetFailedLogin(context.Background(), "lockout@example.com")
	_, token, err := s.Login(context.Background(), "lockout@example.com", "password", "ua", "127.0.0.1")
	if err != nil || token == "" {
		t.Errorf("login should succeed after reset, got err=%v", err)
	}
}

func TestLogout_BlocklistsJTI(t *testing.T) {
	s, _, mr := setup(t)
	jti := "test-jti"
	expiry := time.Now().Add(1 * time.Hour)

	err := s.Logout(context.Background(), jti, expiry)
	if err != nil {
		t.Fatal(err)
	}

	if !mr.Exists("blocklist:" + jti) {
		t.Error("jti should be blocklisted in redis")
	}
}

func TestCalculateRiskScore(t *testing.T) {
	// Since calculateRiskScore is private, we can't test it directly from service_test.
	// But we can test RefreshToken which uses it.
}

func TestRefreshToken_RevokesRiskThreshold(t *testing.T) {
	s, repo, _ := setup(t)
	familyID := uuid.New()
	token := "valid-token"
	rtHash := crypto.HashSHA256(token)

	repo.RefreshTokens[rtHash] = &iam_repo.RefreshToken{
		OrgID: "org1", UserID: "user1", FamilyID: familyID, ExpiresAt: time.Now().Add(1 * time.Hour), Revoked: false,
	}
	
	// Create a session with specific UA and IP
	repo.Sessions = append(repo.Sessions, iam_repo.Session{
		JTI: "j1", UserAgent: "Chrome/120.0", IPAddress: "1.2.3.4",
	})

	// Refresh with significantly different UA and IP (Family changed: 60, Subnet changed: 40, Total: 100 > 80)
	_, err := s.RefreshToken(context.Background(), token, "Firefox/120.0", "10.0.0.1")
	
	if err == nil || !strings.Contains(err.Error(), "SESSION_REVOKED_RISK") {
		t.Errorf("expected SESSION_REVOKED_RISK, got %v", err)
	}
	if !repo.RevokedFamilies[familyID] {
		t.Error("family should be revoked due to high risk score")
	}
}

func TestRegisterUser_SCIMConflict(t *testing.T) {
	s, repo, _ := setup(t)
	extID := "ext-123"
	
	repo.Users["u1"] = &iam_repo.User{
		ID: "u1", OrgID: "org1", Email: "old@example.com", Status: "deprovisioned", SCIMExternalID: &extID,
	}

	_, _, err := s.RegisterUser(context.Background(), service.RegisterUserRequest{
		OrgID: "org1", Email: "new@example.com", SCIMExternalID: extID,
	})

	if err == nil || !strings.Contains(err.Error(), "CONFLICT:user was deprovisioned") {
		t.Errorf("expected SCIM conflict error, got %v", err)
	}
}


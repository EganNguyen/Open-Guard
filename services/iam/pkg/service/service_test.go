package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/shared/crypto"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type MockRepository struct {
	service.Repository
	Users              map[string]map[string]interface{}
	FailedLogins       map[string]int
	LockedUntil        map[string]time.Time
	Sessions           []map[string]interface{}
	RefreshTokens      map[string]map[string]interface{}
	RevokedFamilies    map[uuid.UUID]bool
	MFAConfigs         map[string][]map[string]interface{}
}

func (m *MockRepository) GetUserByEmail(ctx context.Context, email string) (map[string]interface{}, error) {
	for _, u := range m.Users {
		if u["email"] == email {
			user := make(map[string]interface{})
			for k, v := range u {
				user[k] = v
			}
			if until, ok := m.LockedUntil[email]; ok {
				user["locked_until"] = &until
			} else {
				user["locked_until"] = (*time.Time)(nil)
			}
			user["failed_login_count"] = m.FailedLogins[email]
			return user, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockRepository) GetUserByID(ctx context.Context, id string) (map[string]interface{}, error) {
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

func (m *MockRepository) ListMFAConfigs(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	return m.MFAConfigs[userID], nil
}

func (m *MockRepository) CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ip string, expiresAt time.Time) error {
	m.Sessions = append(m.Sessions, map[string]interface{}{
		"org_id": orgID, "user_id": userID, "jti": jti,
	})
	return nil
}

func (m *MockRepository) CreateRefreshToken(ctx context.Context, orgID, userID, tokenHash string, familyID uuid.UUID, expiresAt time.Time) error {
	if m.RefreshTokens == nil {
		m.RefreshTokens = make(map[string]map[string]interface{})
	}
	m.RefreshTokens[tokenHash] = map[string]interface{}{
		"org_id": orgID, "user_id": userID, "family_id": familyID, "expires_at": expiresAt, "revoked": false,
	}
	return nil
}

func (m *MockRepository) GetRefreshToken(ctx context.Context, tokenHash string) (map[string]interface{}, error) {
	if rt, ok := m.RefreshTokens[tokenHash]; ok {
		res := make(map[string]interface{})
		for k, v := range rt {
			res[k] = v
		}
		if m.RevokedFamilies[rt["family_id"].(uuid.UUID)] {
			res["revoked"] = true
		}
		return res, nil
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

func (m *MockRepository) ClaimRefreshToken(ctx context.Context, tokenHash string) (map[string]interface{}, error) {
	rt, err := m.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if rt["revoked"].(bool) || time.Now().After(rt["expires_at"].(time.Time)) {
		return nil, fmt.Errorf("revoked or expired")
	}
	delete(m.RefreshTokens, tokenHash)
	return rt, nil
}

func (m *MockRepository) RevokeRefreshTokenFamilyByHash(ctx context.Context, tokenHash string) error {
	if m.RevokedFamilies == nil {
		m.RevokedFamilies = make(map[uuid.UUID]bool)
	}
	if rt, ok := m.RefreshTokens[tokenHash]; ok {
		familyID := rt["family_id"].(uuid.UUID)
		m.RevokedFamilies[familyID] = true
	}
	return nil
}

func setup(_ *testing.T) (*service.Service, *MockRepository, *miniredis.Miniredis) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := &MockRepository{
		Users:           make(map[string]map[string]interface{}),
		FailedLogins:    make(map[string]int),
		LockedUntil:     make(map[string]time.Time),
		RefreshTokens:   make(map[string]map[string]interface{}),
		RevokedFamilies: make(map[uuid.UUID]bool),
		MFAConfigs:      make(map[string][]map[string]interface{}),
	}
	pool := service.NewAuthWorkerPool(1, context.Background())
	keyring := []crypto.JWTKey{{Kid: "k1", Secret: "test-secret-at-least-32-bytes!!", Algorithm: "HS256", Status: "active"}}
	s := service.NewService(repo, pool, keyring, nil, rdb)
	return s, repo, mr
}

func TestLogin_SuccessWithoutMFA(t *testing.T) {
	s, repo, _ := setup(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 10)
	repo.Users["1"] = map[string]interface{}{
		"id": "1", "org_id": "org1", "email": "test@example.com", "password_hash": string(hash), "status": "active",
	}

	user, token, err := s.Login(context.Background(), "test@example.com", "password", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if user["id"] != "1" || token == "" {
		t.Errorf("login failed: user=%v, token=%s", user, token)
	}
}

func TestLogin_LockedAccount(t *testing.T) {
	s, repo, _ := setup(t)
	repo.Users["1"] = map[string]interface{}{
		"id": "1", "org_id": "org1", "email": "test@example.com", "password_hash": "hash", "status": "active",
	}
	repo.LockedUntil["test@example.com"] = time.Now().Add(1 * time.Hour)

	_, _, err := s.Login(context.Background(), "test@example.com", "any", "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Errorf("expected locked error, got %v", err)
	}
}

func TestLogin_LockAfterTenFailures(t *testing.T) {
	s, repo, _ := setup(t)
	repo.Users["1"] = map[string]interface{}{
		"id": "1", "org_id": "org1", "email": "test@example.com", "password_hash": "hash", "status": "active",
	}
	repo.FailedLogins["test@example.com"] = 9

	_, _, err := s.Login(context.Background(), "test@example.com", "wrong", "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "locked due to too many failed attempts") {
		t.Errorf("expected lock error after 10 attempts, got %v", err)
	}
	if repo.LockedUntil["test@example.com"].IsZero() {
		t.Error("account should be locked")
	}
}

func TestLogin_MFARequired_ReturnsChallengeToken(t *testing.T) {
	s, repo, mr := setup(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 10)
	repo.Users["1"] = map[string]interface{}{
		"id": "1", "org_id": "org1", "email": "test@example.com", "password_hash": string(hash), "status": "active",
	}
	repo.MFAConfigs["1"] = []map[string]interface{}{
		{"mfa_type": "totp", "secret_encrypted": "enc"},
	}

	res, token, err := s.Login(context.Background(), "test@example.com", "password", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if res["mfa_required"] != true || res["mfa_challenge"] == "" {
		t.Errorf("expected MFA required, got %v", res)
	}
	if token != "" {
		t.Error("access token should be empty when MFA is required")
	}
	
	challengeToken := res["mfa_challenge"].(string)
	if !mr.Exists("mfa_challenge:" + challengeToken) {
		t.Error("challenge token should be in redis")
	}
}

func TestRefreshToken_RevokesFamilyOnReuse(t *testing.T) {
	s, repo, _ := setup(t)
	familyID := uuid.New()
	token := "token123"
	rtHash := crypto.HashSHA256(token)
	
	repo.RefreshTokens[rtHash] = map[string]interface{}{
		"org_id": "org1", "user_id": "user1", "family_id": familyID, "expires_at": time.Now().Add(1 * time.Hour), "revoked": true,
	}

	_, err := s.RefreshToken(context.Background(), token, "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "SESSION_COMPROMISED") {
		t.Errorf("expected SESSION_COMPROMISED error, got %v", err)
	}
	if !repo.RevokedFamilies[familyID] {
		t.Error("family should be revoked")
	}
}

package service_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/services/iam/pkg/service"
	"github.com/openguard/shared/crypto"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

type MockSession struct {
	iam_repo.Session
	UserID string
}

type MockTx struct {
	pgx.Tx
}

func (m *MockTx) Commit(ctx context.Context) error   { return nil }
func (m *MockTx) Rollback(ctx context.Context) error { return nil }

type MockRepository struct {
	service.Repository
	Users           map[string]*iam_repo.User
	FailedLogins    map[string]int
	LockedUntil     map[string]time.Time
	Sessions        map[string]*MockSession
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

func (m *MockRepository) CreateSession(ctx context.Context, orgID, userID, jti, userAgent, ipAddress string, expiresAt time.Time) error {
	m.Sessions[jti] = &MockSession{
		Session: iam_repo.Session{
			JTI:       jti,
			UserAgent: userAgent,
			IPAddress: ipAddress,
		},
		UserID: userID,
	}
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
	rt, ok := m.RefreshTokens[tokenHash]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if rt.Revoked || time.Now().After(rt.ExpiresAt) || m.RevokedFamilies[rt.FamilyID] {
		return nil, fmt.Errorf("revoked or expired")
	}
	rt.Revoked = true
	return rt, nil
}

func (m *MockRepository) RevokeRefreshTokenFamilyByHash(ctx context.Context, tokenHash string) error {
	if m.RevokedFamilies == nil {
		m.RevokedFamilies = make(map[uuid.UUID]bool)
	}
	if rt, ok := m.RefreshTokens[tokenHash]; ok {
		m.RevokedFamilies[rt.FamilyID] = true
	} else {
		// Even if not found in tokens map, we should still handle the case if we have family info
		// For the mock, we assume the test will provide it if needed.
	}
	return nil
}

func (m *MockRepository) GetSessionByUserID(ctx context.Context, userID string) (*iam_repo.Session, error) {
	for _, s := range m.Sessions {
		if s.UserID == userID {
			return &s.Session, nil
		}
	}
	return nil, nil
}

func (m *MockRepository) UpsertMFAConfig(ctx context.Context, orgID, userID, mfaType, secret string) error {
	if m.MFAConfigs == nil {
		m.MFAConfigs = make(map[string][]iam_repo.MFAConfig)
	}
	m.MFAConfigs[userID] = append(m.MFAConfigs[userID], iam_repo.MFAConfig{
		MFAType: mfaType, SecretEncrypted: secret,
	})
	return nil
}

func (m *MockRepository) EnableUserMFA(ctx context.Context, userID string, enabled bool, method string) error {
	if u, ok := m.Users[userID]; ok {
		u.MFAEnabled = enabled
		u.MFAMethod = method
		return nil
	}
	return fmt.Errorf("user not found")
}

func (m *MockRepository) GetMFAConfig(ctx context.Context, userID, mfaType string) (*iam_repo.MFAConfig, error) {
	configs := m.MFAConfigs[userID]
	for _, c := range configs {
		if c.MFAType == mfaType {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (m *MockRepository) CreateUser(ctx context.Context, orgID, email, passwordHash, displayName, role, status string) (string, error) {
	id := uuid.New().String()
	m.Users[id] = &iam_repo.User{
		ID: id, OrgID: orgID, Email: email, PasswordHash: passwordHash, DisplayName: displayName, Role: role, Status: status,
	}
	return id, nil
}

func (m *MockRepository) UpdateUserSCIM(ctx context.Context, userID, externalID, status string) error {
	if u, ok := m.Users[userID]; ok {
		u.Status = status
		return nil
	}
	return fmt.Errorf("not found")
}

func (m *MockRepository) GetUserByExternalID(ctx context.Context, orgID, externalID string) (*iam_repo.User, error) {
	return nil, nil
}


func (m *MockRepository) StoreBackupCodes(ctx context.Context, userID string, hashes []string) error {
	return nil
}

func (m *MockRepository) SaveWebAuthnCredential(ctx context.Context, orgID, userID string, cred iam_repo.WebAuthnCredential) error {
	return nil
}

func (m *MockRepository) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return &MockTx{}, nil
}

func (m *MockRepository) CreateOutboxEvent(ctx context.Context, tx pgx.Tx, orgID, topic, key string, payload []byte) error {
	return nil
}

func (m *MockRepository) GetActiveJTIs(ctx context.Context, userID string) ([]string, error) {
	var jtis []string
	for jti, s := range m.Sessions {
		if s.UserID == userID {
			jtis = append(jtis, jti)
		}
	}
	return jtis, nil
}

func (m *MockRepository) GetSessionTTL(ctx context.Context, jti string) time.Duration {
	return 1 * time.Hour
}

func (m *MockRepository) RevokeSessions(ctx context.Context, userID string) error {
	for jti, s := range m.Sessions {
		if s.UserID == userID {
			delete(m.Sessions, jti)
		}
	}
	return nil
}

func (m *MockRepository) UpdateUserStatus(ctx context.Context, userID, status string) error {
	if u, ok := m.Users[userID]; ok {
		u.Status = status
		return nil
	}
	return fmt.Errorf("not found")
}

func setup(_ *testing.T) (*service.Service, *MockRepository, *miniredis.Miniredis) {
	mr, _ := miniredis.Run()
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	repo := &MockRepository{
		Users:           make(map[string]*iam_repo.User),
		FailedLogins:    make(map[string]int),
		LockedUntil:     make(map[string]time.Time),
		Sessions:        make(map[string]*MockSession),
		RefreshTokens:   make(map[string]*iam_repo.RefreshToken),
		RevokedFamilies: make(map[uuid.UUID]bool),
		MFAConfigs:      make(map[string][]iam_repo.MFAConfig),
	}
	aesKey := "01234567890123456789012345678901"
	aesKeyring := []crypto.EncryptionKey{{Kid: "a1", Key: aesKey, Status: "active"}}
	pool := service.NewAuthWorkerPool(1, context.Background())
	keyring := []crypto.JWTKey{{Kid: "k1", Secret: "test-secret-at-least-32-bytes!!", Algorithm: "HS256", Status: "active"}}
	s := service.NewService(repo, pool, keyring, aesKeyring, rdb)
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
	if token == nil || token.AccessToken == "" || user.ID != "1" {
		t.Errorf("login failed: user=%v, token=%v", user, token)
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
	if token == nil || token.AccessToken == "" || user.ID != "1" {
		t.Errorf("expected MFA required, got user=%v, token=%v", user, token)
	}

	challengeToken := token.AccessToken
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

func TestRefreshToken_FullRotation(t *testing.T) {
	s, repo, _ := setup(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 10)
	repo.Users["1"] = &iam_repo.User{
		ID: "1", OrgID: "org1", Email: "test@example.com", PasswordHash: string(hash), Status: "active",
	}

	// 1. Initial Login
	_, _, err := s.Login(context.Background(), "test@example.com", "password", "ua", "127.0.0.1")
	require.NoError(t, err)

	// Let's use IssueTokens directly for the rotation test.
	res1, err := s.IssueTokens(context.Background(), service.IssueTokensRequest{
		OrgID: "org1", UserID: "1", UserAgent: "ua", IPAddress: "127.0.0.1", FamilyID: uuid.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rt1 := res1.RefreshToken
	familyID := repo.RefreshTokens[crypto.HashSHA256(rt1)].FamilyID

	// 2. First Rotation (Happy Path)
	res2, err := s.RefreshToken(context.Background(), rt1, "ua", "127.0.0.1")
	if err != nil {
		t.Fatalf("first rotation failed: %v", err)
	}
	rt2 := res2.RefreshToken

	// Verify RT1 is revoked in mock
	if rt, ok := repo.RefreshTokens[crypto.HashSHA256(rt1)]; !ok || !rt.Revoked {
		t.Error("RT1 should be revoked in DB after successful rotation")
	}

	// 3. Second Rotation (Happy Path)
	res3, err := s.RefreshToken(context.Background(), rt2, "ua", "127.0.0.1")
	if err != nil {
		t.Fatalf("second rotation failed: %v", err)
	}
	rt3 := res3.RefreshToken
	_ = rt3

	// 4. Reuse RT1 (Compromise Detection)
	_, err = s.RefreshToken(context.Background(), rt1, "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "SESSION_COMPROMISED") {
		t.Errorf("expected SESSION_COMPROMISED error on RT1 reuse, got %v", err)
	}

	// Verify Family is revoked
	if !repo.RevokedFamilies[familyID] {
		t.Error("family should be revoked after RT reuse")
	}

	// 5. Attempt to use RT3 (should fail because family is revoked)
	_, err = s.RefreshToken(context.Background(), rt3, "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "SESSION_COMPROMISED") {
		t.Errorf("expected SESSION_COMPROMISED error on RT3 after family revocation, got %v", err)
	}
}

func TestRefreshToken_RiskBasedRevocation(t *testing.T) {
	s, repo, _ := setup(t)
	familyID := uuid.New()
	token := "risk-token"
	rtHash := crypto.HashSHA256(token)

	repo.RefreshTokens[rtHash] = &iam_repo.RefreshToken{
		OrgID: "org1", UserID: "user1", FamilyID: familyID, ExpiresAt: time.Now().Add(1 * time.Hour), Revoked: false,
	}
	// Initial session with UA/IP
	repo.Sessions["jti-1"] = &MockSession{
		Session: iam_repo.Session{
			JTI:       "jti-1",
			UserAgent: "Mozilla/5.0",
			IPAddress: "1.1.1.1",
		},
		UserID: "user1",
	}

	// 1. Refresh with significant UA and IP change (threshold = 80)
	// UA Family change = 60, IP Subnet change = 40. Total = 100.
	_, err := s.RefreshToken(context.Background(), token, "Chrome/100", "2.2.2.2")
	
	if err == nil || !strings.Contains(err.Error(), "SESSION_REVOKED_RISK") {
		t.Errorf("expected SESSION_REVOKED_RISK error, got %v", err)
	}

	// Verify family is revoked
	if !repo.RevokedFamilies[familyID] {
		t.Error("family should be revoked due to high risk score")
	}
}

func TestRefreshToken_Success(t *testing.T) {
	s, repo, _ := setup(t)
	familyID := uuid.New()
	token := crypto.GenerateRandomString(64)
	rtHash := crypto.HashSHA256(token)

	repo.RefreshTokens[rtHash] = &iam_repo.RefreshToken{
		OrgID: "org1", UserID: "user1", FamilyID: familyID, ExpiresAt: time.Now().Add(1 * time.Hour), Revoked: false,
	}
	repo.Sessions["jti-1"] = &MockSession{
		Session: iam_repo.Session{
			UserAgent: "ua", IPAddress: "127.0.0.1",
		},
		UserID: "user1",
	}

	res, err := s.RefreshToken(context.Background(), token, "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Error("expected new tokens")
	}
	if rt, ok := repo.RefreshTokens[rtHash]; !ok || !rt.Revoked {
		t.Error("old token should be revoked in mock")
	}
}

func TestRefreshToken_Expired(t *testing.T) {
	s, repo, _ := setup(t)
	familyID := uuid.New()
	token := "expired-token"
	rtHash := crypto.HashSHA256(token)

	repo.RefreshTokens[rtHash] = &iam_repo.RefreshToken{
		OrgID: "org1", UserID: "user1", FamilyID: familyID, ExpiresAt: time.Now().Add(-1 * time.Hour), Revoked: false,
	}

	_, err := s.RefreshToken(context.Background(), token, "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "SESSION_COMPROMISED") {
		t.Errorf("expected SESSION_COMPROMISED (or similar) error on expired token, got %v", err)
	}
}

func TestLogout_BlocklistsJTI(t *testing.T) {
	s, _, mr := setup(t)
	jti := "test-jti"
	expiresAt := time.Now().Add(1 * time.Minute)

	err := s.Logout(context.Background(), jti, expiresAt)
	if err != nil {
		t.Fatal(err)
	}

	if !mr.Exists("blocklist:" + jti) {
		t.Error("jti should be in blocklist")
	}
}

func TestTOTP_Verify_Success(t *testing.T) {
	s, repo, _ := setup(t)
	repo.Users["1"] = &iam_repo.User{ID: "1", OrgID: "org1", Email: "test@example.com", Status: "active"}

	// 1. Setup TOTP
	secret, _, err := s.GenerateTOTPSetup(context.Background(), "1", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	// 2. Enable TOTP (requires a valid code)
	code, _ := totp.GenerateCode(secret, time.Now())
	_, err = s.EnableTOTP(context.Background(), "org1", "1", code, secret)
	if err != nil {
		t.Fatalf("failed to enable totp: %v", err)
	}

	// 3. Verify MFA enabled
	if !repo.Users["1"].MFAEnabled {
		t.Error("MFA should be enabled")
	}

	// 4. Verify TOTP (Login Flow)
	ok, err := s.VerifyTOTP(context.Background(), "1", code)
	if err != nil || !ok {
		t.Errorf("failed to verify totp: ok=%v, err=%v", ok, err)
	}

	// 5. Replay Protection (Same code should fail)
	_, err = s.VerifyTOTP(context.Background(), "1", code)
	if err == nil || !strings.Contains(err.Error(), "already used") {
		t.Errorf("expected replay protection error, got %v", err)
	}
}

func TestTOTP_Verify_InvalidCode(t *testing.T) {
	s, repo, _ := setup(t)
	repo.Users["1"] = &iam_repo.User{ID: "1", OrgID: "org1", Email: "test@example.com", Status: "active"}

	// 1. Setup TOTP
	secret, _, _ := s.GenerateTOTPSetup(context.Background(), "1", "test@example.com")
	
	// 2. Enable TOTP with wrong code
	_, err := s.EnableTOTP(context.Background(), "org1", "1", "000000", secret)
	if err == nil {
		t.Error("expected error with invalid totp code")
	}

	// 3. Verify MFA still disabled
	if repo.Users["1"].MFAEnabled {
		t.Error("MFA should not be enabled")
	}
}

func TestVerifyTOTP_InvalidCode(t *testing.T) {
	s, repo, _ := setup(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("password"), 10)
	repo.Users["1"] = &iam_repo.User{ID: "1", OrgID: "org1", Email: "test@example.com", PasswordHash: string(hash), Status: "active"}

	// 1. Setup and Enable TOTP
	secret, _, _ := s.GenerateTOTPSetup(context.Background(), "1", "test@example.com")
	code, _ := totp.GenerateCode(secret, time.Now())
	_, _ = s.EnableTOTP(context.Background(), "org1", "1", code, secret)

	// 2. Initial Login to get challenge
	_, token, _ := s.Login(context.Background(), "test@example.com", "password", "ua", "127.0.0.1")
	challengeToken := token.AccessToken

	// 3. Verify with wrong code
	_, _, err := s.VerifyMFAAndLogin(context.Background(), challengeToken, "000000", "ua", "127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "invalid mfa code") {
		t.Errorf("expected invalid mfa code error, got %v", err)
	}
}

func TestSCIM_Deprovisioning_RevokesSessions(t *testing.T) {
	s, repo, mr := setup(t)
	repo.Users["user1"] = &iam_repo.User{ID: "user1", OrgID: "org1", Email: "user1@test.io", Status: "active"}
	
	// Create active sessions
	repo.Sessions["jti-1"] = &MockSession{UserID: "user1", Session: iam_repo.Session{JTI: "jti-1"}}
	repo.Sessions["jti-2"] = &MockSession{UserID: "user1", Session: iam_repo.Session{JTI: "jti-2"}}

	// Deprovision user
	err := s.DeleteUser(context.Background(), "user1")
	require.NoError(t, err)

	// 1. Verify user status
	assert.Equal(t, "deprovisioned", repo.Users["user1"].Status)

	// 2. Verify sessions revoked in DB
	assert.Empty(t, repo.Sessions)

	// 3. Verify JTIs blocklisted in Redis
	assert.True(t, mr.Exists("blocklist:jti-1"))
	assert.True(t, mr.Exists("blocklist:jti-2"))
}

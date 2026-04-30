package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
)

type WebAuthnUser struct {
	id          []byte
	displayName string
	name        string
	credentials []webauthn.Credential
}

func (u *WebAuthnUser) WebAuthnID() []byte                         { return u.id }
func (u *WebAuthnUser) WebAuthnName() string                       { return u.name }
func (u *WebAuthnUser) WebAuthnDisplayName() string                { return u.displayName }
func (u *WebAuthnUser) WebAuthnIcon() string                       { return "" }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func (s *Service) BeginWebAuthnRegistration(ctx context.Context, userID string) (string, *webauthn.SessionData, *protocol.CredentialCreation, error) {
	if s.webauthn == nil {
		return "", nil, nil, fmt.Errorf("webauthn not configured")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return "", nil, nil, err
	}

	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user.DisplayName,
		name:        user.Email,
	}

	options, session, err := s.webauthn.BeginRegistration(wUser)
	if err != nil {
		return "", nil, nil, err
	}

	sessionID := uuid.New().String()
	sessionKey := fmt.Sprintf("webauthn:reg:%s:%s", userID, sessionID)
	sessionJSON, _ := json.Marshal(session)
	if err := s.rdb.Set(ctx, sessionKey, sessionJSON, 5*time.Minute).Err(); err != nil {
		return "", nil, nil, err
	}

	return sessionID, session, options, nil
}

func (s *Service) FinishWebAuthnRegistration(ctx context.Context, orgID, userID, sessionID string, response *http.Request) error {
	if s.webauthn == nil {
		return fmt.Errorf("webauthn not configured")
	}

	sessionKey := fmt.Sprintf("webauthn:reg:%s:%s", userID, sessionID)
	val, err := s.rdb.GetDel(ctx, sessionKey).Result()
	if err != nil {
		return fmt.Errorf("registration session expired or invalid")
	}
	var session webauthn.SessionData
	_ = json.Unmarshal([]byte(val), &session)

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user.DisplayName,
		name:        user.Email,
	}

	credential, err := s.webauthn.FinishRegistration(wUser, session, response)
	if err != nil {
		return err
	}

	cred := iam_repo.WebAuthnCredential{
		CredentialID:    hex.EncodeToString(credential.ID),
		PublicKey:       hex.EncodeToString(credential.PublicKey),
		AttestationType: credential.AttestationType,
		SignCount:       int32(credential.Authenticator.SignCount),
	}

	if err := s.repo.SaveWebAuthnCredential(ctx, orgID, userID, cred); err != nil {
		return err
	}

	s.rdb.Del(ctx, "webauthn:reg:"+userID)
	return nil
}

func (s *Service) BeginWebAuthnLogin(ctx context.Context, email string) (string, *webauthn.SessionData, *protocol.CredentialAssertion, error) {
	if s.webauthn == nil {
		return "", nil, nil, fmt.Errorf("webauthn not configured")
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return "", nil, nil, err
	}

	userID := user.ID
	credentials, _ := s.repo.ListWebAuthnCredentials(ctx, userID)

	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user.DisplayName,
		name:        user.Email,
	}

	for _, c := range credentials {
		id, _ := hex.DecodeString(c.CredentialID)
		pubKey, _ := hex.DecodeString(c.PublicKey)
		wUser.credentials = append(wUser.credentials, webauthn.Credential{
			ID:        id,
			PublicKey: pubKey,
			Authenticator: webauthn.Authenticator{
				SignCount: uint32(c.SignCount),
			},
		})
	}

	options, session, err := s.webauthn.BeginLogin(wUser)
	if err != nil {
		return "", nil, nil, err
	}

	sessionID := uuid.New().String()
	sessionKey := fmt.Sprintf("webauthn:login:%s:%s", userID, sessionID)
	sessionJSON, _ := json.Marshal(session)
	if err := s.rdb.Set(ctx, sessionKey, sessionJSON, 5*time.Minute).Err(); err != nil {
		return "", nil, nil, err
	}

	return sessionID, session, options, nil
}

func (s *Service) FinishWebAuthnLogin(ctx context.Context, email, sessionID string, userAgent, ip string, response *http.Request) (*iam_repo.User, string, error) {
	if s.webauthn == nil {
		return nil, "", fmt.Errorf("webauthn not configured")
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, "", err
	}

	userID := user.ID
	sessionKey := fmt.Sprintf("webauthn:login:%s:%s", userID, sessionID)
	val, err := s.rdb.GetDel(ctx, sessionKey).Result()
	if err != nil {
		return nil, "", fmt.Errorf("login session expired or invalid")
	}
	var session webauthn.SessionData
	_ = json.Unmarshal([]byte(val), &session)

	credentials, _ := s.repo.ListWebAuthnCredentials(ctx, userID)
	wUser := &WebAuthnUser{
		id:          []byte(userID),
		displayName: user.DisplayName,
		name:        user.Email,
	}
	for _, c := range credentials {
		id, _ := hex.DecodeString(c.CredentialID)
		pubKey, _ := hex.DecodeString(c.PublicKey)
		wUser.credentials = append(wUser.credentials, webauthn.Credential{
			ID:        id,
			PublicKey: pubKey,
			Authenticator: webauthn.Authenticator{
				SignCount: uint32(c.SignCount),
			},
		})
	}

	_, err = s.webauthn.FinishLogin(wUser, session, response)
	if err != nil {
		return nil, "", err
	}

	s.rdb.Del(ctx, "webauthn:login:"+userID)

	res, err := s.IssueTokens(ctx, IssueTokensRequest{
		OrgID:     user.OrgID,
		UserID:    userID,
		UserAgent: userAgent,
		IPAddress: ip,
		FamilyID:  uuid.New(),
	})
	if err != nil {
		return nil, "", err
	}

	return user, res.AccessToken, nil
}

func (s *Service) GenerateTOTPSetup(ctx context.Context, userID, email string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "OpenGuard",
		AccountName: email,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func (s *Service) EnableTOTP(ctx context.Context, orgID, userID, code, secret string) ([]string, error) {
	if !totp.Validate(code, secret) {
		return nil, fmt.Errorf("invalid totp code")
	}

	encrypted, err := crypto.Encrypt([]byte(secret), s.aesKeyring)
	if err != nil {
		return nil, fmt.Errorf("encrypt secret: %w", err)
	}

	if err := s.repo.UpsertMFAConfig(ctx, orgID, userID, "totp", encrypted); err != nil {
		return nil, err
	}

	if err := s.repo.EnableUserMFA(ctx, userID, true, "totp"); err != nil {
		return nil, err
	}

	return s.GenerateBackupCodes(ctx, orgID, userID)
}

func (s *Service) GenerateBackupCodes(ctx context.Context, orgID, userID string) ([]string, error) {
	secret := os.Getenv("IAM_MFA_BACKUP_CODE_HMAC_SECRET")
	if secret == "" {
		secret = "dev-backup-code-secret-fixed-value-for-dev"
	}
	codes := make([]string, 8)
	hashes := make([]string, 8)
	for i := range codes {
		raw := crypto.GenerateRandomString(8)
		codes[i] = raw
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(raw))
		hashes[i] = hex.EncodeToString(mac.Sum(nil))
	}
	if err := s.repo.StoreBackupCodes(ctx, userID, hashes); err != nil {
		return nil, err
	}
	return codes, nil
}

func (s *Service) VerifyBackupCode(ctx context.Context, userID, code string) (bool, error) {
	secret := os.Getenv("IAM_MFA_BACKUP_CODE_HMAC_SECRET")
	if secret == "" {
		secret = "dev-backup-code-secret-fixed-value-for-dev"
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(code))
	codeHash := hex.EncodeToString(mac.Sum(nil))
	return s.repo.ConsumeBackupCode(ctx, userID, codeHash)
}

func (s *Service) VerifyTOTP(ctx context.Context, userID, code string) (bool, error) {
	nonceKey := fmt.Sprintf("totp:used:%s:%s", userID, code)
	res, err := s.rdb.SetArgs(ctx, nonceKey, "1", redis.SetArgs{Mode: "NX", TTL: 90 * time.Second}).Result()
	if err != nil && err != redis.Nil {
		return false, err
	}
	if res != "OK" {
		return false, fmt.Errorf("totp code already used")
	}

	config, err := s.repo.GetMFAConfig(ctx, userID, "totp")
	if err != nil {
		return false, err
	}

	secretBytes, err := crypto.Decrypt(config.SecretEncrypted, s.aesKeyring)
	if err != nil {
		return false, fmt.Errorf("decrypt secret: %w", err)
	}

	return totp.Validate(code, string(secretBytes)), nil
}

func (s *Service) VerifyMFAAndLogin(ctx context.Context, challengeToken, code, userAgent, ip string) (*iam_repo.User, string, error) {
	res, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		return s.rdb.GetDel(ctx, "mfa_challenge:"+challengeToken).Result()
	})
	if err != nil {
		return nil, "", fmt.Errorf("invalid or expired challenge")
	}
	userID := res.(string)

	ok, err := s.VerifyTOTP(ctx, userID, code)
	if err != nil || !ok {
		return nil, "", fmt.Errorf("invalid mfa code")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, "", err
	}

	resToken, err := s.IssueTokens(ctx, IssueTokensRequest{
		OrgID:     user.OrgID,
		UserID:    user.ID,
		UserAgent: userAgent,
		IPAddress: ip,
		FamilyID:  uuid.New(),
	})
	if err != nil {
		return nil, "", err
	}

	return user, resToken.AccessToken, nil
}

func (s *Service) VerifyBackupCodeAndLogin(ctx context.Context, challengeToken, code, userAgent, ip string) (*iam_repo.User, string, error) {
	res, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		return s.rdb.GetDel(ctx, "mfa_challenge:"+challengeToken).Result()
	})
	if err != nil {
		return nil, "", fmt.Errorf("invalid or expired challenge")
	}
	userID := res.(string)

	ok, err := s.VerifyBackupCode(ctx, userID, code)
	if err != nil || !ok {
		return nil, "", fmt.Errorf("invalid backup code")
	}

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, "", err
	}

	resToken, err := s.IssueTokens(ctx, IssueTokensRequest{
		OrgID:     user.OrgID,
		UserID:    user.ID,
		UserAgent: userAgent,
		IPAddress: ip,
		FamilyID:  uuid.New(),
	})
	if err != nil {
		return nil, "", err
	}

	return user, resToken.AccessToken, nil
}

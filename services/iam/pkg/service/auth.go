package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
)

func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (*iam_repo.User, string, error) {
	user, userErr := s.userRepo.GetUserByEmail(ctx, email)

	// Rationale: constant-time comparison to prevent account enumeration.
	// Use a pre-generated dummy hash for cost 12 to equalize timing.
	hashToCompare := "$2a$12$R9h/cIPz0gi.URQHeNH5OuLzBeGPWbS6vS6vS6vS6vS6vS6vS6vS6"
	if userErr == nil {
		hashToCompare = user.PasswordHash
	}

	// Always run bcrypt comparison to equalize timing (~350ms)
	bcryptErr := s.pool.Compare(ctx, password, hashToCompare)

	if userErr != nil {
		return nil, "", ErrInvalidCredentials
	}

	// Status and lockout checks happen AFTER bcrypt to prevent state enumeration
	if user.Status == "initializing" {
		return nil, "", ErrAccountSetup
	}

	if user.LockedUntil != nil {
		if time.Now().Before(*user.LockedUntil) {
			return nil, "", ErrInvalidCredentials
		}
	}

	if bcryptErr != nil {
		count, _ := s.userRepo.IncrementFailedLogin(ctx, email)
		if count >= 10 {
			until := time.Now().Add(lockoutDuration(count))
			_ = s.userRepo.LockAccount(ctx, email, until)
		}
		return nil, "", ErrInvalidCredentials
	}

	_ = s.userRepo.ResetFailedLogin(ctx, email)

	mfaConfigs, _ := s.mfaRepo.ListMFAConfigs(ctx, user.ID)
	if len(mfaConfigs) > 0 {
		challengeToken := uuid.New().String()
		_, _ = resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
			return nil, s.rdb.Set(ctx, "mfa_challenge:"+challengeToken, user.ID, 5*time.Minute).Err()
		})
		// We return a partially populated user for MFA redirection
		return &iam_repo.User{
			ID:    user.ID,
			OrgID: user.OrgID,
		}, challengeToken, nil
	}

	res, err := s.IssueTokens(ctx, IssueTokensRequest{
		OrgID:     user.OrgID,
		UserID:    user.ID,
		UserAgent: userAgent,
		IPAddress: ip,
		FamilyID:  uuid.New(),
	})
	if err != nil {
		return nil, "", err
	}

	return user, res.AccessToken, nil
}

func (s *Service) IssueTokens(ctx context.Context, req IssueTokensRequest) (*TokenResponse, error) {
	ip := req.IPAddress
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}

	jti := uuid.New().String()
	ttl := 1 * time.Hour
	accessToken, err := s.SignToken(req.OrgID, req.UserID, jti, ttl)
	if err != nil {
		return nil, err
	}

	err = s.sessionRepo.CreateSession(ctx, req.OrgID, req.UserID, jti, req.UserAgent, ip, time.Now().Add(ttl))
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	refreshToken := crypto.GenerateRandomString(64)
	rtHash := crypto.HashSHA256(refreshToken)
	rtTTL := 7 * 24 * time.Hour

	err = s.tokenRepo.CreateRefreshToken(ctx, req.OrgID, req.UserID, rtHash, req.FamilyID, time.Now().Add(rtTTL))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token: %w", err)
	}

	return &TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(ttl.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken, userAgent, ip string) (*TokenResponse, error) {
	rtHash := crypto.HashSHA256(refreshToken)
	rt, err := s.tokenRepo.ClaimRefreshToken(ctx, rtHash)
	if err != nil {
		_ = s.tokenRepo.RevokeRefreshTokenFamilyByHash(ctx, rtHash)
		return nil, ErrSessionCompromised
	}

	session, err := s.sessionRepo.GetSessionByUserID(ctx, rt.UserID)
	if err == nil && session != nil {
		score := calculateRiskScore(session.UserAgent, userAgent, session.IPAddress, ip)
		if score >= riskThresholdRevoke {
			_ = s.tokenRepo.RevokeRefreshTokenFamily(ctx, rt.FamilyID)
			return nil, ErrSessionRevokedRisk
		}
	}

	return s.IssueTokens(ctx, IssueTokensRequest{
		OrgID:     rt.OrgID,
		UserID:    rt.UserID,
		UserAgent: userAgent,
		IPAddress: ip,
		FamilyID:  rt.FamilyID,
	})
}

func (s *Service) SignToken(orgID, userID, jti string, ttl time.Duration) (string, error) {
	claims := crypto.NewStandardClaims(orgID, userID, jti, ttl)
	return crypto.Sign(claims, s.keyring)
}

func (s *Service) Logout(ctx context.Context, jti string, expiresAt time.Time) error {
	if s.rdb == nil {
		return nil
	}
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil
	}
	_, err := resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
		return nil, s.rdb.Set(ctx, "blocklist:"+jti, "revoked", ttl).Err()
	})
	return err
}

func (s *Service) StoreAuthCode(ctx context.Context, code, orgID, userID, codeChallenge string) error {
	if s.rdb == nil {
		return fmt.Errorf("redis not configured")
	}
	data, _ := json.Marshal(map[string]string{
		"org_id":         orgID,
		"user_id":        userID,
		"code_challenge": codeChallenge,
	})
	return s.rdb.Set(ctx, "auth_code:"+code, data, 10*time.Minute).Err()
}

func (s *Service) GetAuthCode(ctx context.Context, code string) (string, string, string, error) {
	if s.rdb == nil {
		return "", "", "", fmt.Errorf("redis not configured")
	}
	val, err := s.rdb.GetDel(ctx, "auth_code:"+code).Result()
	if err != nil {
		return "", "", "", err
	}

	var data map[string]string
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		parts := strings.Split(val, ":")
		if len(parts) >= 2 {
			return parts[0], parts[1], "", nil
		}
		return "", "", "", fmt.Errorf("invalid auth code data")
	}

	return data["org_id"], data["user_id"], data["code_challenge"], nil
}

func calculateRiskScore(storedUA, currentUA, storedIP, currentIP string) int {
	score := 0
	storedFamily, storedVersion := parseUserAgent(storedUA)
	currentFamily, currentVersion := parseUserAgent(currentUA)

	if storedFamily != currentFamily {
		score += riskScoreUAFamilyChange
	} else if storedVersion != currentVersion {
		score += riskScoreUAVersionChange
	}

	storedNet := ipToSubnet16(storedIP)
	currentNet := ipToSubnet16(currentIP)
	if storedNet != currentNet {
		score += riskScoreIPSubnetChange
	} else if storedIP != currentIP {
		score += riskScoreIPHostChange
	}

	return score
}

func parseUserAgent(ua string) (family, version string) {
	parts := strings.Split(ua, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return ua, "0.0"
}

func ipToSubnet16(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ipStr
	}
	ip = ip.To4()
	if ip == nil {
		return ipStr
	}
	return fmt.Sprintf("%d.%d", ip[0], ip[1])
}

func lockoutDuration(failCount int) time.Duration {
	// failCount=10 -> 15m, 20 -> 30m, 30 -> 60m, ... cap at 24h
	steps := (failCount / 10) - 1
	if steps < 0 {
		steps = 0
	}
	dur := 15 * time.Minute * (1 << uint(steps))
	if dur > 24*time.Hour {
		dur = 24 * time.Hour
	}
	return dur
}

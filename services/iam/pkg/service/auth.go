package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openguard/shared/crypto"
	"github.com/openguard/shared/resilience"
)

func (s *Service) Login(ctx context.Context, email, password, userAgent, ip string) (map[string]interface{}, string, error) {
	user, userErr := s.repo.GetUserByEmail(ctx, email)
	
	// Rationale: constant-time comparison to prevent account enumeration.
	// Use a pre-generated dummy hash for cost 12 to equalize timing.
	hashToCompare := "$2a$12$R9h/cIPz0gi.URQHeNH5OuLzBeGPWbS6vS6vS6vS6vS6vS6vS6vS6" 
	if userErr == nil {
		hashToCompare = user["password_hash"].(string)
	}

	// Always run bcrypt comparison to equalize timing (~350ms)
	bcryptErr := s.pool.Compare(ctx, password, hashToCompare)

	if userErr != nil {
		return nil, "", fmt.Errorf("INVALID_CREDENTIALS")
	}

	// Status and lockout checks happen AFTER bcrypt to prevent state enumeration
	if user["status"].(string) == "initializing" {
		return nil, "", fmt.Errorf("ACCOUNT_SETUP_PENDING")
	}

	if user["locked_until"] != nil {
		until := user["locked_until"].(*time.Time)
		if until != nil && time.Now().Before(*until) {
			return nil, "", fmt.Errorf("INVALID_CREDENTIALS")
		}
	}

	if bcryptErr != nil {
		count, _ := s.repo.IncrementFailedLogin(ctx, email)
		if count >= 10 {
			until := time.Now().Add(lockoutDuration(count))
			_ = s.repo.LockAccount(ctx, email, until)
		}
		return nil, "", fmt.Errorf("INVALID_CREDENTIALS")
	}

	_ = s.repo.ResetFailedLogin(ctx, email)

	mfaConfigs, _ := s.repo.ListMFAConfigs(ctx, user["id"].(string))
	if len(mfaConfigs) > 0 {
		challengeToken := uuid.New().String()
		_, _ = resilience.Call(ctx, s.redisBreaker, 100*time.Millisecond, func(ctx context.Context) (interface{}, error) {
			return nil, s.rdb.Set(ctx, "mfa_challenge:"+challengeToken, user["id"].(string), 5*time.Minute).Err()
		})
		return map[string]interface{}{
			"mfa_required":  true,
			"mfa_challenge": challengeToken,
			"user_id":       user["id"].(string),
		}, "", nil
	}

	delete(user, "password_hash")
	delete(user, "failed_login_count")
	delete(user, "locked_until")

	res, err := s.IssueTokens(ctx, user["org_id"].(string), user["id"].(string), userAgent, ip, uuid.New())
	if err != nil {
		return nil, "", err
	}

	return user, res["access_token"].(string), nil
}

func (s *Service) IssueTokens(ctx context.Context, orgID, userID, userAgent, ip string, familyID uuid.UUID) (map[string]interface{}, error) {
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}

	jti := uuid.New().String()
	ttl := 1 * time.Hour
	accessToken, err := s.SignToken(orgID, userID, jti, ttl)
	if err != nil {
		return nil, err
	}

	err = s.repo.CreateSession(ctx, orgID, userID, jti, userAgent, ip, time.Now().Add(ttl))
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	refreshToken := crypto.GenerateRandomString(64)
	rtHash := crypto.HashSHA256(refreshToken)
	rtTTL := 7 * 24 * time.Hour

	err = s.repo.CreateRefreshToken(ctx, orgID, userID, rtHash, familyID, time.Now().Add(rtTTL))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh token: %w", err)
	}

	return map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_in":    int(ttl.Seconds()),
	}, nil
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken, userAgent, ip string) (map[string]interface{}, error) {
	rtHash := crypto.HashSHA256(refreshToken)
	rt, err := s.repo.ClaimRefreshToken(ctx, rtHash)
	if err != nil {
		s.repo.RevokeRefreshTokenFamilyByHash(ctx, rtHash)
		return nil, fmt.Errorf("SESSION_COMPROMISED")
	}

	session, err := s.repo.GetSessionByUserID(ctx, rt["user_id"].(string))
	if err == nil && session != nil {
		storedUA, _ := session["user_agent"].(string)
		storedIP, _ := session["ip_address"].(string)
		score := calculateRiskScore(storedUA, userAgent, storedIP, ip)
		if score >= riskThresholdRevoke {
			s.repo.RevokeRefreshTokenFamily(ctx, rt["family_id"].(uuid.UUID))
			return nil, fmt.Errorf("SESSION_REVOKED_RISK")
		}
	}

	return s.IssueTokens(ctx, rt["org_id"].(string), rt["user_id"].(string), userAgent, ip, rt["family_id"].(uuid.UUID))
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

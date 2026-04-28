package crypto

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrTokenExpired = errors.New("token expired")
	ErrTokenInvalid = errors.New("token invalid")
)

// JWTKey represents a key in the JWT multi-key keyring.
type JWTKey struct {
	Kid       string `json:"kid"`
	Secret    string `json:"secret"`    // base64-encoded secret for HS256
	Algorithm string `json:"algorithm"` // "HS256" | "RS256"
	Status    string `json:"status"`    // "active" | "verify_only"
}

// @AI-INTENT: [Pattern: Key Rotation via Multi-Key Keyring]
// [Rationale: Security agility. Supporting multiple keys allows for seamless secret rotation.
// The 'verify_only' status enables the system to accept old tokens while issuing new ones with 'active' keys.]

// Sign generates a signed JWT using the first active key in the keyring.
func Sign(claims jwt.Claims, keyring []JWTKey) (string, error) {
	var activeKey *JWTKey
	for _, k := range keyring {
		if k.Status == "active" {
			activeKey = &k
			break
		}
	}

	if activeKey == nil {
		return "", ErrKeyNotFound
	}

	token := jwt.NewWithClaims(jwt.GetSigningMethod(activeKey.Algorithm), claims)
	token.Header["kid"] = activeKey.Kid

	return token.SignedString([]byte(activeKey.Secret))
}

// Verify parses and verifies a JWT using the matching key from the keyring.
func Verify(tokenString string, keyring []JWTKey, claims jwt.Claims) (*jwt.Token, error) {
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in header")
		}

		for _, k := range keyring {
			if k.Kid == kid {
				if k.Algorithm != token.Method.Alg() {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
				}
				return []byte(k.Secret), nil
			}
		}

		return nil, ErrKeyNotFound
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrTokenInvalid, err)
	}

	if !token.Valid {
		return nil, ErrTokenInvalid
	}

	return token, nil
}

// StandardClaims includes core OpenGuard claims.
type StandardClaims struct {
	jwt.RegisteredClaims
	OrgID  string `json:"org_id"`
	UserID string `json:"user_id"`
}

// NewStandardClaims creates a new StandardClaims with JTI and standard expiration.
func NewStandardClaims(orgID, userID, jti string, ttl time.Duration) StandardClaims {
	now := time.Now()
	return StandardClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    "openguard-iam",
			Subject:   userID,
			Audience:  jwt.ClaimStrings{"openguard-ui", "openguard-sdk"},
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now.Add(-1 * time.Minute)), // 1m clock skew
		},
		OrgID:  orgID,
		UserID: userID,
	}
}
// LoadKeyring parses a JSON-encoded keyring.
func LoadKeyring(jsonStr string) ([]JWTKey, error) {
	var keyring []JWTKey
	if err := json.Unmarshal([]byte(jsonStr), &keyring); err != nil {
		return nil, fmt.Errorf("invalid keyring JSON: %w", err)
	}
	return keyring, nil
}

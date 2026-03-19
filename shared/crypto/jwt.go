package crypto

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type JWTKey struct {
	Kid       string `json:"kid"`
	Secret    string `json:"secret"`
	Algorithm string `json:"algorithm"`
	Status    string `json:"status"` // "active" | "verify_only"
}

type JWTKeyring struct {
	keys []JWTKey
}

func NewJWTKeyring(keys []JWTKey) *JWTKeyring {
	return &JWTKeyring{keys: keys}
}

// Sign uses the first key with status="active".
func (k *JWTKeyring) Sign(claims jwt.Claims) (string, error) {
	for _, key := range k.keys {
		if key.Status == "active" {
			token := jwt.NewWithClaims(jwt.GetSigningMethod(key.Algorithm), claims)
			token.Header["kid"] = key.Kid
			return token.SignedString([]byte(key.Secret))
		}
	}
	return "", errors.New("no active jwt key found")
}

// Verify tries all keys, matching on kid from the token header.
// Returns an error if the token is expired or invalid.
func (k *JWTKeyring) Verify(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, errors.New("missing kid in token header")
		}
		for _, key := range k.keys {
			if key.Kid == kid {
				if token.Method.Alg() != key.Algorithm {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
				}
				return []byte(key.Secret), nil
			}
		}
		return nil, errors.New("unknown kid")
	})

	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid jwt token")
}

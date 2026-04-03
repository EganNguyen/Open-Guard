package crypto

import (
	"errors"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type JWTKey struct {
	Kid       string `json:"kid"`
	Secret    string `json:"secret"` // Symmetric secret for HS256 or PEM-encoded RSA Private Key for RS256
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

			if strings.HasPrefix(key.Algorithm, "RS") {
				rsaKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(key.Secret))
				if err != nil {
					return "", fmt.Errorf("invalid rsa private key for kid %s: %w", key.Kid, err)
				}
				return token.SignedString(rsaKey)
			}
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

				if strings.HasPrefix(key.Algorithm, "RS") {
					// Check if Secret is private key (common in our config) or public key
					rsaKey, err := jwt.ParseRSAPublicKeyFromPEM([]byte(key.Secret))
					if err != nil {
						// Not a public key, try parsing as private key and extracting public key
						privKey, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(key.Secret))
						if err != nil {
							return nil, fmt.Errorf("invalid rsa key for kid %s: %w", key.Kid, err)
						}
						return &privKey.PublicKey, nil
					}
					return rsaKey, nil
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

// GetKeys returns all keys in the keyring.
func (k *JWTKeyring) GetKeys() []JWTKey {
	return k.keys
}

package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type AuthClient struct {
	provider     *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	oauth2Config oauth2.Config
	jtiBlocklist JTIBlocklist
	httpClient   *http.Client
}

type JTIBlocklist interface {
	IsBlocked(ctx context.Context, jti string) (bool, error)
}

func NewAuthClient(ctx context.Context, issuer, clientID, clientSecret, redirectURL string, jtiBlocklist JTIBlocklist, httpClient *http.Client) (*AuthClient, error) {
	if httpClient != nil {
		ctx = oidc.ClientContext(ctx, httpClient)
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}

	oidcConfig := &oidc.Config{
		ClientID:        clientID,
		SkipIssuerCheck: true,
	}
	verifier := provider.Verifier(oidcConfig)

	config := oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "org"},
	}

	return &AuthClient{
		provider:     provider,
		verifier:     verifier,
		oauth2Config: config,
		jtiBlocklist: jtiBlocklist,
		httpClient:   httpClient,
	}, nil
}

func (c *AuthClient) AuthURL(state string) string {
	return c.oauth2Config.AuthCodeURL(state)
}

func (c *AuthClient) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	if c.httpClient != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, c.httpClient)
	}
	return c.oauth2Config.Exchange(ctx, code)
}

func (c *AuthClient) VerifyToken(ctx context.Context, rawToken string) (*oidc.IDToken, error) {
	// 1. Validate signature via JWKS
	idToken, err := c.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("token verification: %w", err)
	}

	// 2. Check JTI blocklist in Redis
	var claims struct {
		JTI string `json:"jti"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("extracting jti: %w", err)
	}

	if claims.JTI != "" && c.jtiBlocklist != nil {
		blocked, err := c.jtiBlocklist.IsBlocked(ctx, claims.JTI)
		if err != nil {
			// FAIL CLOSED if Redis is down
			return nil, fmt.Errorf("check jti blocklist: %w (FAIL CLOSED on Redis error)", err)
		}
		if blocked {
			return nil, fmt.Errorf("token is revoked")
		}
	}

	return idToken, nil
}

func ExtractToken(r *http.Request) string {
	// 1. Check Cookie (preferred for web frontend)
	if cookie, err := r.Cookie("access_token"); err == nil {
		return cookie.Value
	}

	// 2. Fallback to Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return parts[1]
}

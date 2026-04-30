package handlers

// saml.go — SAML 2.0 IdP integration handlers.
import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	iam_repo "github.com/openguard/services/iam/pkg/repository"
	shared_middleware "github.com/openguard/shared/middleware"
	"github.com/redis/go-redis/v9"
)

// spBaseURL returns the SP base URL from env, e.g. https://auth.example.com
func spBaseURL() string {
	if v := os.Getenv("SAML_SP_BASE_URL"); v != "" {
		return v
	}
	return "http://localhost:8081"
}

// generateSPKeyPair generates a new ECDSA P-256 key pair and self-signed certificate
// for use as the SP signing/encryption credential.
func generateSPKeyPair(orgID string) (certPEM, keyPEM string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("generate sp key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("openguard-sp-%s", orgID),
			Organization: []string{"OpenGuard"},
		},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("create sp cert: %w", err)
	}

	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("marshal sp key: %w", err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))

	return certPEM, keyPEM, nil
}

// buildSAMLSP constructs a crewjam/saml ServiceProvider from a stored SAMLProvider record.
func buildSAMLSP(p *iam_repo.SAMLProvider) (*saml.ServiceProvider, error) {
	base, err := url.Parse(spBaseURL())
	if err != nil {
		return nil, fmt.Errorf("parse sp base url: %w", err)
	}

	// Parse IdP metadata XML
	var idpMetadata saml.EntityDescriptor
	if err := xml.Unmarshal([]byte(p.MetadataXML), &idpMetadata); err != nil {
		return nil, fmt.Errorf("parse idp metadata: %w", err)
	}

	// Decode SP key pair
	keyBlock, _ := pem.Decode([]byte(p.SPKeyPEM))
	if keyBlock == nil {
		return nil, fmt.Errorf("invalid sp key pem")
	}
	spKey, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse sp key: %w", err)
	}

	certBlock, _ := pem.Decode([]byte(p.SPCertPEM))
	if certBlock == nil {
		return nil, fmt.Errorf("invalid sp cert pem")
	}
	spCert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse sp cert: %w", err)
	}

	sp := samlsp.DefaultServiceProvider(samlsp.Options{
		URL:         *base,
		Key:         spKey,
		Certificate: spCert,
		IDPMetadata: &idpMetadata,
	})

	return &sp, nil
}

// SAMLMetadata serves the SP metadata XML consumed by the IdP during federation setup.
// GET /auth/saml/metadata?org_id=<uuid>
func (h *Handler) SAMLMetadata(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	if orgID == "" {
		h.writeError(w, http.StatusBadRequest, "org_id required")
		return
	}

	provider, err := h.svc.GetSAMLProvider(r.Context(), orgID)
	if err != nil {
		slog.Error("saml: get provider for metadata", "org_id", orgID, "error", err)
		h.writeError(w, http.StatusNotFound, "SAML provider not configured")
		return
	}

	sp, err := buildSAMLSP(provider)
	if err != nil {
		slog.Error("saml: build sp for metadata", "org_id", orgID, "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	metadata, err := xml.MarshalIndent(sp.Metadata(), "", "  ")
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to generate metadata")
		return
	}

	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(metadata)
}

// SAMLAssertionConsumerService processes the HTTP-POST binding response from the IdP.
// POST /auth/saml/acs
func (h *Handler) SAMLAssertionConsumerService(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	orgID := r.FormValue("RelayState")
	if orgID == "" {
		h.writeError(w, http.StatusBadRequest, "missing RelayState (org_id)")
		return
	}

	provider, err := h.svc.GetSAMLProvider(r.Context(), orgID)
	if err != nil || !provider.Enabled {
		h.writeError(w, http.StatusUnauthorized, "SAML provider not configured or disabled")
		return
	}

	sp, err := buildSAMLSP(provider)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	assertion, err := sp.ParseResponse(r, nil)
	if err != nil {
		h.writeError(w, http.StatusUnauthorized, "invalid SAML response")
		return
	}

	// GAP-SEC-04: SAML Assertion Replay Protection
	assertionID := assertion.ID
	notOnOrAfter := assertion.Conditions.NotOnOrAfter
	ttl := time.Until(notOnOrAfter)
	if ttl <= 0 {
		h.writeError(w, http.StatusUnauthorized, "expired assertion")
		return
	}

	replayKey := fmt.Sprintf("saml:assertion:%s", assertionID)
	// Atomic SetNX prevents replay within the assertion's validity window.
	res, err := h.svc.Redis().SetArgs(r.Context(), replayKey, "1", redis.SetArgs{Mode: "NX", TTL: ttl}).Result()
	if (err != nil && err != redis.Nil) || res != "OK" {
		slog.Warn("saml: assertion replay detected", "assertion_id", assertionID, "org_id", orgID)
		h.writeError(w, http.StatusUnauthorized, "assertion replay detected")
		return
	}

	nameID := assertion.Subject.NameID.Value
	if nameID == "" {
		h.writeError(w, http.StatusUnauthorized, "SAML assertion missing NameID")
		return
	}

	var attrMap map[string]string
	_ = json.Unmarshal(provider.AttributeMap, &attrMap)

	email := nameID
	displayName := ""
	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			switch attr.Name {
			case "email", attrMap["email"]:
				if len(attr.Values) > 0 {
					email = attr.Values[0].Value
				}
			case "displayName", "name", attrMap["displayName"]:
				if len(attr.Values) > 0 {
					displayName = attr.Values[0].Value
				}
			}
		}
	}

	user, err := h.svc.ProvisionOrGetSAMLUser(r.Context(), orgID, nameID, email, displayName)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "user provisioning failed")
		return
	}

	userID := user.ID
	jti := fmt.Sprintf("saml-%d", time.Now().UnixNano())
	token, err := h.svc.SignToken(orgID, userID, jti, 8*time.Hour)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "token issuance failed")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "og_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("NODE_ENV") == "production",
		MaxAge:   int((8 * time.Hour).Seconds()),
		SameSite: http.SameSiteLaxMode,
	})

	dashboardURL := os.Getenv("OPENGUARD_DASHBOARD_URL")
	if dashboardURL == "" {
		dashboardURL = "http://localhost:4200"
	}
	http.Redirect(w, r, dashboardURL+"/dashboard", http.StatusFound)
}

// CreateSAMLProvider registers or updates the SAML IdP configuration for the caller's org.
func (h *Handler) CreateSAMLProvider(w http.ResponseWriter, r *http.Request) {
	orgID := shared_middleware.GetOrgID(r.Context())
	if orgID == "" {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var body struct {
		EntityID     string          `json:"entity_id"`
		SSOURL       string          `json:"sso_url"`
		SLOURL       string          `json:"slo_url"`
		MetadataXML  string          `json:"metadata_xml"`
		AttributeMap json.RawMessage `json:"attribute_map"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.MetadataXML == "" {
		h.writeError(w, http.StatusBadRequest, "metadata_xml required")
		return
	}

	var idpMeta saml.EntityDescriptor
	if err := xml.Unmarshal([]byte(body.MetadataXML), &idpMeta); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid metadata_xml: "+err.Error())
		return
	}
	if body.EntityID == "" {
		body.EntityID = idpMeta.EntityID
	}
	if body.SSOURL == "" && len(idpMeta.IDPSSODescriptors) > 0 {
		for _, sso := range idpMeta.IDPSSODescriptors[0].SingleSignOnServices {
			if sso.Binding == "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" {
				body.SSOURL = sso.Location
				break
			}
		}
	}

	certPEM, keyPEM, err := generateSPKeyPair(orgID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to generate SP credentials")
		return
	}

	attrMap := body.AttributeMap
	if attrMap == nil {
		attrMap = json.RawMessage("{}")
	}

	p := &iam_repo.SAMLProvider{
		OrgID:        orgID,
		EntityID:     body.EntityID,
		SSOURL:       body.SSOURL,
		SLOURL:       body.SLOURL,
		MetadataXML:  body.MetadataXML,
		SPCertPEM:    certPEM,
		SPKeyPEM:     keyPEM,
		NameIDFormat: "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
		AttributeMap: attrMap,
		Enabled:      true,
	}

	saved, err := h.svc.UpsertSAMLProvider(r.Context(), orgID, p)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to save SAML provider")
		return
	}

	saved.SPKeyPEM = ""
	h.writeJSON(w, http.StatusCreated, saved)
}

// ListSAMLProviders returns SAML provider configuration(s) for the caller's org.
func (h *Handler) ListSAMLProviders(w http.ResponseWriter, r *http.Request) {
	orgID := shared_middleware.GetOrgID(r.Context())
	if orgID == "" {
		h.writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	providers, err := h.svc.ListSAMLProviders(r.Context(), orgID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "failed to list SAML providers")
		return
	}

	for _, p := range providers {
		p.SPKeyPEM = ""
	}
	if providers == nil {
		providers = []*iam_repo.SAMLProvider{}
	}
	h.writeJSON(w, http.StatusOK, providers)
}

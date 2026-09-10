package oidccallbackauthn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	signozerrors "github.com/SigNoz/signoz/pkg/errors"
	"github.com/SigNoz/signoz/pkg/factory"
	"github.com/SigNoz/signoz/pkg/global"
	"github.com/SigNoz/signoz/pkg/types"
	"github.com/SigNoz/signoz/pkg/types/authtypes"
	"github.com/SigNoz/signoz/pkg/valuer"
)

const (
	testOIDCClientID     = "client-id"
	testOIDCClientSecret = "client-secret"
)

type mockAuthNStore struct {
	domain *authtypes.AuthDomain
}

func (m *mockAuthNStore) GetActiveUserAndFactorPasswordByEmailAndOrgID(context.Context, string, valuer.UUID) (*types.User, *types.FactorPassword, []*authtypes.UserRole, error) {
	return nil, nil, nil, signozerrors.New(signozerrors.TypeUnsupported, signozerrors.CodeUnsupported, "not implemented")
}

func (m *mockAuthNStore) GetAuthDomainFromID(_ context.Context, domainID valuer.UUID) (*authtypes.AuthDomain, error) {
	if m.domain == nil || m.domain.StorableAuthDomain().ID != domainID {
		return nil, signozerrors.Newf(signozerrors.TypeNotFound, signozerrors.CodeNotFound, "domain %s not found", domainID)
	}

	return m.domain, nil
}

type oidcTestServer struct {
	server         *httptest.Server
	privateKey     *rsa.PrivateKey
	keyID          string
	clientID       string
	issuer         string
	tokenClaims    map[string]any
	userInfoClaims map[string]any
}

func newOIDCTestServer(t *testing.T, tokenClaims map[string]any, userInfoClaims map[string]any) *oidcTestServer {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	testServer := &oidcTestServer{
		privateKey:     privateKey,
		keyID:          "test-key-id",
		clientID:       testOIDCClientID,
		tokenClaims:    tokenClaims,
		userInfoClaims: userInfoClaims,
	}

	handler := http.NewServeMux()
	handler.HandleFunc("/.well-known/openid-configuration", testServer.handleDiscovery)
	handler.HandleFunc("/authorize", testServer.handleAuthorize)
	handler.HandleFunc("/token", testServer.handleToken)
	handler.HandleFunc("/keys", testServer.handleKeys)
	handler.HandleFunc("/userinfo", testServer.handleUserInfo)
	handler.HandleFunc("/end_session", testServer.handleEndSession)

	testServer.server = httptest.NewServer(handler)
	testServer.issuer = testServer.server.URL

	return testServer
}

func (s *oidcTestServer) close() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *oidcTestServer) handleDiscovery(rw http.ResponseWriter, _ *http.Request) {
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(map[string]any{
		"issuer":                 s.issuer,
		"authorization_endpoint": s.issuer + "/authorize",
		"token_endpoint":         s.issuer + "/token",
		"userinfo_endpoint":      s.issuer + "/userinfo",
		"end_session_endpoint":   s.issuer + "/end_session",
		"jwks_uri":               s.issuer + "/keys",
		"id_token_signing_alg_values_supported": []string{
			"RS256",
		},
	})
}

func (*oidcTestServer) handleAuthorize(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
}

func (s *oidcTestServer) handleToken(rw http.ResponseWriter, req *http.Request) {
	if err := req.ParseForm(); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	resp := map[string]any{
		"access_token": "access-token",
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	idToken, err := s.issueIDToken()
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	resp["id_token"] = idToken

	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(resp)
}

func (s *oidcTestServer) handleKeys(rw http.ResponseWriter, _ *http.Request) {
	set := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{
			Key:       &s.privateKey.PublicKey,
			Use:       "sig",
			Algorithm: string(jose.RS256),
			KeyID:     s.keyID,
		}},
	}

	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(set)
}

func (s *oidcTestServer) handleUserInfo(rw http.ResponseWriter, _ *http.Request) {
	if s.userInfoClaims == nil {
		http.Error(rw, "userinfo unavailable", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(s.userInfoClaims)
}

func (*oidcTestServer) handleEndSession(rw http.ResponseWriter, _ *http.Request) {
	rw.WriteHeader(http.StatusOK)
}

func (s *oidcTestServer) issueIDToken() (string, error) {
	claims := jwt.MapClaims{
		"iss": s.issuer,
		"sub": "oidc-subject",
		"aud": s.clientID,
		"iat": time.Now().Add(-1 * time.Minute).Unix(),
		"exp": time.Now().Add(5 * time.Minute).Unix(),
	}

	for k, v := range s.tokenClaims {
		claims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.keyID

	return token.SignedString(s.privateKey)
}

func TestLoginURLBuildsSuccessfully(t *testing.T) {
	testServer := newOIDCTestServer(t, map[string]any{"email": "user@example.com"}, nil)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProvider(t, authDomain)

	siteURL := mustParseURL(t, "https://signoz.local/login")

	loginURL, err := provider.LoginURL(context.Background(), siteURL, authDomain)
	if err != nil {
		t.Fatalf("expected login URL, got error: %v", err)
	}

	parsed := mustParseURL(t, loginURL)
	if parsed.Host != strings.TrimPrefix(testServer.issuer, "http://") && parsed.Host != strings.TrimPrefix(testServer.issuer, "https://") {
		t.Fatalf("expected host to be OIDC provider, got %s", parsed.Host)
	}
	if parsed.Path != "/authorize" {
		t.Fatalf("expected authorization path /authorize, got %s", parsed.Path)
	}

	scope := strings.Fields(parsed.Query().Get("scope"))
	for _, expected := range []string{"openid", "profile", "email"} {
		if !slices.Contains(scope, expected) {
			t.Fatalf("expected scope %q in %v", expected, scope)
		}
	}
	if parsed.Query().Get("prompt") != "select_account" {
		t.Fatalf("expected prompt=select_account, got %s", parsed.Query().Get("prompt"))
	}

	redirectURI := parsed.Query().Get("redirect_uri")
	if !strings.HasSuffix(redirectURI, "/api/v1/complete/oidc") {
		t.Fatalf("expected redirect_uri to end with /api/v1/complete/oidc, got %s", redirectURI)
	}

	state := parsed.Query().Get("state")
	if !strings.HasPrefix(state, "v1.") {
		t.Fatalf("expected signed state with v1 prefix, got %s", state)
	}
}

func TestLogoutURLBuildsSuccessfully(t *testing.T) {
	testServer := newOIDCTestServer(t, map[string]any{"email": "user@example.com"}, nil)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProvider(t, authDomain)

	siteURL := mustParseURL(t, "https://signoz.local/login")

	logoutURL, err := provider.LogoutURL(context.Background(), siteURL, authDomain)
	if err != nil {
		t.Fatalf("expected logout URL, got error: %v", err)
	}

	parsed := mustParseURL(t, logoutURL)
	if parsed.Host != strings.TrimPrefix(testServer.issuer, "http://") && parsed.Host != strings.TrimPrefix(testServer.issuer, "https://") {
		t.Fatalf("expected host to be OIDC provider, got %s", parsed.Host)
	}
	if parsed.Path != "/end_session" {
		t.Fatalf("expected end session path /end_session, got %s", parsed.Path)
	}
	if parsed.Query().Get("post_logout_redirect_uri") != "https://signoz.local/login" {
		t.Fatalf("unexpected post_logout_redirect_uri: %s", parsed.Query().Get("post_logout_redirect_uri"))
	}
	if parsed.Query().Get("client_id") != testOIDCClientID {
		t.Fatalf("unexpected client_id: %s", parsed.Query().Get("client_id"))
	}
}

func TestHandleCallbackFailsWithoutCode(t *testing.T) {
	testServer := newOIDCTestServer(t, map[string]any{"email": "user@example.com"}, nil)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProvider(t, authDomain)

	state := authtypes.NewState(mustParseURL(t, "https://signoz.local/login"), authDomain.StorableAuthDomain().ID)

	_, err := provider.HandleCallback(context.Background(), url.Values{
		"state": {state.URL.String()},
	})
	if err == nil || !strings.Contains(err.Error(), "missing code") {
		t.Fatalf("expected missing code error, got %v", err)
	}
}

func TestHandleCallbackFailsWithInvalidState(t *testing.T) {
	testServer := newOIDCTestServer(t, map[string]any{"email": "user@example.com"}, nil)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProvider(t, authDomain)

	_, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {"not-a-valid-state"},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("expected invalid state error, got %v", err)
	}
}

func TestHandleCallbackExtractsEmailFromIDToken(t *testing.T) {
	testServer := newOIDCTestServer(
		t,
		map[string]any{
			"email":          "alice@example.com",
			"name":           "Alice",
			"email_verified": true,
		},
		nil,
	)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProvider(t, authDomain)

	state := mustSignedStateFromLoginURL(t, provider, authDomain, mustParseURL(t, "https://signoz.local/login"))
	identity, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {state},
	})
	if err != nil {
		t.Fatalf("expected callback success, got error: %v", err)
	}

	if identity.Email.StringValue() != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %s", identity.Email.StringValue())
	}
	if identity.Name != "Alice" {
		t.Fatalf("expected name Alice, got %s", identity.Name)
	}
}

func TestHandleCallbackFallsBackToUserInfoForEmail(t *testing.T) {
	testServer := newOIDCTestServer(
		t,
		map[string]any{
			"name":           "Fallback User",
			"email_verified": true,
		},
		map[string]any{
			"email": "fallback@example.com",
			"name":  "Fallback User",
		},
	)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, true)
	provider := mustNewProvider(t, authDomain)

	state := mustSignedStateFromLoginURL(t, provider, authDomain, mustParseURL(t, "https://signoz.local/login"))
	identity, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {state},
	})
	if err != nil {
		t.Fatalf("expected callback success using userinfo fallback, got error: %v", err)
	}

	if identity.Email.StringValue() != "fallback@example.com" {
		t.Fatalf("expected fallback email, got %s", identity.Email.StringValue())
	}
}

func TestHandleCallbackFallsBackToUserInfoWhenIDTokenEmailEmpty(t *testing.T) {
	testServer := newOIDCTestServer(
		t,
		map[string]any{
			"email":          "",
			"name":           "Fallback User",
			"email_verified": true,
		},
		map[string]any{
			"email": "fallback-empty@example.com",
			"name":  "Fallback User",
		},
	)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, true)
	provider := mustNewProvider(t, authDomain)

	state := mustSignedStateFromLoginURL(t, provider, authDomain, mustParseURL(t, "https://signoz.local/login"))
	identity, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {state},
	})
	if err != nil {
		t.Fatalf("expected callback success using userinfo fallback for empty email, got error: %v", err)
	}

	if identity.Email.StringValue() != "fallback-empty@example.com" {
		t.Fatalf("expected fallback email from userinfo, got %s", identity.Email.StringValue())
	}
}

func TestHandleCallbackFailsWhenEmailMissingEverywhere(t *testing.T) {
	testServer := newOIDCTestServer(
		t,
		map[string]any{
			"name":           "No Email",
			"email_verified": true,
		},
		map[string]any{
			"name": "No Email",
		},
	)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, true)
	provider := mustNewProvider(t, authDomain)

	state := mustSignedStateFromLoginURL(t, provider, authDomain, mustParseURL(t, "https://signoz.local/login"))
	_, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {state},
	})
	if err == nil || !strings.Contains(err.Error(), "missing email in claims") {
		t.Fatalf("expected missing email error, got %v", err)
	}
}

func TestHandleCallbackFailsWhenStateSignatureIsTampered(t *testing.T) {
	testServer := newOIDCTestServer(
		t,
		map[string]any{
			"email":          "alice@example.com",
			"name":           "Alice",
			"email_verified": true,
		},
		nil,
	)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProvider(t, authDomain)

	state := mustSignedStateFromLoginURL(t, provider, authDomain, mustParseURL(t, "https://signoz.local/login"))
	state = state[:len(state)-1] + "X"

	_, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {state},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("expected invalid state error, got %v", err)
	}
}

func TestHandleCallbackStrictEmailVerifiedPolicyBlocksUnverifiedEmail(t *testing.T) {
	testServer := newOIDCTestServer(
		t,
		map[string]any{
			"email":          "alice@example.com",
			"name":           "Alice",
			"email_verified": false,
		},
		nil,
	)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false, func(config *authtypes.OIDCConfig) {
		config.EmailVerifiedPolicy = authtypes.OIDCEmailVerifiedPolicyStrict
	})
	provider := mustNewProvider(t, authDomain)

	state := mustSignedStateFromLoginURL(t, provider, authDomain, mustParseURL(t, "https://signoz.local/login"))
	_, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {state},
	})
	if err == nil || !strings.Contains(err.Error(), "email is not verified") {
		t.Fatalf("expected strict email verification failure, got %v", err)
	}
}

func TestHandleCallbackEnforcesEmailDomainWhenConfigured(t *testing.T) {
	testServer := newOIDCTestServer(
		t,
		map[string]any{
			"email":          "alice@other.com",
			"name":           "Alice",
			"email_verified": true,
		},
		nil,
	)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false, func(config *authtypes.OIDCConfig) {
		config.EnforceEmailDomain = true
	})
	provider := mustNewProvider(t, authDomain)

	state := mustSignedStateFromLoginURL(t, provider, authDomain, mustParseURL(t, "https://signoz.local/login"))
	_, err := provider.HandleCallback(context.Background(), url.Values{
		"code":  {"valid-code"},
		"state": {state},
	})
	if err == nil || !strings.Contains(err.Error(), "email domain mismatch") {
		t.Fatalf("expected email domain mismatch error, got %v", err)
	}
}

func TestLoginURLRespectsConfiguredScopesAndAlwaysAddsOpenID(t *testing.T) {
	testServer := newOIDCTestServer(t, map[string]any{"email": "user@example.com"}, nil)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false, func(config *authtypes.OIDCConfig) {
		config.Scopes = []string{"email", "profile", "custom_scope"}
	})
	provider := mustNewProvider(t, authDomain)

	siteURL := mustParseURL(t, "https://signoz.local/login")
	loginURL, err := provider.LoginURL(context.Background(), siteURL, authDomain)
	if err != nil {
		t.Fatalf("expected login URL, got error: %v", err)
	}

	parsed := mustParseURL(t, loginURL)
	scope := strings.Fields(parsed.Query().Get("scope"))

	for _, expected := range []string{"openid", "email", "profile", "custom_scope"} {
		if !slices.Contains(scope, expected) {
			t.Fatalf("expected scope %q in %v", expected, scope)
		}
	}
}

func mustNewProvider(t *testing.T, authDomain *authtypes.AuthDomain) *AuthN {
	t.Helper()

	return mustNewProviderWithGlobalConfig(t, authDomain, global.Config{})
}

// mustNewProviderWithGlobalConfig builds a provider whose external URL carries the
// given base path, so the callback and post-logout URLs can be asserted against it.
func mustNewProviderWithBasePath(t *testing.T, authDomain *authtypes.AuthDomain, basePath string) *AuthN {
	t.Helper()

	return mustNewProviderWithGlobalConfig(t, authDomain, global.Config{
		ExternalURL: &url.URL{Scheme: "https", Host: "signoz.local", Path: basePath},
	})
}

func mustNewProviderWithGlobalConfig(t *testing.T, authDomain *authtypes.AuthDomain, globalConfig global.Config) *AuthN {
	t.Helper()

	provider, err := New(&mockAuthNStore{domain: authDomain}, factory.ProviderSettings{
		Logger:               slog.New(slog.DiscardHandler),
		MeterProvider:        noop.NewMeterProvider(),
		TracerProvider:       tracenoop.NewTracerProvider(),
		PrometheusRegisterer: prometheus.NewRegistry(),
	}, globalConfig)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	return provider
}

func mustNewOIDCDomain(t *testing.T, issuer string, getUserInfo bool, overrides ...func(*authtypes.OIDCConfig)) *authtypes.AuthDomain {
	t.Helper()

	oidcConfig := authtypes.OIDCConfig{
		Issuer:       issuer,
		ClientID:     testOIDCClientID,
		ClientSecret: testOIDCClientSecret,
		ClaimMapping: authtypes.AttributeMapping{
			Email:  "email",
			Name:   "name",
			Groups: "groups",
			Role:   "role",
		},
		GetUserInfo: getUserInfo,
	}

	for _, override := range overrides {
		override(&oidcConfig)
	}

	authDomain, err := authtypes.NewAuthDomainFromPostableAuthDomain(&authtypes.PostableAuthDomain{
		Name:    "example.com",
		Enabled: true,
		Config: authtypes.AuthDomainConfig{
			Kind: authtypes.AuthNProviderOIDC,
			Spec: oidcConfig,
		},
	}, valuer.GenerateUUID())
	if err != nil {
		t.Fatalf("failed to create auth domain: %v", err)
	}

	return authDomain
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("failed to parse URL %q: %v", raw, err)
	}

	return u
}

func mustSignedStateFromLoginURL(t *testing.T, provider *AuthN, authDomain *authtypes.AuthDomain, siteURL *url.URL) string {
	t.Helper()

	loginURL, err := provider.LoginURL(context.Background(), siteURL, authDomain)
	if err != nil {
		t.Fatalf("failed to build login URL: %v", err)
	}

	parsed := mustParseURL(t, loginURL)
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatal("expected signed state in login URL")
	}

	return state
}

// Upstream added base path handling to every callback provider in v0.128.0 (#11588).
// These two tests pin it for this fork's community OIDC provider: when
// global::external_url carries a sub path, the URLs handed to the IdP must carry it too,
// otherwise the callback lands on a 404.
func TestLoginURLIncludesExternalBasePathInRedirectURI(t *testing.T) {
	testServer := newOIDCTestServer(t, map[string]any{"email": "user@example.com"}, nil)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProviderWithBasePath(t, authDomain, "/signoz")

	siteURL := mustParseURL(t, "https://signoz.local/signoz/login")

	loginURL, err := provider.LoginURL(context.Background(), siteURL, authDomain)
	if err != nil {
		t.Fatalf("expected login URL, got error: %v", err)
	}

	parsed := mustParseURL(t, loginURL)
	if got, want := parsed.Query().Get("redirect_uri"), "https://signoz.local/signoz/api/v1/complete/oidc"; got != want {
		t.Fatalf("expected redirect_uri %s, got %s", want, got)
	}
}

func TestLogoutURLIncludesExternalBasePathInPostLogoutRedirectURI(t *testing.T) {
	testServer := newOIDCTestServer(t, map[string]any{"email": "user@example.com"}, nil)
	defer testServer.close()

	authDomain := mustNewOIDCDomain(t, testServer.issuer, false)
	provider := mustNewProviderWithBasePath(t, authDomain, "/signoz")

	siteURL := mustParseURL(t, "https://signoz.local/signoz/login")

	logoutURL, err := provider.LogoutURL(context.Background(), siteURL, authDomain)
	if err != nil {
		t.Fatalf("expected logout URL, got error: %v", err)
	}

	parsed := mustParseURL(t, logoutURL)
	if got, want := parsed.Query().Get("post_logout_redirect_uri"), "https://signoz.local/signoz/login"; got != want {
		t.Fatalf("expected post_logout_redirect_uri %s, got %s", want, got)
	}
}

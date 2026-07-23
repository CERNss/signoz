package oidccallbackauthn

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/SigNoz/signoz/pkg/authn"
	"github.com/SigNoz/signoz/pkg/errors"
	"github.com/SigNoz/signoz/pkg/factory"
	"github.com/SigNoz/signoz/pkg/http/client"
	"github.com/SigNoz/signoz/pkg/types/authtypes"
	"github.com/SigNoz/signoz/pkg/valuer"
)

const (
	redirectPath          string = "/api/v1/complete/oidc"
	postLogoutPath        string = "/login"
	stateSignatureVersion string = "v1"
)

var _ authn.CallbackAuthN = (*AuthN)(nil)
var _ authn.LogoutURLProvider = (*AuthN)(nil)

type AuthN struct {
	settings   factory.ScopedProviderSettings
	store      authtypes.AuthNStore
	httpClient *client.Client
}

func New(store authtypes.AuthNStore, providerSettings factory.ProviderSettings) (*AuthN, error) {
	settings := factory.NewScopedProviderSettings(providerSettings, "github.com/SigNoz/signoz/pkg/authn/callbackauthn/oidccallbackauthn")

	httpClient, err := client.New(providerSettings.Logger, providerSettings.TracerProvider, providerSettings.MeterProvider)
	if err != nil {
		return nil, err
	}

	return &AuthN{
		settings:   settings,
		store:      store,
		httpClient: httpClient,
	}, nil
}

func (a *AuthN) LoginURL(ctx context.Context, siteURL *url.URL, authDomain *authtypes.AuthDomain) (string, error) {
	if authDomain.Kind() != authtypes.AuthNProviderOIDC {
		return "", errors.Newf(errors.TypeInternal, authtypes.ErrCodeAuthDomainMismatch, "domain type is not oidc")
	}

	_, oauth2Config, err := a.oidcProviderAndOAuth2Config(ctx, siteURL, authDomain)
	if err != nil {
		return "", err
	}

	stateSigningSecret, err := a.stateSigningSecret(authDomain)
	if err != nil {
		return "", err
	}

	state, err := signStateValue(authtypes.NewState(siteURL, authDomain.StorableAuthDomain().ID).URL.String(), stateSigningSecret)
	if err != nil {
		return "", err
	}

	return oauth2Config.AuthCodeURL(state, oauth2.SetAuthURLParam("prompt", "select_account")), nil
}

func (a *AuthN) HandleCallback(ctx context.Context, query url.Values) (*authtypes.CallbackIdentity, error) {
	if err := query.Get("error"); err != "" {
		return nil, errors.Newf(errors.TypeInternal, errors.CodeInternal, "oidc: error while authenticating").WithAdditional(query.Get("error_description"))
	}

	if query.Get("code") == "" {
		return nil, errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: missing code in callback")
	}

	state, payload, signature, err := parseSignedStateValue(query.Get("state"))
	if err != nil {
		return nil, errors.Newf(errors.TypeInvalidInput, authtypes.ErrCodeInvalidState, "oidc: invalid state").WithAdditional(err.Error())
	}

	authDomain, err := a.store.GetAuthDomainFromID(ctx, state.DomainID)
	if err != nil {
		return nil, err
	}

	oidcConfig, err := authDomain.Config().OIDCConfig()
	if err != nil {
		return nil, err
	}

	stateSigningSecret, err := a.stateSigningSecret(authDomain)
	if err != nil {
		return nil, err
	}

	if err := verifySignedStateSignature(payload, signature, stateSigningSecret); err != nil {
		return nil, errors.Newf(errors.TypeInvalidInput, authtypes.ErrCodeInvalidState, "oidc: invalid state").WithAdditional(err.Error())
	}

	oidcProvider, oauth2Config, err := a.oidcProviderAndOAuth2Config(ctx, state.URL, authDomain)
	if err != nil {
		return nil, err
	}

	ctx = context.WithValue(ctx, oauth2.HTTPClient, a.httpClient.Client())
	token, err := oauth2Config.Exchange(ctx, query.Get("code"))
	if err != nil {
		var retrieveError *oauth2.RetrieveError
		if errors.As(err, &retrieveError) {
			return nil, errors.Newf(errors.TypeForbidden, errors.CodeForbidden, "oidc: failed to get token").WithAdditional(retrieveError.ErrorDescription).WithAdditional(string(retrieveError.Body))
		}

		return nil, errors.Newf(errors.TypeInternal, errors.CodeInternal, "oidc: failed to get token").WithAdditional(err.Error())
	}

	claims := map[string]any{}
	idTokenClaims, err := a.claimsFromIDToken(ctx, authDomain, oidcProvider, token)
	if err != nil && !errors.Ast(err, errors.TypeNotFound) {
		return nil, err
	}

	for k, v := range idTokenClaims {
		claims[k] = v
	}

	emailClaimName := oidcConfig.ClaimMapping.Email
	if emailClaimName == "" {
		emailClaimName = "email"
	}

	// Fetch user info either when explicitly requested, or when email is missing from id_token.
	idTokenEmail, hasEmailInIDToken := claims[emailClaimName].(string)
	hasEmailInIDToken = hasEmailInIDToken && idTokenEmail != ""
	if oidcConfig.GetUserInfo || !hasEmailInIDToken {
		userInfoClaims, err := a.claimsFromUserInfo(ctx, oidcProvider, token)
		if err != nil {
			if !hasEmailInIDToken {
				return nil, err
			}
		} else {
			for k, v := range userInfoClaims {
				existing, exists := claims[k]
				if !exists || existing == "" {
					claims[k] = v
				}
			}
		}
	}

	emailClaim, ok := claims[emailClaimName].(string)
	if !ok || emailClaim == "" {
		return nil, errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: missing email in claims")
	}

	email, err := valuer.NewEmail(emailClaim)
	if err != nil {
		return nil, errors.Newf(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: failed to parse email").WithAdditional(err.Error())
	}

	switch oidcConfig.EffectiveEmailVerifiedPolicy() {
	case authtypes.OIDCEmailVerifiedPolicyStrict:
		emailVerified, exists := claims["email_verified"]
		if !exists {
			return nil, errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: missing email_verified in claims")
		}

		verified, ok := emailVerified.(bool)
		if !ok {
			return nil, errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: invalid email_verified in claims")
		}

		if !verified {
			return nil, errors.New(errors.TypeForbidden, errors.CodeForbidden, "oidc: email is not verified")
		}
	case authtypes.OIDCEmailVerifiedPolicyWarn:
		if emailVerified, exists := claims["email_verified"]; exists {
			if verified, ok := emailVerified.(bool); ok && !verified {
				a.settings.Logger().WarnContext(ctx, "oidc: email is not verified", slog.String("email", email.StringValue()))
			}
		}
	}

	if oidcConfig.EnforceEmailDomain && !emailDomainMatches(email.StringValue(), authDomain.StorableAuthDomain().Name) {
		return nil, errors.Newf(errors.TypeForbidden, errors.CodeForbidden, "oidc: email domain mismatch").WithAdditional(fmt.Sprintf("expected domain %q", authDomain.StorableAuthDomain().Name))
	}

	name := resolveName(claims, oidcConfig.ClaimMapping.Name)

	groups := extractGroups(ctx, claims, oidcConfig.ClaimMapping.Groups, a.settings.Logger())

	role := ""
	if roleClaim := oidcConfig.ClaimMapping.Role; roleClaim != "" {
		if r, ok := claims[roleClaim].(string); ok {
			role = r
		}
	}

	return authtypes.NewCallbackIdentity(name, email, authDomain.StorableAuthDomain().OrgID, state, groups, role), nil
}

func (a *AuthN) ProviderInfo(context.Context, *authtypes.AuthDomain) *authtypes.AuthNProviderInfo {
	return &authtypes.AuthNProviderInfo{
		RelayStatePath: nil,
	}
}

func (a *AuthN) LogoutURL(ctx context.Context, siteURL *url.URL, authDomain *authtypes.AuthDomain) (string, error) {
	if authDomain.Kind() != authtypes.AuthNProviderOIDC {
		return "", errors.Newf(errors.TypeInternal, authtypes.ErrCodeAuthDomainMismatch, "domain type is not oidc")
	}
	if siteURL == nil || siteURL.Scheme == "" || siteURL.Host == "" {
		return "", errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: invalid site URL for logout")
	}

	oidcConfig, err := authDomain.Config().OIDCConfig()
	if err != nil {
		return "", err
	}

	oidcProvider, _, err := a.oidcProviderAndOAuth2Config(ctx, siteURL, authDomain)
	if err != nil {
		return "", err
	}

	metadata := struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}{}
	if err := oidcProvider.Claims(&metadata); err != nil {
		return "", errors.Newf(errors.TypeInternal, errors.CodeInternal, "oidc: failed to decode provider metadata").WithAdditional(err.Error())
	}

	if metadata.EndSessionEndpoint == "" {
		return "", nil
	}

	endSessionURL, err := url.Parse(metadata.EndSessionEndpoint)
	if err != nil {
		return "", errors.Newf(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: invalid end_session endpoint").WithAdditional(err.Error())
	}

	postLogoutRedirectURI := (&url.URL{
		Scheme: siteURL.Scheme,
		Host:   siteURL.Host,
		Path:   postLogoutPath,
	}).String()

	query := endSessionURL.Query()
	query.Set("post_logout_redirect_uri", postLogoutRedirectURI)
	query.Set("client_id", oidcConfig.ClientID)
	endSessionURL.RawQuery = query.Encode()

	return endSessionURL.String(), nil
}

func (a *AuthN) oidcProviderAndOAuth2Config(ctx context.Context, siteURL *url.URL, authDomain *authtypes.AuthDomain) (*oidc.Provider, *oauth2.Config, error) {
	oidcConfig, err := authDomain.Config().OIDCConfig()
	if err != nil {
		return nil, nil, err
	}

	if oidcConfig.IssuerAlias != "" {
		ctx = oidc.InsecureIssuerURLContext(ctx, oidcConfig.IssuerAlias)
	}

	oidcProvider, err := oidc.NewProvider(ctx, oidcConfig.Issuer)
	if err != nil {
		return nil, nil, err
	}

	scopes := oidcConfig.EffectiveScopes()

	if authDomain.RoleMapping() != nil && len(authDomain.RoleMapping().GroupMappings) > 0 {
		hasGroups := false
		for _, scope := range scopes {
			if scope == "groups" {
				hasGroups = true
				break
			}
		}

		if !hasGroups {
			scopes = append(scopes, "groups")
		}
	}

	return oidcProvider, &oauth2.Config{
		ClientID:     oidcConfig.ClientID,
		ClientSecret: oidcConfig.ClientSecret,
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       scopes,
		RedirectURL: (&url.URL{
			Scheme: siteURL.Scheme,
			Host:   siteURL.Host,
			Path:   redirectPath,
		}).String(),
	}, nil
}

func (a *AuthN) claimsFromIDToken(ctx context.Context, authDomain *authtypes.AuthDomain, provider *oidc.Provider, token *oauth2.Token) (map[string]any, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New(errors.TypeNotFound, errors.CodeNotFound, "oidc: no id_token in token response")
	}

	oidcConfig, err := authDomain.Config().OIDCConfig()
	if err != nil {
		return nil, err
	}

	verifier := provider.Verifier(&oidc.Config{ClientID: oidcConfig.ClientID})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, errors.Newf(errors.TypeForbidden, errors.CodeForbidden, "oidc: failed to verify token").WithAdditional(err.Error())
	}

	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		return nil, errors.Newf(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: failed to decode claims").WithAdditional(err.Error())
	}

	return claims, nil
}

func (a *AuthN) claimsFromUserInfo(ctx context.Context, provider *oidc.Provider, token *oauth2.Token) (map[string]any, error) {
	var claims map[string]any

	userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: token.AccessToken,
		TokenType:   "Bearer", // UserInfo endpoint requires bearer token (RFC6750).
	}))
	if err != nil {
		return nil, errors.Newf(errors.TypeInternal, errors.CodeInternal, "oidc: failed to get user info").WithAdditional(err.Error())
	}

	if err := userInfo.Claims(&claims); err != nil {
		return nil, errors.Newf(errors.TypeInvalidInput, errors.CodeInvalidInput, "oidc: failed to decode claims").WithAdditional(err.Error())
	}

	return claims, nil
}

func resolveName(claims map[string]any, configuredClaim string) string {
	if configuredClaim != "" {
		if n, ok := claims[configuredClaim].(string); ok {
			return n
		}
	}

	for _, key := range []string{"name", "preferred_username"} {
		if n, ok := claims[key].(string); ok {
			return n
		}
	}

	return ""
}

func extractGroups(ctx context.Context, claims map[string]any, groupsClaim string, logger *slog.Logger) []string {
	if groupsClaim == "" {
		return nil
	}

	claimValue, exists := claims[groupsClaim]
	if !exists {
		return nil
	}

	var groups []string
	switch g := claimValue.(type) {
	case []any:
		for _, group := range g {
			if gs, ok := group.(string); ok {
				groups = append(groups, gs)
			}
		}
	case []string:
		groups = append(groups, g...)
	case string:
		groups = append(groups, g)
	default:
		logger.WarnContext(ctx, "oidc: unsupported groups type", slog.String("type", fmt.Sprintf("%T", claimValue)))
	}

	return groups
}

func (a *AuthN) stateSigningSecret(authDomain *authtypes.AuthDomain) (string, error) {
	oidcConfig, err := authDomain.Config().OIDCConfig()
	if err != nil {
		return "", err
	}

	return oidcConfig.ClientSecret, nil
}

func signStateValue(rawState string, secret string) (string, error) {
	if rawState == "" {
		return "", errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "state cannot be empty")
	}

	if secret == "" {
		return "", errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "state signing secret cannot be empty")
	}

	payload := base64.RawURLEncoding.EncodeToString([]byte(rawState))
	signature := computeStateSignature(payload, secret)

	return strings.Join([]string{stateSignatureVersion, payload, signature}, "."), nil
}

func parseSignedStateValue(stateParam string) (authtypes.State, string, string, error) {
	parts := strings.Split(stateParam, ".")
	if len(parts) != 3 || parts[0] != stateSignatureVersion {
		return authtypes.State{}, "", "", errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "invalid state format")
	}

	payload := parts[1]
	signature := parts[2]
	if payload == "" || signature == "" {
		return authtypes.State{}, "", "", errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "invalid state format")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return authtypes.State{}, "", "", err
	}

	state, err := authtypes.NewStateFromString(string(decoded))
	if err != nil {
		return authtypes.State{}, "", "", err
	}

	return state, payload, signature, nil
}

func verifySignedStateSignature(payload string, signature string, secret string) error {
	if secret == "" {
		return errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "state signing secret cannot be empty")
	}

	expected := computeStateSignature(payload, secret)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return errors.New(errors.TypeForbidden, errors.CodeForbidden, "state signature mismatch")
	}

	return nil
}

func computeStateSignature(payload string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func emailDomainMatches(email string, domain string) bool {
	idx := strings.LastIndex(email, "@")
	if idx == -1 || idx == len(email)-1 {
		return false
	}

	return strings.EqualFold(email[idx+1:], domain)
}

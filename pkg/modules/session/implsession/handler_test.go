package implsession

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/SigNoz/signoz/pkg/errors"
	"github.com/SigNoz/signoz/pkg/types/authtypes"
	"github.com/SigNoz/signoz/pkg/valuer"
)

type mockSessionModule struct {
	createCallbackAuthNSession func(context.Context, authtypes.AuthNProvider, url.Values) (string, error)
	getSessionSSOContext       func(context.Context, *url.URL) (*authtypes.SessionSSOContext, error)
	getSessionLogoutContext    func(context.Context, *url.URL) (*authtypes.SessionLogoutContext, error)
}

func (m *mockSessionModule) GetSessionContext(context.Context, valuer.Email, *url.URL) (*authtypes.SessionContext, error) {
	return nil, errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
}

func (m *mockSessionModule) GetSessionSSOContext(ctx context.Context, siteURL *url.URL) (*authtypes.SessionSSOContext, error) {
	if m.getSessionSSOContext == nil {
		return nil, errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
	}

	return m.getSessionSSOContext(ctx, siteURL)
}

func (m *mockSessionModule) CreatePasswordAuthNSession(context.Context, authtypes.AuthNProvider, valuer.Email, string, valuer.UUID) (*authtypes.Token, error) {
	return nil, errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
}

func (m *mockSessionModule) CreateCallbackAuthNSession(ctx context.Context, authNProvider authtypes.AuthNProvider, values url.Values) (string, error) {
	if m.createCallbackAuthNSession == nil {
		return "", errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
	}

	return m.createCallbackAuthNSession(ctx, authNProvider, values)
}

func (m *mockSessionModule) RotateSession(context.Context, string, string) (*authtypes.Token, error) {
	return nil, errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
}

func (m *mockSessionModule) DeleteSession(context.Context, string) error {
	return errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
}

func (m *mockSessionModule) GetSessionLogoutContext(ctx context.Context, siteURL *url.URL) (*authtypes.SessionLogoutContext, error) {
	if m.getSessionLogoutContext == nil {
		return nil, errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
	}

	return m.getSessionLogoutContext(ctx, siteURL)
}

func (*mockSessionModule) GetRotationInterval(context.Context) time.Duration {
	return 0
}

func TestCreateSessionByOIDCCallbackRedirectsToProviderURL(t *testing.T) {
	h := &handler{module: &mockSessionModule{
		createCallbackAuthNSession: func(_ context.Context, provider authtypes.AuthNProvider, values url.Values) (string, error) {
			if provider != authtypes.AuthNProviderOIDC {
				t.Fatalf("expected oidc provider, got %s", provider.StringValue())
			}
			if values.Get("code") != "valid-code" {
				t.Fatalf("expected code to be forwarded")
			}

			return "https://signoz.local/login?accessToken=test-access&refreshToken=test-refresh", nil
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/complete/oidc?code=valid-code&state=ok", nil)
	rw := httptest.NewRecorder()

	h.CreateSessionByOIDCCallback(rw, req)

	resp := rw.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", resp.StatusCode)
	}

	if got := resp.Header.Get("Location"); got != "https://signoz.local/login?accessToken=test-access&refreshToken=test-refresh" {
		t.Fatalf("unexpected redirect URL: %s", got)
	}
}

func TestCreateSessionByOIDCCallbackRedirectsToLoginOnError(t *testing.T) {
	h := &handler{module: &mockSessionModule{
		createCallbackAuthNSession: func(context.Context, authtypes.AuthNProvider, url.Values) (string, error) {
			return "", errors.Newf(errors.TypeForbidden, errors.CodeForbidden, "oidc callback failed").WithAdditional("bad token")
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/complete/oidc?code=invalid&state=ok", nil)
	rw := httptest.NewRecorder()

	h.CreateSessionByOIDCCallback(rw, req)

	resp := rw.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/login?") {
		t.Fatalf("expected login redirect, got %s", location)
	}
	if !strings.Contains(location, "callbackauthnerr=true") {
		t.Fatalf("expected callbackauthnerr flag in redirect, got %s", location)
	}
	if !strings.Contains(location, "code=forbidden") {
		t.Fatalf("expected error code in redirect, got %s", location)
	}
}

func TestGetSessionSSOContextReturnsDomains(t *testing.T) {
	h := &handler{module: &mockSessionModule{
		getSessionSSOContext: func(_ context.Context, siteURL *url.URL) (*authtypes.SessionSSOContext, error) {
			if siteURL == nil {
				t.Fatalf("expected siteURL to be forwarded")
			}
			if siteURL.Host != "signoz.local" {
				t.Fatalf("unexpected siteURL host: %s", siteURL.Host)
			}

			return authtypes.NewSessionSSOContext().AddSSODomainContext(
				authtypes.NewSSODomainContext(
					"signoz.io",
					authtypes.AuthNProviderOIDC,
					"https://issuer.local/auth?client_id=abc",
				),
			), nil
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sessions/sso_context?ref=https://signoz.local/login", nil)
	rw := httptest.NewRecorder()

	h.GetSessionSSOContext(rw, req)

	resp := rw.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestGetSessionLogoutContextReturnsURL(t *testing.T) {
	h := &handler{module: &mockSessionModule{
		getSessionLogoutContext: func(_ context.Context, siteURL *url.URL) (*authtypes.SessionLogoutContext, error) {
			if siteURL == nil {
				t.Fatalf("expected siteURL to be forwarded")
			}
			if siteURL.Host != "signoz.local" {
				t.Fatalf("unexpected siteURL host: %s", siteURL.Host)
			}

			return authtypes.NewSessionLogoutContext("https://issuer.local/end_session?post_logout_redirect_uri=https%3A%2F%2Fsignoz.local%2Flogin"), nil
		},
	}}

	req := httptest.NewRequest(http.MethodGet, "/api/v2/sessions/logout_context?ref=https://signoz.local/login", nil)
	rw := httptest.NewRecorder()

	h.GetSessionLogoutContext(rw, req)

	resp := rw.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

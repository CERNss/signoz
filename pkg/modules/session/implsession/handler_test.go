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
}

func (m *mockSessionModule) GetSessionContext(context.Context, valuer.Email, *url.URL) (*authtypes.SessionContext, error) {
	return nil, errors.New(errors.TypeUnsupported, errors.CodeUnsupported, "not implemented")
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

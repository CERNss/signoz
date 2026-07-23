package implsession

import (
	"context"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/SigNoz/signoz/pkg/authn"
	"github.com/SigNoz/signoz/pkg/authz"
	"github.com/SigNoz/signoz/pkg/errors"
	"github.com/SigNoz/signoz/pkg/factory"
	"github.com/SigNoz/signoz/pkg/global"
	"github.com/SigNoz/signoz/pkg/modules/authdomain"
	"github.com/SigNoz/signoz/pkg/modules/organization"
	"github.com/SigNoz/signoz/pkg/modules/session"
	"github.com/SigNoz/signoz/pkg/modules/user"
	"github.com/SigNoz/signoz/pkg/tokenizer"
	"github.com/SigNoz/signoz/pkg/types"
	"github.com/SigNoz/signoz/pkg/types/authtypes"
	"github.com/SigNoz/signoz/pkg/valuer"
)

type module struct {
	settings     factory.ScopedProviderSettings
	authNs       map[authtypes.AuthNProvider]authn.AuthN
	userSetter   user.Setter
	userGetter   user.Getter
	authDomain   authdomain.Module
	tokenizer    tokenizer.Tokenizer
	orgGetter    organization.Getter
	authz        authz.AuthZ
	globalConfig global.Config
}

func NewModule(providerSettings factory.ProviderSettings, authNs map[authtypes.AuthNProvider]authn.AuthN, userSetter user.Setter, userGetter user.Getter, authDomain authdomain.Module, tokenizer tokenizer.Tokenizer, orgGetter organization.Getter, authz authz.AuthZ, globalConfig global.Config) session.Module {
	return &module{
		settings:     factory.NewScopedProviderSettings(providerSettings, "github.com/SigNoz/signoz/pkg/modules/session/implsession"),
		authNs:       authNs,
		userSetter:   userSetter,
		userGetter:   userGetter,
		authDomain:   authDomain,
		tokenizer:    tokenizer,
		orgGetter:    orgGetter,
		authz:        authz,
		globalConfig: globalConfig,
	}
}

func (module *module) GetSessionContext(ctx context.Context, email valuer.Email, siteURL *url.URL) (*authtypes.SessionContext, error) {
	context := authtypes.NewSessionContext()

	orgs, err := module.orgGetter.ListByOwnedKeyRange(ctx)
	if err != nil {
		return nil, err
	}

	if len(orgs) == 0 {
		context.Exists = false
		return context, nil
	}

	var orgIDs []valuer.UUID
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.ID)
	}

	users, err := module.userGetter.ListUsersByEmailAndOrgIDs(ctx, email, orgIDs)
	if err != nil {
		return nil, err
	}

	// filter out deleted users
	users = slices.DeleteFunc(users, func(user *types.User) bool { return user.ErrIfDeleted() != nil })

	// Since email is a valuer, we can be sure that it is a valid email and we can split it to get the domain name.
	name := strings.Split(email.String(), "@")[1]

	if len(users) == 0 {
		context.Exists = false

		for _, org := range orgs {
			orgContext, err := module.getOrgSessionContext(ctx, org, name, siteURL)
			if err != nil {
				// For some reason, there was an error in getting the org session context. Instead of failing the context call, we create a PasswordAuthNSupport for the org and add a warning.
				orgContext = authtypes.NewOrgSessionContext(org.ID, org.Name).AddPasswordAuthNSupport(authtypes.AuthNProviderEmailPassword).AddWarning(err)
			}

			context = context.AddOrgContext(orgContext)
		}

		return context, nil
	}

	context.Exists = true
	for _, user := range users {
		idx := slices.IndexFunc(orgs, func(org *types.Organization) bool {
			return org.ID == user.OrgID
		})

		if idx == -1 {
			continue
		}

		org := orgs[idx]
		orgContext, err := module.getOrgSessionContext(ctx, org, name, siteURL)
		if err != nil {
			// For some reason, there was an error in getting the org session context. Instead of failing the context call, we create a PasswordAuthNSupport for the org and add a warning.
			orgContext = authtypes.NewOrgSessionContext(org.ID, org.Name).AddPasswordAuthNSupport(authtypes.AuthNProviderEmailPassword).AddWarning(err)
		}

		context = context.AddOrgContext(orgContext)
	}

	return context, nil
}

func (module *module) GetSessionSSOContext(ctx context.Context, siteURL *url.URL) (*authtypes.SessionSSOContext, error) {
	context := authtypes.NewSessionSSOContext()

	if !module.globalConfig.IsOriginAllowed(siteURL) {
		return nil, errors.Newf(errors.TypeInvalidInput, global.ErrCodeOriginNotAllowed, "ref %q is not an allowed origin", siteURL.String())
	}

	orgs, err := module.orgGetter.ListByOwnedKeyRange(ctx)
	if err != nil {
		return nil, err
	}

	for _, org := range orgs {
		authDomains, err := module.authDomain.ListByOrgID(ctx, org.ID)
		if err != nil {
			return nil, err
		}

		for _, authDomain := range authDomains {
			if !authDomain.AuthDomainConfig().SSOEnabled {
				continue
			}

			provider, err := getProvider[authn.CallbackAuthN](authDomain.AuthDomainConfig().AuthNProvider, module.authNs)
			if err != nil {
				module.settings.Logger().WarnContext(
					ctx,
					"failed to resolve callback authn provider for sso shortcut",
					errors.Attr(err),
					slog.Any("authn_provider", authDomain.AuthDomainConfig().AuthNProvider),
					slog.String("domain", authDomain.StorableAuthDomain().Name),
					slog.String("org_id", org.ID.StringValue()),
				)
				continue
			}

			loginURL, err := provider.LoginURL(ctx, siteURL, authDomain)
			if err != nil {
				module.settings.Logger().WarnContext(
					ctx,
					"failed to compute sso shortcut login URL",
					errors.Attr(err),
					slog.Any("authn_provider", authDomain.AuthDomainConfig().AuthNProvider),
					slog.String("domain", authDomain.StorableAuthDomain().Name),
					slog.String("org_id", org.ID.StringValue()),
				)
				continue
			}

			context = context.AddSSODomainContext(
				authtypes.NewSSODomainContext(
					authDomain.StorableAuthDomain().Name,
					authDomain.AuthDomainConfig().AuthNProvider,
					loginURL,
				),
			)
		}
	}

	slices.SortFunc(context.Domains, func(a, b authtypes.SSODomainContext) int {
		if cmp := strings.Compare(a.Domain, b.Domain); cmp != 0 {
			return cmp
		}

		if cmp := strings.Compare(a.Provider.StringValue(), b.Provider.StringValue()); cmp != 0 {
			return cmp
		}

		return strings.Compare(a.URL, b.URL)
	})

	return context, nil
}

func (module *module) CreatePasswordAuthNSession(ctx context.Context, authNProvider authtypes.AuthNProvider, email valuer.Email, password string, orgID valuer.UUID) (*authtypes.Token, error) {
	passwordAuthN, err := getProvider[authn.PasswordAuthN](authNProvider, module.authNs)
	if err != nil {
		return nil, err
	}

	identity, err := passwordAuthN.Authenticate(ctx, email.String(), password, orgID)
	if err != nil {
		return nil, err
	}

	return module.tokenizer.CreateToken(ctx, identity, map[string]string{})
}

func (module *module) CreateCallbackAuthNSession(ctx context.Context, authNProvider authtypes.AuthNProvider, values url.Values) (string, error) {
	callbackAuthN, err := getProvider[authn.CallbackAuthN](authNProvider, module.authNs)
	if err != nil {
		return "", err
	}

	callbackIdentity, err := callbackAuthN.HandleCallback(ctx, values)
	if err != nil {
		module.settings.Logger().ErrorContext(ctx, "failed to handle callback", errors.Attr(err), slog.Any("authn_provider", authNProvider))
		return "", err
	}

	if callbackIdentity.State.URL.Host != "" && !module.globalConfig.IsOriginAllowed(callbackIdentity.State.URL) {
		return "", errors.Newf(errors.TypeForbidden, global.ErrCodeOriginNotAllowed, "state redirect %q is not an allowed origin", callbackIdentity.State.URL.String())
	}

	authDomain, err := module.authDomain.GetByOrgIDAndID(ctx, callbackIdentity.OrgID, callbackIdentity.State.DomainID)
	if err != nil {
		return "", err
	}

	roleMapping := authDomain.AuthDomainConfig().RoleMapping

	roleAttributeExists := false
	if roleMapping != nil && roleMapping.UseRoleAttribute && callbackIdentity.Role != "" {
		_, err := module.authz.GetByOrgIDAndName(ctx, callbackIdentity.OrgID, authtypes.NormalizeRoleName(callbackIdentity.Role))
		if err == nil {
			roleAttributeExists = true
		}
	}

	roleNames := roleMapping.NewRolesFromCallbackIdentity(callbackIdentity, roleAttributeExists)

	var signedInUser *types.User

	allowJITProvisioning := true
	if authDomain.AuthDomainConfig().AuthNProvider == authtypes.AuthNProviderOIDC && authDomain.AuthDomainConfig().OIDC != nil {
		allowJITProvisioning = authDomain.AuthDomainConfig().OIDC.IsAllowJIT()
	}

	if allowJITProvisioning {
		newUser, err := types.NewUser(callbackIdentity.Name, callbackIdentity.Email, callbackIdentity.OrgID, types.UserStatusActive)
		if err != nil {
			return "", err
		}

		signedInUser, err = module.userSetter.GetOrCreateUser(ctx, newUser, user.WithRoleNames(roleNames))
		if err != nil {
			return "", err
		}
	} else {
		signedInUser, err = module.userGetter.GetNonDeletedUserByEmailAndOrgID(ctx, callbackIdentity.Email, callbackIdentity.OrgID)
		if err != nil {
			if errors.Ast(err, errors.TypeNotFound) {
				return "", errors.Newf(errors.TypeForbidden, errors.CodeForbidden, "user %q is not allowed to sign in via SSO", callbackIdentity.Email.StringValue())
			}

			return "", err
		}
	}

	if err := signedInUser.ErrIfRoot(); err != nil {
		return "", errors.WithAdditionalf(err, "root user can only authenticate via password")
	}

	token, err := module.tokenizer.CreateToken(ctx, authtypes.NewPrincipalUserIdentity(signedInUser.ID, signedInUser.OrgID, signedInUser.Email, authtypes.IdentNProviderTokenizer), map[string]string{})
	if err != nil {
		return "", err
	}

	redirectURL := &url.URL{
		Scheme:   callbackIdentity.State.URL.Scheme,
		Host:     callbackIdentity.State.URL.Host,
		Path:     callbackIdentity.State.URL.Path,
		RawQuery: authtypes.NewURLValuesFromToken(token, module.GetRotationInterval(ctx)).Encode(),
	}

	return redirectURL.String(), nil
}

func (module *module) RotateSession(ctx context.Context, accessToken string, refreshToken string) (*authtypes.Token, error) {
	return module.tokenizer.RotateToken(ctx, accessToken, refreshToken)
}

func (module *module) DeleteSession(ctx context.Context, accessToken string) error {
	return module.tokenizer.DeleteToken(ctx, accessToken)
}

func (module *module) GetSessionLogoutContext(ctx context.Context, siteURL *url.URL) (*authtypes.SessionLogoutContext, error) {
	if !module.globalConfig.IsOriginAllowed(siteURL) {
		return nil, errors.Newf(errors.TypeInvalidInput, global.ErrCodeOriginNotAllowed, "ref %q is not an allowed origin", siteURL.String())
	}

	claims, err := authtypes.ClaimsFromContext(ctx)
	if err != nil {
		return nil, err
	}

	email, err := valuer.NewEmail(claims.Email)
	if err != nil {
		return nil, err
	}

	orgID, err := valuer.NewUUID(claims.OrgID)
	if err != nil {
		return nil, err
	}

	// Since email is a valuer, we can safely split to retrieve the domain.
	name := strings.Split(email.String(), "@")[1]
	authDomain, err := module.authDomain.GetByNameAndOrgID(ctx, name, orgID)
	if err != nil {
		if errors.Ast(err, errors.TypeNotFound) {
			return authtypes.NewSessionLogoutContext(""), nil
		}

		return nil, err
	}

	if !authDomain.AuthDomainConfig().SSOEnabled {
		return authtypes.NewSessionLogoutContext(""), nil
	}

	logoutProvider, ok := module.authNs[authDomain.AuthDomainConfig().AuthNProvider].(authn.LogoutURLProvider)
	if !ok {
		return authtypes.NewSessionLogoutContext(""), nil
	}

	logoutURL, err := logoutProvider.LogoutURL(ctx, siteURL, authDomain)
	if err != nil {
		module.settings.Logger().WarnContext(ctx, "failed to compute provider logout URL", errors.Attr(err), slog.Any("authn_provider", authDomain.AuthDomainConfig().AuthNProvider))
		return authtypes.NewSessionLogoutContext(""), nil
	}

	return authtypes.NewSessionLogoutContext(logoutURL), nil
}

func (module *module) GetRotationInterval(context.Context) time.Duration {
	return module.tokenizer.Config().Rotation.Interval
}

func (module *module) getOrgSessionContext(ctx context.Context, org *types.Organization, name string, siteURL *url.URL) (*authtypes.OrgSessionContext, error) {
	authDomain, err := module.authDomain.GetByNameAndOrgID(ctx, name, org.ID)
	if err != nil && !errors.Ast(err, errors.TypeNotFound) {
		return nil, err
	}

	if authDomain == nil {
		return authtypes.NewOrgSessionContext(org.ID, org.Name).AddPasswordAuthNSupport(authtypes.AuthNProviderEmailPassword), nil
	}

	if !authDomain.AuthDomainConfig().SSOEnabled {
		return authtypes.NewOrgSessionContext(org.ID, org.Name).AddPasswordAuthNSupport(authtypes.AuthNProviderEmailPassword), nil
	}

	provider, err := getProvider[authn.CallbackAuthN](authDomain.AuthDomainConfig().AuthNProvider, module.authNs)
	if err != nil {
		return nil, err
	}

	if !module.globalConfig.IsOriginAllowed(siteURL) {
		return nil, errors.Newf(errors.TypeInvalidInput, global.ErrCodeOriginNotAllowed, "ref %q is not an allowed origin", siteURL.String())
	}

	loginURL, err := provider.LoginURL(ctx, siteURL, authDomain)
	if err != nil {
		return nil, err
	}

	return authtypes.NewOrgSessionContext(org.ID, org.Name).AddCallbackAuthNSupport(authDomain.AuthDomainConfig().AuthNProvider, loginURL), nil
}

func getProvider[T authn.AuthN](authNProvider authtypes.AuthNProvider, authNs map[authtypes.AuthNProvider]authn.AuthN) (T, error) {
	var provider T

	provider, ok := authNs[authNProvider].(T)
	if !ok {
		return provider, errors.New(errors.TypeNotFound, errors.CodeNotFound, "authn provider not found")
	}

	return provider, nil
}

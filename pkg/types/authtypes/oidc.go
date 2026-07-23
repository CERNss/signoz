package authtypes

import (
	"encoding/json"
	"strings"

	"github.com/SigNoz/signoz/pkg/errors"
)

const (
	OIDCEmailVerifiedPolicyStrict = "strict"
	OIDCEmailVerifiedPolicyWarn   = "warn"
	OIDCEmailVerifiedPolicyIgnore = "ignore"
)

var defaultOIDCScopes = []string{"openid", "profile", "email"}

type OIDCConfig struct {
	// It is the URL identifier for the service. For example: "https://accounts.google.com" or "https://login.salesforce.com".
	Issuer string `json:"issuer" required:"true"`

	// Some offspec providers like Azure, Oracle IDCS have oidc discovery url different from issuer url which causes issuerValidation to fail
	// This provides a way to override the Issuer url from the .well-known/openid-configuration issuer
	// from the .well-known/openid-configuration issuer
	IssuerAlias string `json:"issuerAlias"`

	// It is the application's ID.
	ClientID string `json:"clientId" required:"true"`

	// It is the application's secret.
	ClientSecret string `json:"clientSecret" required:"true" format:"password"`

	// Mapping of claims to the corresponding fields in the token.
	ClaimMapping AttributeMapping `json:"claimMapping"`

	// Optional OAuth scopes. Defaults to ["openid", "profile", "email"].
	Scopes []string `json:"scopes,omitempty"`

	// Whether to skip email verification. Defaults to "false"
	InsecureSkipEmailVerified bool `json:"insecureSkipEmailVerified"`

	// Uses the userinfo endpoint to get additional claims for the token. This is especially useful where upstreams return "thin" id tokens
	GetUserInfo bool `json:"getUserInfo"`

	// Controls email_verified handling. Allowed values: strict|warn|ignore.
	// If omitted, defaults to:
	// - ignore when insecureSkipEmailVerified is true
	// - warn otherwise
	EmailVerifiedPolicy string `json:"emailVerifiedPolicy"`

	// If true, the email domain from the callback identity must match auth domain name.
	EnforceEmailDomain bool `json:"enforceEmailDomain"`

	// If omitted, defaults to true for backward compatibility.
	AllowJIT *bool `json:"allowJit,omitempty"`
}

func (config *OIDCConfig) UnmarshalJSON(data []byte) error {
	type Alias OIDCConfig

	var temp Alias
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	if temp.Issuer == "" {
		return errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "issuer is required")
	}

	if temp.ClientID == "" {
		return errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "clientId is required")
	}

	if temp.ClientSecret == "" {
		return errors.New(errors.TypeInvalidInput, errors.CodeInvalidInput, "clientSecret is required")
	}

	if temp.ClaimMapping == (AttributeMapping{}) {
		if err := json.Unmarshal([]byte("{}"), &temp.ClaimMapping); err != nil {
			return err
		}
	}

	temp.Scopes = normalizeOIDCScopes(temp.Scopes)

	if temp.EmailVerifiedPolicy == "" {
		if temp.InsecureSkipEmailVerified {
			temp.EmailVerifiedPolicy = OIDCEmailVerifiedPolicyIgnore
		} else {
			temp.EmailVerifiedPolicy = OIDCEmailVerifiedPolicyWarn
		}
	} else {
		temp.EmailVerifiedPolicy = strings.ToLower(strings.TrimSpace(temp.EmailVerifiedPolicy))
		switch temp.EmailVerifiedPolicy {
		case OIDCEmailVerifiedPolicyStrict, OIDCEmailVerifiedPolicyWarn, OIDCEmailVerifiedPolicyIgnore:
		default:
			return errors.Newf(errors.TypeInvalidInput, errors.CodeInvalidInput, "emailVerifiedPolicy must be one of %q, %q, %q", OIDCEmailVerifiedPolicyStrict, OIDCEmailVerifiedPolicyWarn, OIDCEmailVerifiedPolicyIgnore)
		}
	}

	*config = OIDCConfig(temp)
	return nil
}

func (config *OIDCConfig) IsAllowJIT() bool {
	return config.AllowJIT == nil || *config.AllowJIT
}

func (config *OIDCConfig) EffectiveEmailVerifiedPolicy() string {
	if config.EmailVerifiedPolicy != "" {
		return config.EmailVerifiedPolicy
	}

	if config.InsecureSkipEmailVerified {
		return OIDCEmailVerifiedPolicyIgnore
	}

	return OIDCEmailVerifiedPolicyWarn
}

func (config *OIDCConfig) EffectiveScopes() []string {
	return normalizeOIDCScopes(config.Scopes)
}

func normalizeOIDCScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return append([]string(nil), defaultOIDCScopes...)
	}

	normalized := make([]string, 0, len(scopes)+1)
	seen := make(map[string]struct{}, len(scopes)+1)

	add := func(scope string) {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return
		}

		if _, ok := seen[scope]; ok {
			return
		}

		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}

	// OIDC code flow requires openid.
	add("openid")
	for _, scope := range scopes {
		add(scope)
	}

	return normalized
}

package authtypes

import (
	"encoding/json"
	"testing"
)

func TestOIDCConfigDefaults(t *testing.T) {
	var cfg OIDCConfig
	err := json.Unmarshal([]byte(`{
		"issuer": "https://issuer.example.com",
		"clientId": "client-id",
		"clientSecret": "client-secret"
	}`), &cfg)
	if err != nil {
		t.Fatalf("expected config to unmarshal: %v", err)
	}

	if cfg.EffectiveEmailVerifiedPolicy() != OIDCEmailVerifiedPolicyWarn {
		t.Fatalf("expected default email policy warn, got %q", cfg.EffectiveEmailVerifiedPolicy())
	}

	if !cfg.IsAllowJIT() {
		t.Fatal("expected default allowJit=true")
	}

	scopes := cfg.EffectiveScopes()
	expected := []string{"openid", "profile", "email"}
	if len(scopes) != len(expected) {
		t.Fatalf("expected scopes %v, got %v", expected, scopes)
	}

	for i := range expected {
		if scopes[i] != expected[i] {
			t.Fatalf("expected scopes %v, got %v", expected, scopes)
		}
	}
}

func TestOIDCConfigInsecureSkipMapsToIgnorePolicy(t *testing.T) {
	var cfg OIDCConfig
	err := json.Unmarshal([]byte(`{
		"issuer": "https://issuer.example.com",
		"clientId": "client-id",
		"clientSecret": "client-secret",
		"insecureSkipEmailVerified": true
	}`), &cfg)
	if err != nil {
		t.Fatalf("expected config to unmarshal: %v", err)
	}

	if cfg.EffectiveEmailVerifiedPolicy() != OIDCEmailVerifiedPolicyIgnore {
		t.Fatalf("expected email policy ignore, got %q", cfg.EffectiveEmailVerifiedPolicy())
	}
}

func TestOIDCConfigNormalizesScopesAndAllowJIT(t *testing.T) {
	var cfg OIDCConfig
	err := json.Unmarshal([]byte(`{
		"issuer": "https://issuer.example.com",
		"clientId": "client-id",
		"clientSecret": "client-secret",
		"allowJit": false,
		"scopes": ["profile", "email", "custom_scope", "", "profile"]
	}`), &cfg)
	if err != nil {
		t.Fatalf("expected config to unmarshal: %v", err)
	}

	if cfg.IsAllowJIT() {
		t.Fatal("expected allowJit=false")
	}

	scopes := cfg.EffectiveScopes()
	expected := []string{"openid", "profile", "email", "custom_scope"}
	if len(scopes) != len(expected) {
		t.Fatalf("expected scopes %v, got %v", expected, scopes)
	}

	for i := range expected {
		if scopes[i] != expected[i] {
			t.Fatalf("expected scopes %v, got %v", expected, scopes)
		}
	}
}

func TestOIDCConfigRejectsInvalidEmailVerifiedPolicy(t *testing.T) {
	var cfg OIDCConfig
	err := json.Unmarshal([]byte(`{
		"issuer": "https://issuer.example.com",
		"clientId": "client-id",
		"clientSecret": "client-secret",
		"emailVerifiedPolicy": "invalid"
	}`), &cfg)
	if err == nil {
		t.Fatal("expected invalid emailVerifiedPolicy to fail")
	}
}

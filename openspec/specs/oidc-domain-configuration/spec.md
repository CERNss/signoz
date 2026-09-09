# oidc-domain-configuration Specification

## Purpose
Define the OIDC auth-domain configuration contract across backend parsing, OpenAPI schemas, generated frontend types, and the admin create/edit form.

## Requirements
### Requirement: OIDC domain config SHALL expose extended policy fields
The system SHALL expose and persist OIDC config fields `scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, and `allowJit` through API schemas and frontend DTOs.

#### Scenario: Create or update OIDC auth domain with extended fields
- **GIVEN** an auth-domain create or update request carries `config` as a `{kind: "oidc", spec: {...}}` envelope
- **AND** the `spec` includes extended OIDC fields
- **WHEN** the request is validated and stored
- **THEN** the same fields are available in subsequent auth-domain read responses under `config.spec`

#### Scenario: Migrate legacy auth-domain documents without losing extended fields
- **GIVEN** a persisted auth domain written in the legacy `{ssoType, oidcConfig}` shape
- **WHEN** the stored document is migrated to the `{enabled, config: {kind, spec}, roleMapping}` shape
- **THEN** the whole legacy `oidcConfig` object becomes `config.spec`
- **AND** `scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, and `allowJit` are carried over unchanged

#### Scenario: OpenAPI schema includes OIDC policy fields
- **GIVEN** API documentation is generated
- **WHEN** the `AuthtypesOIDCConfig` schema is inspected
- **THEN** it includes `scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, and nullable `allowJit`

#### Scenario: Frontend auth-domain DTOs include OIDC policy fields
- **GIVEN** frontend types are generated or maintained from the auth-domain contract
- **WHEN** create, update, and list DTOs describe OIDC config
- **THEN** each DTO can carry `scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, and `allowJit`

### Requirement: OIDC config SHALL normalize defaults and validate policies
The system SHALL normalize OIDC scopes and validate email verification policy values during config parsing.

#### Scenario: Apply default scopes and JIT behavior
- **GIVEN** OIDC config omits `scopes` and `allowJit`
- **WHEN** config is unmarshaled
- **THEN** effective scopes include `openid`, `profile`, and `email`
- **AND** effective JIT behavior is enabled

#### Scenario: Legacy insecure skip maps to ignore policy
- **GIVEN** OIDC config sets `insecureSkipEmailVerified=true`
- **AND** omits `emailVerifiedPolicy`
- **WHEN** config is unmarshaled
- **THEN** effective email verification policy is `ignore`

#### Scenario: Default email verification policy warns
- **GIVEN** OIDC config omits `emailVerifiedPolicy`
- **AND** `insecureSkipEmailVerified` is false or omitted
- **WHEN** config is unmarshaled
- **THEN** effective email verification policy is `warn`

#### Scenario: Normalize configured scopes
- **GIVEN** OIDC config includes duplicate, empty, or whitespace-padded scopes
- **WHEN** config is unmarshaled
- **THEN** empty scopes are removed
- **AND** duplicate scopes are removed
- **AND** `openid` is included even if omitted by the request

#### Scenario: Reject unsupported email policy value
- **GIVEN** OIDC config sets `emailVerifiedPolicy` to an unsupported value
- **WHEN** config is unmarshaled
- **THEN** parsing fails with invalid input error

### Requirement: OIDC admin UI SHALL serialize scopes and policy fields correctly
The system SHALL map user-entered OIDC form fields into API payload format and map stored values back into editable form state.

#### Scenario: Convert scopes text to API array
- **GIVEN** admin enters comma or whitespace separated scopes text
- **WHEN** the OIDC form is submitted
- **THEN** the `config.spec` payload contains a normalized `scopes` array
- **AND** the form-only `scopesText` field is not sent

#### Scenario: Omit empty scopes payload
- **GIVEN** admin leaves scopes text blank
- **WHEN** the OIDC form is submitted
- **THEN** the payload omits `scopes` so backend defaults apply

#### Scenario: Emit the OIDC config envelope
- **GIVEN** the admin selected the OIDC provider
- **WHEN** the OIDC form is submitted
- **THEN** the request `config` is `{kind: "oidc", spec: <oidc config>}`
- **AND** the spec carries `scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, and `allowJit`

#### Scenario: Load existing OIDC config for editing
- **GIVEN** an existing OIDC auth-domain record whose `config.kind` is `oidc`
- **AND** whose `config.spec` contains `scopes` and nullable `allowJit`
- **WHEN** the edit form is initialized
- **THEN** scopes are shown as text
- **AND** `allowJit` defaults to checked when omitted

#### Scenario: Admin UI exposes policy controls
- **GIVEN** admin opens the OIDC auth-domain form
- **WHEN** the form is rendered
- **THEN** the form exposes controls for scopes, email verified policy, JIT user creation, email-domain enforcement, and UserInfo usage

### Requirement: OIDC admin UI SHALL show copyable integration URLs
The OIDC configuration UI SHALL show copy-ready URLs for provider integration setup.

#### Scenario: Show callback URL for provider redirect URI setup
- **GIVEN** admin opens OIDC configuration page
- **WHEN** the form is rendered
- **THEN** a copyable `OIDC Callback URL` is shown
- **AND** the value is `<current-origin>/api/v1/complete/oidc`

#### Scenario: Show post logout redirect URI for provider logout setup
- **GIVEN** admin opens OIDC configuration page
- **WHEN** the form is rendered
- **THEN** a copyable `Post Logout Redirect URI` is shown
- **AND** the value is `<current-origin>/login`

### Requirement: OIDC session context contracts SHALL be documented
The system SHALL document session context response contracts used by OIDC login shortcuts and logout behavior.

#### Scenario: SSO context schema exposes shortcut domains
- **GIVEN** API documentation is generated
- **WHEN** the `AuthtypesSessionSSOContext` schema is inspected
- **THEN** it contains `domains`
- **AND** each domain entry contains `domain`, `provider`, and `url`

#### Scenario: Logout context schema exposes provider logout URL
- **GIVEN** API documentation is generated
- **WHEN** the `AuthtypesSessionLogoutContext` schema is inspected
- **THEN** it contains a string `url`

#### Scenario: Session context paths are documented
- **GIVEN** API documentation is generated
- **WHEN** session paths are inspected
- **THEN** `/api/v2/sessions/sso_context` is documented as an open login-page context endpoint
- **AND** `/api/v2/sessions/logout_context` is documented as a tokenizer-secured logout context endpoint

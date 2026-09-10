# oidc-callback-authentication Specification

## Purpose
Define OIDC callback sign-in behavior for SigNoz, including provider URL generation, signed callback state validation, token and claim handling, policy-aware session provisioning, login-page SSO shortcuts, and provider logout context.

## Requirements
### Requirement: OIDC callback provider SHALL be registered
The system SHALL register OIDC as a callback authentication provider so OIDC auth domains can generate login URLs, handle provider callbacks, and expose provider logout URLs.

#### Scenario: OIDC auth provider is available for callback login
- **GIVEN** an auth domain uses provider `oidc`
- **WHEN** the auth provider registry is initialized
- **THEN** the OIDC callback provider is available for login URL generation and callback handling

### Requirement: OIDC login URL generation with signed state
The system SHALL generate OIDC authorization URLs for OIDC auth domains that include a signed state value and a callback redirect URI under `/api/v1/complete/oidc`.

#### Scenario: Generate authorization URL for OIDC domain
- **GIVEN** an auth domain configured with OIDC issuer and client credentials
- **WHEN** login URL is requested
- **THEN** the URL targets the provider authorization endpoint
- **AND** the query includes `redirect_uri` ending with `/api/v1/complete/oidc`
- **AND** the `redirect_uri` path is prefixed with the `global::external_url` base path when one is configured
- **AND** the query includes a versioned, signed `state` value
- **AND** the query includes `prompt=select_account`

#### Scenario: Authorization URL uses effective scopes
- **GIVEN** an OIDC auth domain has configured scopes
- **WHEN** login URL is requested
- **THEN** the authorization URL scope query includes the configured scopes
- **AND** the scope query always includes `openid`

#### Scenario: Group role mappings request group scope
- **GIVEN** an OIDC auth domain has role group mappings
- **AND** configured scopes do not include `groups`
- **WHEN** login URL is requested
- **THEN** the authorization URL scope query includes `groups`

#### Scenario: Reject non-OIDC domain for OIDC login URL
- **GIVEN** an auth domain is not configured with provider `oidc`
- **WHEN** OIDC login URL generation is requested
- **THEN** login URL generation fails with auth-domain mismatch

### Requirement: OIDC callback state SHALL be signed and verified
The system SHALL encode callback state with a versioned HMAC signature and verify that signature before exchanging callback code.

#### Scenario: Callback accepts valid signed state
- **GIVEN** a callback request includes a `state` produced by OIDC login URL generation
- **WHEN** callback handling parses and verifies the state
- **THEN** the callback state identifies the original return URL and auth domain

#### Scenario: Callback rejects malformed state
- **GIVEN** a callback request includes a malformed `state`
- **WHEN** callback handling parses the request
- **THEN** callback handling fails with invalid-state error

#### Scenario: Callback rejects tampered state signature
- **GIVEN** a callback request includes a signed `state` whose signature no longer matches the payload
- **WHEN** callback handling verifies the state
- **THEN** callback handling fails with invalid-state error

### Requirement: OIDC callback exchange and claim resolution
The system SHALL exchange callback `code` for tokens, verify `id_token`, and resolve identity claims by merging ID token claims with UserInfo claims when configured or required.

#### Scenario: Resolve identity from ID token
- **GIVEN** callback code exchange succeeds
- **AND** verified `id_token` claims contain a non-empty email claim
- **WHEN** callback is processed
- **THEN** the callback identity uses the email and name claims from `id_token`

#### Scenario: Resolve email using UserInfo fallback
- **GIVEN** callback code exchange succeeds
- **AND** the configured email claim is missing or empty in `id_token`
- **WHEN** UserInfo endpoint returns an email claim
- **THEN** the callback identity uses the email from UserInfo

#### Scenario: Merge UserInfo without overwriting existing claims
- **GIVEN** `getUserInfo` is enabled
- **AND** verified `id_token` and UserInfo both contain claims
- **WHEN** callback is processed
- **THEN** UserInfo fills claims missing from `id_token`
- **AND** non-empty `id_token` claims remain authoritative

#### Scenario: Reject callback without required code
- **WHEN** callback request is missing `code`
- **THEN** callback handling fails with invalid input

#### Scenario: Reject provider error callback
- **GIVEN** callback request includes provider `error`
- **WHEN** callback is processed
- **THEN** callback handling fails with provider error context

#### Scenario: Reject callback without resolvable email
- **GIVEN** neither verified `id_token` claims nor UserInfo claims contain the configured email claim
- **WHEN** callback is processed
- **THEN** callback handling fails with invalid input

#### Scenario: Extract callback groups and role
- **GIVEN** the OIDC auth domain configures group and role claim mappings
- **WHEN** callback claims contain matching group and role values
- **THEN** the callback identity includes those groups and role for downstream role mapping

### Requirement: OIDC trust policy enforcement
The system SHALL enforce configured OIDC trust policies for `email_verified` handling and optional email-domain matching.

#### Scenario: Strict email verification blocks unverified email
- **GIVEN** `emailVerifiedPolicy` is `strict`
- **AND** callback claims contain `email_verified=false`
- **WHEN** callback is processed
- **THEN** authentication is rejected

#### Scenario: Strict email verification requires boolean email_verified claim
- **GIVEN** `emailVerifiedPolicy` is `strict`
- **AND** callback claims do not contain a boolean `email_verified` claim
- **WHEN** callback is processed
- **THEN** authentication is rejected with invalid input

#### Scenario: Warn email verification allows login
- **GIVEN** `emailVerifiedPolicy` is `warn`
- **AND** callback claims contain `email_verified=false`
- **WHEN** callback is processed
- **THEN** authentication is allowed
- **AND** the unverified email is logged as a warning

#### Scenario: Ignore email verification skips claim enforcement
- **GIVEN** `emailVerifiedPolicy` is `ignore`
- **WHEN** callback claims omit `email_verified`
- **THEN** authentication may continue if all other required claims are valid

#### Scenario: Enforce email-domain match
- **GIVEN** `enforceEmailDomain` is enabled
- **AND** callback email domain does not match auth domain name
- **WHEN** callback is processed
- **THEN** authentication is rejected with forbidden error

#### Scenario: Email-domain comparison is case-insensitive
- **GIVEN** `enforceEmailDomain` is enabled
- **AND** callback email domain matches auth domain name with different casing
- **WHEN** callback is processed
- **THEN** authentication is allowed

### Requirement: OIDC session provisioning with optional JIT
The system SHALL support OIDC session creation with configurable JIT provisioning behavior.

#### Scenario: Allow JIT creates user on first login
- **GIVEN** `allowJit` is true or omitted
- **AND** callback identity does not map to an existing user
- **WHEN** session is created from callback identity
- **THEN** a new non-root user is created and session tokens are issued

#### Scenario: Disable JIT requires pre-existing user
- **GIVEN** `allowJit` is false
- **AND** callback identity does not map to an existing user
- **WHEN** session is created from callback identity
- **THEN** session creation is denied with forbidden error

#### Scenario: Disable JIT allows existing user
- **GIVEN** `allowJit` is false
- **AND** callback identity maps to an existing non-deleted user in the callback organization
- **WHEN** session is created from callback identity
- **THEN** session tokens are issued for the existing user

#### Scenario: Root users cannot sign in through OIDC callback
- **GIVEN** callback identity maps to a root user
- **WHEN** session is created from callback identity
- **THEN** session creation is rejected because root users authenticate with password only

### Requirement: OIDC callback endpoint redirect behavior
The session callback endpoint SHALL redirect to provider return URL on success and to login URL with callback-authn error context on failure.

#### Scenario: Successful callback redirect
- **GIVEN** callback session creation succeeds
- **WHEN** `/api/v1/complete/oidc` is requested
- **THEN** response is HTTP 303 redirect to the returned provider URL

#### Scenario: Failed callback redirect
- **GIVEN** callback session creation fails
- **WHEN** `/api/v1/complete/oidc` is requested
- **THEN** response is HTTP 303 redirect to `/login` with callback error query parameters

### Requirement: Login page SSO context SHALL expose enabled callback domains
The system SHALL expose SSO shortcut context for the login page through `/api/v2/sessions/sso_context`.

#### Scenario: Return SSO shortcut for enabled OIDC domain
- **GIVEN** an organization has an enabled OIDC auth domain
- **WHEN** `/api/v2/sessions/sso_context` is requested with a valid `ref` URL
- **THEN** the response includes the auth domain name
- **AND** the response provider is `oidc`
- **AND** the response URL is an OIDC login URL for the supplied `ref`

#### Scenario: Skip disabled or unsupported callback domains
- **GIVEN** an auth domain is not SSO-enabled or its callback provider cannot generate a login URL
- **WHEN** SSO context is generated
- **THEN** that auth domain is omitted from the SSO shortcut response

#### Scenario: SSO shortcuts are stable sorted
- **GIVEN** multiple SSO-enabled callback domains exist
- **WHEN** SSO context is generated
- **THEN** shortcut domains are sorted by domain, provider, and URL

#### Scenario: Login page redirects through SSO shortcut
- **GIVEN** the login page receives SSO shortcut context with an OIDC URL
- **WHEN** the user clicks the shortcut for that domain
- **THEN** the browser navigates to the provided OIDC login URL

### Requirement: OIDC logout context generation with provider end-session metadata
The system SHALL expose a session logout context for OIDC users by deriving provider end-session URL and adding post-logout redirect parameters.

#### Scenario: Build OIDC provider logout URL
- **GIVEN** the current authenticated user belongs to an OIDC-enabled auth domain
- **AND** provider metadata includes `end_session_endpoint`
- **WHEN** session logout context is requested
- **THEN** response includes a non-empty logout URL targeting provider `end_session_endpoint`
- **AND** the URL query includes `post_logout_redirect_uri` pointing to `/login` on the current SigNoz origin
- **AND** that `post_logout_redirect_uri` path is prefixed with the `global::external_url` base path when one is configured
- **AND** the URL query includes `client_id` from OIDC domain configuration

#### Scenario: Fallback when provider logout is unavailable
- **GIVEN** the current authenticated user does not map to an OIDC logout URL (for example, metadata has no `end_session_endpoint`)
- **WHEN** session logout context is requested
- **THEN** response includes an empty logout URL so clients can fallback to local logout behavior

#### Scenario: Logout context endpoint requires authenticated session
- **GIVEN** a user has a valid tokenizer session
- **WHEN** `/api/v2/sessions/logout_context` is requested with a valid `ref` URL
- **THEN** the response returns the provider logout URL when available

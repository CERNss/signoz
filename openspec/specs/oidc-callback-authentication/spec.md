# oidc-callback-authentication Specification

## Purpose
TBD - created by archiving change oidc-support-customized-action-runner. Update Purpose after archive.
## Requirements
### Requirement: OIDC login URL generation with signed state
The system SHALL generate OIDC authorization URLs for OIDC auth domains that include a signed state value and a callback redirect URI under `/api/v1/complete/oidc`.

#### Scenario: Generate authorization URL for OIDC domain
- **GIVEN** an auth domain configured with OIDC issuer and client credentials
- **WHEN** login URL is requested
- **THEN** the URL targets the provider authorization endpoint
- **AND** the query includes `redirect_uri` ending with `/api/v1/complete/oidc`
- **AND** the query includes a versioned, signed `state` value

### Requirement: OIDC callback exchange and claim resolution
The system SHALL exchange callback `code` for tokens, verify `id_token`, and resolve identity claims by merging ID token claims with UserInfo claims when configured or required.

#### Scenario: Resolve email using UserInfo fallback
- **GIVEN** callback code exchange succeeds
- **AND** the configured email claim is missing or empty in `id_token`
- **WHEN** UserInfo endpoint returns an email claim
- **THEN** the callback identity uses the email from UserInfo

#### Scenario: Reject callback without required code
- **WHEN** callback request is missing `code`
- **THEN** callback handling fails with invalid input

### Requirement: OIDC trust policy enforcement
The system SHALL enforce configured OIDC trust policies for `email_verified` handling and optional email-domain matching.

#### Scenario: Strict email verification blocks unverified email
- **GIVEN** `emailVerifiedPolicy` is `strict`
- **AND** callback claims contain `email_verified=false`
- **WHEN** callback is processed
- **THEN** authentication is rejected

#### Scenario: Enforce email-domain match
- **GIVEN** `enforceEmailDomain` is enabled
- **AND** callback email domain does not match auth domain name
- **WHEN** callback is processed
- **THEN** authentication is rejected with forbidden error

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


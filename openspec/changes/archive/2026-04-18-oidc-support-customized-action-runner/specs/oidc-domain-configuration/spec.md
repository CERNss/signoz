## ADDED Requirements

### Requirement: OIDC domain config SHALL expose extended policy fields
The system SHALL expose and persist OIDC config fields `scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, and `allowJit` through API schemas and frontend DTOs.

#### Scenario: Create or update OIDC auth domain with extended fields
- **GIVEN** an auth-domain create or update request includes extended OIDC fields
- **WHEN** the request is validated and stored
- **THEN** the same fields are available in subsequent auth-domain read responses

### Requirement: OIDC config SHALL normalize defaults and validate policies
The system SHALL normalize OIDC scopes and validate email verification policy values during config parsing.

#### Scenario: Apply default scopes and JIT behavior
- **GIVEN** OIDC config omits `scopes` and `allowJit`
- **WHEN** config is unmarshaled
- **THEN** effective scopes include `openid`, `profile`, and `email`
- **AND** effective JIT behavior is enabled

#### Scenario: Reject unsupported email policy value
- **GIVEN** OIDC config sets `emailVerifiedPolicy` to an unsupported value
- **WHEN** config is unmarshaled
- **THEN** parsing fails with invalid input error

### Requirement: OIDC admin UI SHALL serialize scopes and policy fields correctly
The system SHALL map user-entered OIDC form fields into API payload format and map stored values back into editable form state.

#### Scenario: Convert scopes text to API array
- **GIVEN** admin enters comma or whitespace separated scopes text
- **WHEN** the OIDC form is submitted
- **THEN** payload contains normalized `scopes` array

#### Scenario: Load existing OIDC config for editing
- **GIVEN** an existing OIDC auth-domain record contains `scopes` and nullable `allowJit`
- **WHEN** the edit form is initialized
- **THEN** scopes are shown as text
- **AND** `allowJit` defaults to checked when omitted

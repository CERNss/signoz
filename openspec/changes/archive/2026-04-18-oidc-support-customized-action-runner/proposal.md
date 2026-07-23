## Why

The community build pipeline and OIDC sign-in path needed stronger safety controls before wider rollout. We needed one change that both hardened authentication behavior and made customized action runner based image builds reproducible.

## What Changes

- Add an OIDC callback authentication provider with signed state, token exchange, claims resolution, and configurable trust checks.
- Extend OIDC domain configuration with `scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, and `allowJit`.
- Update session creation behavior so OIDC can disable JIT provisioning and only allow pre-existing users.
- Add reusable community quality workflow and Docker Hub autobuild workflow with runner selection and quality gate dependency.
- Pin community Docker image base to digest (`alpine@sha256`) via build argument resolution in workflow.
- Add backend and handler tests for OIDC callback behavior, policy enforcement, and redirect handling.

## Capabilities

### New Capabilities
- `oidc-callback-authentication`: Perform secure OIDC callback login with state integrity, claim fallback logic, and policy-aware identity/session creation.
- `oidc-domain-configuration`: Expose and validate extended OIDC config fields consistently across API schema, backend parsing, and frontend forms.
- `community-autobuild-action-runner`: Build and publish community images through a gated, reusable quality workflow with configurable runner target.

### Modified Capabilities
- None.

## Impact

- Backend auth modules: `pkg/authn/callbackauthn/oidccallbackauthn`, `pkg/modules/session/implsession`, `pkg/signoz/authn.go`
- Auth types/API contracts: `pkg/types/authtypes/oidc.go`, `docs/api/openapi.yml`, generated frontend schema/types
- Frontend auth-domain management UI: OIDC create/edit provider views and conversion utilities
- CI/CD and container build: `.github/workflows/_autobuild-community-quality.yaml`, `.github/workflows/autobuild-community-dockerhub.yaml`, `cmd/community/Dockerfile`
- Dependencies/tests: `go.mod`, OIDC config and callback/session tests

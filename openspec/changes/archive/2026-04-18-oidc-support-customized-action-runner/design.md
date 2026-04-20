## Context

This change combines two cross-cutting concerns shipped together in the same code update:
1) OIDC authentication hardening and configurability across backend, API contracts, and frontend auth-domain management.
2) Community Docker Hub autobuild reliability via reusable quality gates and runner-selectable workflows.

The existing system supported callback-based auth providers but did not include first-class OIDC callback handling with configurable trust policies and JIT controls. The existing community autobuild path also needed a standardized quality gate and deterministic image-base pinning.

## Goals / Non-Goals

**Goals:**
- Introduce first-class OIDC callback authentication provider registration.
- Enforce callback-state integrity and configurable claim trust policies.
- Add admin-configurable OIDC controls (`scopes`, `emailVerifiedPolicy`, `enforceEmailDomain`, `allowJit`) with consistent backend/frontend API contracts.
- Gate community image publishing behind reusable quality workflow.
- Make community image base deterministic using digest-pinned Alpine.

**Non-Goals:**
- Adding new identity providers beyond OIDC.
- Reworking existing Google or SAML provider behavior.
- Replacing the entire CI strategy across all workflows.
- Redesigning user role mapping semantics outside current claim-mapping surface.

## Decisions

### 1) Implement OIDC as dedicated callback auth provider
- Decision: register `AuthNProviderOIDC` via `pkg/authn/callbackauthn/oidccallbackauthn`.
- Rationale: keeps provider-specific behavior isolated and consistent with callback authn abstraction.
- Alternatives considered:
  - Extend Google callback provider generically: rejected because provider-specific semantics (claims, policies, state signing) would become harder to reason about.
  - Handle OIDC logic directly in session module: rejected due to poor separation of concerns.

### 2) Use versioned HMAC-signed callback state
- Decision: sign encoded state payload using HMAC-SHA256 and provider secret.
- Rationale: protects callback state integrity and supports forward-compatible versioning (`v1` prefix).
- Alternatives considered:
  - Plain state payload only: rejected due to tampering risk.
  - Server-side ephemeral state storage: not selected for this change to avoid additional state management complexity.

### 3) Use layered claim resolution strategy
- Decision: prefer verified ID token claims and supplement with UserInfo when configured or when email is missing.
- Rationale: supports providers with thin ID tokens while minimizing unnecessary UserInfo calls.
- Alternatives considered:
  - Always require UserInfo: rejected due to extra latency/dependency.
  - ID token only: rejected because some providers omit required claims in token.

### 4) Make identity trust controls explicit and configurable
- Decision: add `emailVerifiedPolicy`, `enforceEmailDomain`, and `allowJit` with backward-compatible defaults.
- Rationale: gives operators explicit policy control while preserving existing behavior for current domains.
- Alternatives considered:
  - Keep `insecureSkipEmailVerified` as sole control: rejected because it is coarse and ambiguous.
  - Disable JIT by default: rejected to avoid breaking existing onboarding flows.

### 5) Reuse quality workflow before DockerHub publish
- Decision: introduce `_autobuild-community-quality.yaml` and call it from publish workflow with `needs`.
- Rationale: centralizes checks, avoids duplication, and blocks unsafe publishes.
- Alternatives considered:
  - Duplicate quality jobs inline in publish workflow: rejected due to maintenance overhead.

### 6) Pin runtime base image by digest
- Decision: change community Dockerfile to `FROM alpine@sha256:${ALPINE_SHA}` and compute digest in workflow.
- Rationale: deterministic base image and reduced drift/supply-chain ambiguity.
- Alternatives considered:
  - Keep tag-based base (`alpine:3.20.3`): rejected due to mutable tag risk.

## Risks / Trade-offs

- [State signing secret coupling] Rotating OIDC client secret invalidates in-flight state values.
  → Mitigation: short callback windows and clear retry semantics via login restart.
- [Strict policy lockout] `emailVerifiedPolicy=strict` or `enforceEmailDomain=true` can block valid users if provider claims are inconsistent.
  → Mitigation: defaults remain non-breaking (`warn`, no domain enforcement) and policy is explicit.
- [UserInfo dependency] Fallback to UserInfo may add network latency or fail on provider outages.
  → Mitigation: only call UserInfo when requested or required by missing email.
- [Runner availability] macOS self-hosted runner path can stall if runner is unavailable.
  → Mitigation: explicit linux fallback via dispatch input or repository variable.

## Migration Plan

1. Deploy backend OIDC provider registration and session behavior updates.
2. Deploy API schema/frontend updates for new OIDC fields.
3. Configure auth domains incrementally:
   - keep defaults (`allowJit=true`, `emailVerifiedPolicy=warn`) for compatibility
   - enable stricter policies per tenant after validation.
4. Enable community autobuild workflow and set `BUILD_RUNNER` as needed.
5. Verify DockerHub publish tags and digest-pinned base behavior.
6. Rollback strategy:
   - revert workflow files and Dockerfile digest mode if publish path regresses
   - revert OIDC provider registration/config parsing changes if callback authn regresses.

## Open Questions

- Should state signing move to a dedicated rotatable signing secret separate from OIDC client secret?
- Should future role/group mapping add typed validation for non-string claim formats beyond current best-effort extraction?

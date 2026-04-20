## 1. OIDC Callback Authentication Backend

- [x] 1.1 Implement OIDC callback auth provider package with login URL generation, callback token exchange, and claim extraction.
- [x] 1.2 Add signed state generation/parsing/signature verification and enforce callback state integrity.
- [x] 1.3 Register OIDC callback provider in authn factory wiring.
- [x] 1.4 Add callback/session handler tests for success and error redirect behavior.

## 2. OIDC Policy and Session Controls

- [x] 2.1 Extend `OIDCConfig` with scopes normalization and policy fields (`emailVerifiedPolicy`, `enforceEmailDomain`, `allowJit`).
- [x] 2.2 Enforce email verification policy and optional email-domain matching in callback flow.
- [x] 2.3 Update callback session creation path to support `allowJit=false` with existing-user-only sign-in.
- [x] 2.4 Add unit tests for OIDC config defaults, validation, and callback policy behavior.

## 3. API Contract and Frontend OIDC UX

- [x] 3.1 Update OpenAPI schema and generated frontend DTOs to include new OIDC config fields.
- [x] 3.2 Update auth-domain create/edit form utilities to map scopes text input to array payload and back.
- [x] 3.3 Add OIDC provider form controls for scopes, email policy, JIT toggle, and email-domain enforcement.
- [x] 3.4 Ensure create/update/list frontend domain types remain aligned with backend contract.

## 4. Community Autobuild Runner and Image Publishing

- [x] 4.1 Add reusable community quality workflow for Go/JS/tests/openapi/integration checks.
- [x] 4.2 Add DockerHub autobuild workflow with quality dependency and runner selection priority.
- [x] 4.3 Implement deterministic image tag generation for branch, main, and release-tag workflows.
- [x] 4.4 Switch community Dockerfile to digest-pinned Alpine base passed via workflow build args.

## 5. Validation and Completion

- [x] 5.1 Validate OpenSpec change artifacts for proposal, specs, design, and tasks completeness.
- [x] 5.2 Archive the completed change to merge delta specs into `openspec/specs/`.

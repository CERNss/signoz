# community-autobuild-action-runner Specification

## Purpose
TBD - created by archiving change oidc-support-customized-action-runner. Update Purpose after archive.
## Requirements
### Requirement: Community autobuild workflow SHALL run gated quality checks
The system SHALL run a reusable quality workflow before any community Docker Hub build and push job starts.

#### Scenario: Quality gates block build when checks fail
- **GIVEN** community autobuild workflow is triggered
- **WHEN** any reusable quality job fails
- **THEN** image build-and-push jobs do not execute

### Requirement: Community autobuild workflow SHALL support runner selection
The system SHALL select build runner target in priority order: dispatch input `runner`, repository variable `BUILD_RUNNER`, then default `linux`.

#### Scenario: Workflow dispatch overrides default runner
- **GIVEN** workflow is started with `runner=macos`
- **WHEN** jobs are evaluated
- **THEN** macOS build job runs
- **AND** linux build job is skipped

### Requirement: Community image build SHALL publish deterministic tags
The system SHALL compute image name and publish tags using branch/tag context and commit SHA.

#### Scenario: Main branch publishes latest tag
- **GIVEN** workflow runs on `refs/heads/main`
- **WHEN** metadata is computed
- **THEN** output tags include `<image>:latest`
- **AND** output tags include branch-safe and short-sha tags

#### Scenario: Git tag publish includes release tag
- **GIVEN** workflow runs for `refs/tags/vX.Y.Z`
- **WHEN** metadata is computed
- **THEN** output tags include `<image>:vX.Y.Z`

### Requirement: Community Dockerfile SHALL use digest-pinned base image
The system SHALL build community image from `alpine@sha256:<digest>` where digest is passed as build arg by workflow.

#### Scenario: Build passes resolved alpine digest
- **GIVEN** workflow resolves Alpine digest via `docker buildx imagetools inspect`
- **WHEN** build step runs
- **THEN** `ALPINE_SHA` build arg is passed to Dockerfile
- **AND** resulting base image reference is digest-pinned


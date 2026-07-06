# Comet Design Handoff

- Change: github-app-integration
- Phase: design
- Mode: compact
- Context hash: 510f5c835a041f2138ebe41bb7915abbfb57fc03009dfe58593f067c9676df9b

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/github-app-integration/proposal.md

- Source: openspec/changes/github-app-integration/proposal.md
- Lines: 1-29
- SHA256: cf545e6b456926b1217307103a0de57c120324197ee7610593020eaa1c3ac22e

```md
## Why

AgentGuild currently relies on a single global GitHub App configured via environment variables for all git operations. In a multi-tenant deployment each tenant needs its own GitHub App configuration, and operators need a way to manage it without restarting the service. Additionally, local development and testing are hampered by the hard requirement for OIDC; a local fallback login lets developers run the console without an identity provider.

## What Changes

- Add per-tenant GitHub App configuration storage (`github_apps` table, migration `000009`).
- Add `GitHubAppService` / `GitHubAppManager` application layer with Upsert/Get/Delete and per-tenant `git.Driver` resolution.
- Wire `CredentialService` and `CommitVerifier` to resolve the driver through `GitHubAppService` instead of a global driver.
- Add REST endpoints under `/v1/github-apps` for administrators to manage their tenant's GitHub App.
- Add `POST /v1/auth/local-login` as a development-only fallback that creates a session cookie when OIDC is not configured.
- Update configuration loader to support `LOCAL_ADMIN_*` settings.

## Capabilities

### New Capabilities

- `github-app-integration`: Per-tenant GitHub App configuration and driver resolution for git operations.
- `local-login`: Development-only password-based session login fallback when OIDC is unavailable.

### Modified Capabilities

- `agent-access-control`: Add `local-login` as an alternative human authentication path for development.

## Impact

- Backend: new migration, new `internal/git/application/github_app.go`, new repository, new REST handlers, changes to `CredentialService`/`CommitVerifier` signatures.
- Configuration: new optional `LocalAdmin` config section.
- Tests: existing git tests updated to the new service signatures; new endpoint tests to be added.
```

## openspec/changes/github-app-integration/design.md

- Source: openspec/changes/github-app-integration/design.md
- Lines: 1-61
- SHA256: 9575ef2966ff1e7da5714262cfe31eb2ad33e9c231cdd3d07fc9d5d7efbfb1f4

```md
## Context

The platform supports git-based delivery and validation for Agent submissions. Until now the git driver was constructed once from a single set of `GITHUB_APP_ID` / `GITHUB_PRIVATE_KEY` / `GITHUB_INSTALLATION_ID` environment variables. This works for single-tenant deployments but prevents multiple tenants from using separate GitHub Apps. At the same time, developers have no easy way to run the Web console locally without configuring a real OIDC provider.

## Goals / Non-Goals

**Goals:**
- Store per-tenant GitHub App configuration securely (private key persisted, never exposed in API responses).
- Resolve the correct `git.Driver` per tenant when issuing credentials or verifying commits.
- Provide admin REST endpoints to Upsert/Get/Delete the tenant GitHub App config.
- Provide a local admin login endpoint for development environments that bypasses OIDC.
- Keep the change backward compatible where no tenant config exists (return `not_configured` errors clearly).

**Non-Goals:**
- Frontend UI for GitHub App management (console-only or API-only for now).
- Automatic GitHub App creation or marketplace flow.
- Production use of local login; it is gated behind explicit config and documented as dev-only.
- Support for non-GitHub providers in this change.

## Decisions

1. **GitHub App configuration lives in PostgreSQL, not environment variables.**
   - Rationale: Multi-tenancy requires isolation; env vars cannot be updated at runtime.
   - Alternative considered: keep env vars as global fallback. Rejected to enforce explicit per-tenant setup.

2. **`GitHubAppService` is injected into `CredentialService` and `CommitVerifier`.**
   - Rationale: Both operations need a tenant-scoped driver. Dependency injection keeps the services testable.
   - Alternative considered: pass driver factory at call site. Rejected because it leaks driver construction into handlers.

3. **Private key is write-only from the API.**
   - Rationale: Security; the private key must never be returned in `GET` responses.
   - `GitHubAppView` omits `PrivateKey` and exposes a `configured` flag.

4. **Local login is disabled unless `LOCAL_ADMIN_ENABLED=true`.**
   - Rationale: Prevents accidental exposure in production. The endpoint is a dev escape hatch.
   - Password is compared with `subtle.ConstantTimeCompare` to resist timing attacks.

5. **Use existing `github.NewDriver` for resolved configurations.**
   - Rationale: Reuses the existing GitHub driver implementation; only configuration sourcing changes.

## Risks / Trade-offs

- [Risk] Existing tests and callers had to be updated to the new `CredentialService`/`CommitVerifier` signatures.
  - Mitigation: Tests were updated as part of this change; all backend tests pass.
- [Risk] Private key stored in PostgreSQL increases attack surface.
  - Mitigation: Stored encrypted-at-rest by PostgreSQL; API never returns it; access limited to admin principals.
- [Risk] Local login could be misused in production.
  - Mitigation: Disabled by default; requires explicit env vars; documented as dev-only.
- [Risk] Tenants without GitHub App config see `not_configured` errors.
  - Mitigation: Clear error code and admin endpoint to set up configuration.

## Migration Plan

1. Apply migration `000009_github_apps.up.sql` to add the `github_apps` table.
2. Existing env-based GitHub App config is no longer used by `CredentialService`/`CommitVerifier`; operators must configure it per tenant via the new API.
3. Rollback: apply `000009_github_apps.down.sql`.

## Open Questions

- Should the global env-based GitHub App config remain as a fallback for backward compatibility?
- Should local login be restricted to non-production builds?
```

## openspec/changes/github-app-integration/tasks.md

- Source: openspec/changes/github-app-integration/tasks.md
- Lines: 1-42
- SHA256: 3675528ac6424e1a302371fe588a5793ca808e369b63ca7ceb7bb9238fa6a890

```md
## 1. Database and Configuration

- [x] 1.1 Add `github_apps` table migration (`000009_github_apps.up.sql` / `.down.sql`)
- [x] 1.2 Add `LocalAdmin` configuration fields to `config.Config`
- [x] 1.3 Register migration in test database helper

## 2. Application Layer

- [x] 2.1 Define `GitHubAppRecord`, `GitHubAppView`, `UpsertGitHubApp`, and `GitHubAppService`/`GitHubAppManager` interfaces
- [x] 2.2 Implement `gitHubAppService` with Upsert/Get/Delete and `Driver` resolution
- [x] 2.3 Implement `GitHubAppRepository` in `internal/git/postgres`
- [x] 2.4 Update `CredentialService` to accept `GitHubAppService` and resolve driver per tenant
- [x] 2.5 Update `CommitVerifier` to accept `GitHubAppService` and resolve driver per tenant
- [x] 2.6 Add `GitHubApps()` to `application.Tx`
- [x] 2.7 Add `ErrGitHubAppNotConfigured` error

## 3. REST Layer

- [x] 3.1 Add `github_app_router.go` with GET/PUT/DELETE `/v1/github-apps`
- [x] 3.2 Add `local_login.go` with `POST /v1/auth/local-login`
- [x] 3.3 Wire GitHub App manager and local login into `Server` and `router.go`
- [x] 3.4 Update `main.go` to construct and inject the new services

## 4. Testing

- [x] 4.1 Update existing git application tests to new `CredentialService`/`CommitVerifier` signatures
- [x] 4.2 Update existing git postgres tests to new signatures
- [x] 4.3 Update worker and contract tests for new `Tx.GitHubApps()` method
- [x] 4.4 Adjust config test for new local admin / OIDC behavior
- [ ] 4.5 Add unit tests for `gitHubAppService` Upsert/Get/Delete and driver resolution
- [ ] 4.6 Add REST tests for `/v1/github-apps` endpoints
- [ ] 4.7 Add REST tests for `/v1/auth/local-login`
- [ ] 4.8 Run full backend test suite (`go test -race ./...`)

## 5. Documentation and Comet

- [x] 5.1 Write `proposal.md` for the change
- [x] 5.2 Write `design.md` for the change
- [x] 5.3 Write delta specs for `github-app-integration` and `local-login`
- [ ] 5.4 Update `AGENTS.md` with new environment variables and endpoints
- [ ] 5.5 Update `skill.md` if Agent-facing behavior changed
- [ ] 5.6 Run Comet open/build/verify/archive flow
```


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

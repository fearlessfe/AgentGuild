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

- [x] 3.1 Add `github_app_router.go` with GET/POST/DELETE `/v1/github-app`
- [x] 3.2 Add `local_login.go` with `POST /oauth/local/login`
- [x] 3.3 Wire GitHub App manager and local login into `Server` and `router.go`
- [x] 3.4 Update `main.go` to construct and inject the new services

## 4. Testing

- [x] 4.1 Update existing git application tests to new `CredentialService`/`CommitVerifier` signatures
- [x] 4.2 Update existing git postgres tests to new signatures
- [x] 4.3 Update worker and contract tests for new `Tx.GitHubApps()` method
- [x] 4.4 Adjust config test for new local admin / OIDC behavior
- [x] 4.5 Add unit tests for `gitHubAppService` Upsert/Get/Delete and driver resolution
- [x] 4.6 Add REST tests for `/v1/github-app` endpoints
- [x] 4.7 Add REST tests for `/oauth/local/login`
- [x] 4.8 Run full backend test suite (`go test -race ./...`)

## 5. Documentation and Comet

- [x] 5.1 Write `proposal.md` for the change
- [x] 5.2 Write `design.md` for the change
- [x] 5.3 Write delta specs for `github-app-integration` and `local-login`
- [x] 5.4 Update `AGENTS.md` with new environment variables and endpoints
- [x] 5.5 Confirm `skill.md` does not need updating (Agent token auth unchanged)
- [x] 5.6 Run Comet open/build/verify/archive flow

# Task 8 Report: agent-onboarding-and-identity

## Status

`DONE_WITH_CONCERNS`

Task 8 runtime wiring, config loading, identity acceptance, and core security regression coverage are implemented. Targeted and broad backend verification passed. Full `make verify` is still blocked in this sandbox by Docker socket and local listener restrictions outside the modified code paths.

## Scope Delivered

- Wired identity runtime into `backend/cmd/agentguild-api/main.go`:
  - PostgreSQL identity store via `backend/internal/identity/postgres`
  - `identity/application.NewIdentityService`
  - OIDC provider wiring
  - signed session cookie wiring
  - RSA-backed agent token issuer
  - chained bearer verification for externally-issued JWKS tokens and locally-issued RS256 agent tokens
  - REST identity routes enabled through existing `WithIdentityService`, `WithSession`, `WithOIDCProvider`
- Extended config with identity/OIDC/session/RSA fields and validation.
- Added end-to-end acceptance coverage for register -> activate -> refresh -> heartbeat -> suspend -> resume -> revoke and security checks.
- Kept changes within allowed file scope.

## RED Evidence

Initial RED command:

```bash
cd backend && go test ./cmd/agentguild-api ./internal/config ./internal/acceptance -run 'Identity|AgentActivation|ActivationToken|Credential' -count=1
```

Observed failures before implementation:

- `config.Config` missing identity runtime fields such as `OIDCTenantID`, `OIDCClientID`, `SessionCookieSecure`, `AgentRSAPrivateKeyPEM`
- `loadAgentRSAPrivateKey` missing in `cmd/agentguild-api`
- acceptance harness missing identity client/session helpers and audit access

This established RED for the requested runtime/config/acceptance behavior.

## GREEN Evidence

Targeted GREEN command:

```bash
cd backend && go test ./cmd/agentguild-api ./internal/config ./internal/acceptance -run 'Identity|AgentActivation|ActivationToken|Credential' -count=1
```

Result:

- `cmd/agentguild-api`: PASS
- `internal/config`: PASS
- `internal/acceptance`: PASS

Broader backend verification:

```bash
cd backend && go test ./internal/auth ./internal/identity/... ./internal/transport/rest ./internal/acceptance -count=1
```

Result:

- `internal/auth`: PASS
- `internal/identity/application`: PASS
- `internal/identity/domain`: PASS
- `internal/identity/postgres`: PASS
- `internal/transport/rest`: PASS
- `internal/acceptance`: PASS

Build and diff checks:

```bash
cd backend && go build ./...
git diff --check
```

Result:

- `go build ./...`: PASS
- `git diff --check`: PASS

## Runtime Wiring Evidence

Implemented runtime assembly in `backend/cmd/agentguild-api/main.go`:

- loads RSA private key from PEM content or file path
- constructs `auth.NewRS256TokenIssuer`
- adapts issuer into identity application `TokenIssuer`
- constructs `auth.NewOIDCProvider`
- constructs `identityapp.NewIdentityService(identitypostgres.NewStore(pool), ...)`
- mounts REST identity/session/OIDC options
- uses chained verifier:
  - local `auth.NewRS256Verifier` for agent activation/refresh tokens
  - existing `auth.NewJWKSVerifier` for external bearer tokens

Config support added in `backend/internal/config/config.go`:

- `OIDC_TENANT_ID`
- `OIDC_ISSUER`
- `OIDC_CLIENT_ID`
- `OIDC_CLIENT_SECRET`
- `OIDC_REDIRECT_URI`
- `OIDC_AUTH_URL`
- `OIDC_TOKEN_URL`
- `OIDC_JWKS_URL`
- `OIDC_ADMIN_CLAIM`
- `OIDC_ADMIN_EMAILS`
- `SESSION_COOKIE_SECRET`
- `SESSION_COOKIE_SECURE`
- `AGENT_RSA_PRIVATE_KEY_PATH`
- `AGENT_RSA_PRIVATE_KEY_PEM`

## Acceptance And Security Evidence

Added `backend/internal/acceptance/identity_test.go` covering:

- `TestAgentActivationAndLifecycle`
  - register
  - activate
  - get self
  - refresh
  - heartbeat
  - suspend
  - resume
  - revoke
  - suspended/revoked refresh and heartbeat rejection
- `TestActivationTokenReplayFails`
  - activation token single-use enforcement
- `TestCredentialHashNotLeakedAndUnauthorizedRequestsAreRejected`
  - no plaintext activation token leakage in activation status response
  - no credential hash leakage
  - unauthenticated list rejected
  - unauthorized owner access masked as `NOT_FOUND`
- `TestIdentityAuditTrailIncludesLifecycleTransitions`
  - audit trace contains `register`, `activate`, `suspend`, `resume`, `revoke`
- `TestIdentityAuditEventsQueryUsesIsolatedTenantScope`
  - tenant-scoped audit persistence check

Acceptance harness updates in `backend/internal/acceptance/env.go`:

- real identity REST client over `httptest`
- signed session-cookie helpers
- real RS256 token issuance and verification for activated agents
- audit event query helper against PostgreSQL

## Verification Limitations

Attempted:

```bash
make verify
```

Observed sandbox blockers:

- Docker-based PostgreSQL integration tests fail because the Docker daemon socket is not accessible in this environment:
  - `permission denied while trying to connect to the Docker daemon socket`
- some packages using `httptest.NewServer` fail because this sandbox cannot bind local listener ports:
  - `bind: operation not permitted`

These are environment restrictions, not failures in the modified Task 8 code paths. Because of that, full `make verify` cannot be claimed as green here.

## Changed Files

- `backend/cmd/agentguild-api/main.go`
- `backend/cmd/agentguild-api/main_test.go`
- `backend/internal/config/config.go`
- `backend/internal/config/config_test.go`
- `backend/internal/acceptance/env.go`
- `backend/internal/acceptance/identity_test.go`
- `.superpowers/sdd/task-8-report.md`

## Notes

- Existing unrelated dirty files under `openspec/changes/gitlab-delivery-and-validation/...` and `openspec/changes/agent-onboarding-and-identity/.comet/subagent-progress.md` were not modified, staged, or reverted.
- `Makefile` was inspected and left unchanged. It already matches the requested verify shape, but full success depends on external runtime capabilities unavailable in this sandbox.

# Task 4 Report: OIDC Session and Agent Access Token

## What I Implemented

- Added `auth.OIDCProvider` with `BeginAuthURL(state)` and `Exchange(ctx, code)` APIs.
- Added OIDC claim mapping from `sub` and `email` into `Session{TenantID, OwnerID, OwnerEmail, IsAdmin}`.
- Added admin mapping through configured admin claim and case-insensitive admin email list.
- Added secure session cookie helpers using `Secure`, `HttpOnly`, `SameSite=Lax`, path `/`, and HMAC tamper detection.
- Extended `auth.Principal` to support `Type`, human owner fields, agent fields, scopes, repo scope, and admin status while preserving the existing `TokenVerifier` interface and `ScopePolicy`.
- Added RS256 Agent Access Token issuance with 15 minute default TTL and claims for `tenant_id`, `agent_id`, `agent_version_id`, `scopes`, and `repo_scope`.
- Added local RS256 verifier that maps agent JWTs to `auth.Principal` and returns typed `auth.ErrTokenExpired` for expired tokens.
- Kept the existing `JWKSVerifier/NewJWKSVerifier` contract intact, while also mapping expired JWTs to `auth.ErrTokenExpired`, setting agent principal type, and carrying `repo_scope`.

## Tests Run/Results

- `cd backend && go test ./internal/auth -count=1` — PASS
- `cd backend && go test ./internal/identity/application -count=1` — PASS
- `cd backend && go test ./internal/auth ./internal/identity/application -count=1` — PASS
- `git diff --check` — PASS
- `gofmt` run on changed Go files.

## TDD RED/GREEN Evidence

### RED

Command:

```bash
cd backend && go test ./internal/auth -count=1
```

Result summary:

```text
FAIL agentguild.dev/agentguild/backend/internal/auth [build failed]
undefined: auth.NewOIDCProvider
undefined: auth.OIDCConfig
undefined: auth.OIDCClaims
```

This failed for the expected reason: the OIDC/session/token APIs did not exist yet.

### GREEN

Command:

```bash
cd backend && go test ./internal/auth -count=1
```

Result summary:

```text
ok agentguild.dev/agentguild/backend/internal/auth 1.988s
```

Final combined verification:

```text
ok agentguild.dev/agentguild/backend/internal/auth 1.944s
ok agentguild.dev/agentguild/backend/internal/identity/application 1.124s
```

## Files Changed

- `backend/internal/auth/oidc.go`
- `backend/internal/auth/session.go`
- `backend/internal/auth/token_issuer.go`
- `backend/internal/auth/principal.go`
- `backend/internal/auth/oauth.go`
- `backend/internal/auth/oidc_test.go`
- `backend/internal/auth/token_test.go`
- `backend/internal/auth/principal_test.go`
- `.superpowers/sdd/task-4-report.md`

## Self-Review Findings/Concerns

- The production OIDC exchanger validates issuer, audience, expiration, and token shape after OAuth token exchange, but it does not yet perform cryptographic ID token signature verification through provider discovery/JWKS. The current design keeps the exchanger injectable so a stricter provider-backed implementation can replace it without changing `OIDCProvider` callers.
- Session cookies are signed and tamper-detecting, not encrypted. They satisfy the secure/httpOnly helper requirement but should not carry secrets beyond the mapped session identity fields.
- Existing task-lifecycle `JWKSVerifier` behavior and `TokenVerifier` shape were preserved.

## Review Round 1 Fixes

- Fixed the default production OIDC exchanger so it verifies `id_token` signatures with RS256 keys fetched from configured JWKS before mapping claims into a session.
- Added `OIDCConfig.JWKSURL`; production/default exchanger construction now requires it, while fake exchanger injection remains available for unit tests.
- Made `Issuer` required so issuer validation cannot be silently skipped.
- Added production/default exchanger tests covering valid signed ID tokens, unsigned ID tokens, and invalid-signature ID tokens.
- Documented that session cookies are signed but not encrypted, so `Session` must contain only non-secret identity fields.

## RED/GREEN Evidence

### RED

Command:

```bash
cd backend && go test ./internal/auth -run TestOIDCProviderDefaultExchangerRejectsUnsignedIDToken -count=1
```

Result summary:

```text
--- FAIL: TestOIDCProviderDefaultExchangerRejectsUnsignedIDToken
Error: An error is expected but got nil.
FAIL agentguild.dev/agentguild/backend/internal/auth
```

This failed for the expected reason: the default exchanger accepted an unsigned `id_token`.

### GREEN

Command:

```bash
cd backend && go test ./internal/auth -run 'TestOIDCProviderDefaultExchanger|TestNewOIDCProviderRequiresIssuer' -count=1
```

Result summary:

```text
ok agentguild.dev/agentguild/backend/internal/auth 1.357s
```

## Verification Results

- `cd backend && go test ./internal/auth -count=1` — PASS
- `cd backend && go test ./internal/identity/application -count=1` — PASS
- `gofmt` run on changed Go files.
- `git diff --check` — PASS

## Files Changed

- `backend/internal/auth/oidc.go`
- `backend/internal/auth/oidc_test.go`
- `backend/internal/auth/session.go`
- `.superpowers/sdd/task-4-report.md`

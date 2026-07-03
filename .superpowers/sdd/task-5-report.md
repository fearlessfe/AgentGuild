# Task 5 Report: REST API for Agent Identity

## What implemented

- Added optional identity REST wiring via `rest.WithIdentityService`, `rest.WithSession`, and `rest.WithOIDCProvider`.
- Added human/session routes:
  - `GET /oauth/oidc/login`
  - `GET /oauth/oidc/callback`
  - `POST /v1/agents`
  - `GET /v1/agents`
  - `GET /v1/agents/:id`
  - `POST /v1/agents/:id:suspend`
  - `POST /v1/agents/:id:resume`
  - `POST /v1/agents/:id:revoke`
  - `GET /v1/agents/:id:token`
- Added agent self routes:
  - `POST /v1/agents/me:activate`
  - `POST /v1/agents/me:refresh`
  - `POST /v1/agents/me:heartbeat`
  - `GET /v1/agents/me`
- Added session middleware that parses signed `auth.Session` cookies and injects an `auth.Principal` into request context.
- Added bearer auth reuse for protected agent self endpoints; activation remains unauthenticated.
- Added identity-specific error mapping for invalid argument, hidden forbidden/not found, state conflict, token expired/revoked, and rate limiting.
- Updated OpenAPI with identity paths, session cookie security scheme, agent/token schemas, and validation coverage through the existing offline OpenAPI test.

## Tests run/results

- `cd backend && go test ./internal/transport/rest -run 'TestRegisterAgent|TestGetAgent|TestAgentActivate' -count=1` PASS.
- `cd backend && go test ./internal/transport/rest -count=1` PASS.
- `cd backend && go test ./internal/transport/contract -count=1` PASS.
- `cd backend && go test ./internal/auth ./internal/identity/application -count=1` PASS.
- `cd backend && gofmt -w internal/transport/rest/identity_router_test.go internal/transport/rest/agent_self_router_test.go internal/transport/rest/router.go internal/transport/rest/session_middleware.go internal/transport/rest/identity_router.go internal/transport/rest/agent_self_router.go internal/transport/rest/errors.go` PASS.
- `cd backend && git diff --check` PASS.

## TDD RED/GREEN evidence

RED command:

```bash
cd backend && go test ./internal/transport/rest -run 'TestRegisterAgent|TestGetAgent|TestAgentActivate' -count=1
```

RED output summary:

```text
FAIL agentguild.dev/agentguild/backend/internal/transport/rest [build failed]
undefined: rest.WithIdentityService
undefined: rest.WithSession
```

Note: the first RED run also exposed an incorrect test expectation that `identity/application.Principal` had an auth-only `Type` field; I corrected the test to match the identity application contract before production edits.

GREEN command:

```bash
cd backend && go test ./internal/transport/rest -count=1
```

GREEN output summary:

```text
ok agentguild.dev/agentguild/backend/internal/transport/rest
```

## Files changed

- `backend/internal/transport/rest/router.go`
- `backend/internal/transport/rest/errors.go`
- `backend/internal/transport/rest/session_middleware.go`
- `backend/internal/transport/rest/identity_router.go`
- `backend/internal/transport/rest/agent_self_router.go`
- `backend/internal/transport/rest/identity_router_test.go`
- `backend/internal/transport/rest/agent_self_router_test.go`
- `backend/internal/transport/rest/openapi.yaml`
- `.superpowers/sdd/task-5-report.md`

## Self-review findings/concerns

- `cmd/agentguild-api` still constructs `rest.NewServer` without passing an identity service, session secret, or OIDC provider. Task-owned files did not include composition/config wiring, so this change exposes the REST surface through server options and tests it at transport level.
- OIDC login currently generates an OAuth state but does not persist/validate it in a separate state cookie. Existing Task 4 auth APIs expose provider/session primitives, but no state-store contract was present in the allowed files.
- Activation rate limiting currently keys by method/path because no bearer principal exists before activation.

## Review round 1 fixes

- Added signed OIDC state cookie persistence on `GET /oauth/oidc/login`.
- Added strict OIDC callback state validation before token exchange/session creation. Missing, mismatched, or tampered state is rejected without calling `Exchange` and without setting a session cookie.
- Added OIDC route tests for login redirect cookie attributes, normal callback state flow, missing state, mismatched state, and tampered state cookie.
- Changed anonymous REST rate-limit key derivation to include caller source from `RemoteAddr` after existing `middleware.RealIP` processing.
- Added focused activation limiter test showing two anonymous callers from different `RemoteAddr` values do not share a global path-only budget.

## RED/GREEN evidence

RED command:

```bash
cd backend && go test ./internal/transport/rest -run 'TestOIDC|TestAgentActivateRateLimitKeyIncludesAnonymousRemoteAddr' -count=1
```

RED output summary:

```text
FAIL TestAgentActivateRateLimitKeyIncludesAnonymousRemoteAddr: expected 200, actual 429
FAIL TestOIDCLoginRedirectsWithSignedStateCookie: expected state cookie, got nil
FAIL TestOIDCCallbackRejectsMissingStateWithoutSession: expected 401, actual 200
FAIL TestOIDCCallbackAcceptsMatchingSignedState / Mismatched / Tampered: expected state cookie, got nil
```

GREEN command:

```bash
cd backend && go test ./internal/transport/rest -run 'TestOIDC|TestAgentActivateRateLimitKeyIncludesAnonymousRemoteAddr' -count=1
```

GREEN output summary:

```text
ok agentguild.dev/agentguild/backend/internal/transport/rest
```

## Verification results

- `cd backend && go test ./internal/transport/rest -count=1` PASS.
- `gofmt -w backend/internal/transport/rest/identity_router.go backend/internal/transport/rest/router.go backend/internal/transport/rest/identity_router_test.go backend/internal/transport/rest/agent_self_router_test.go` PASS.
- `git diff --check` PASS.

## Files changed

- `backend/internal/transport/rest/identity_router.go`
- `backend/internal/transport/rest/router.go`
- `backend/internal/transport/rest/identity_router_test.go`
- `backend/internal/transport/rest/agent_self_router_test.go`
- `.superpowers/sdd/task-5-report.md`

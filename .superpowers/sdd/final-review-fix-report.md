# Final Review Fix Report

Change: agent-onboarding-and-identity
Branch: feature/20260702/agent-onboarding-and-identity

## RED

- `cd backend && go test ./internal/acceptance -run 'TestAgentActivationAndLifecycle|TestSuspendedIdentityIssuedTokenCannotUseTaskOperations|TestRevokedIdentityIssuedTokenReturnsTokenRevokedForTaskOperations' -count=1`
  - Failed as expected: revoked identity refresh/heartbeat returned `STATE_CONFLICT`; suspended/revoked identity-issued task tokens still returned success for MCP task/execution operations.
- `cd frontend && npm test -- --run src/features/agents/agents.test.tsx`
  - Failed as expected: registration form still rendered required `Owner Email` input.
- `cd backend && go test ./internal/identity/application -run TestAgentStatusCheckerDistinguishesSuspendedAndRevokedAgents -count=1`
  - Failed as expected: `application.NewAgentStatusChecker` was undefined.
- `cd backend && go test ./internal/transport/mcp -run TestDomainErrorsMapToStableMCPCodes/token_revoked -count=1`
  - Failed as expected: `token_revoked` mapped to `INTERNAL_ERROR`.
- `cd backend && go test ./internal/transport/rest -run TestTaskRoutesMapRevokedIdentityToken -count=1`
  - Failed as expected: task REST route returned HTTP 500 instead of 401 `TOKEN_REVOKED`.

## Changes

- Added optional task application `AgentStatusChecker` and invoked it after scope authorization and before rate limiting for protected task/execution read and mutation paths.
- Added identity application `AgentStatusChecker` backed by the identity store; active agents pass, suspended/pending agents return `STATE_CONFLICT`, revoked agents return `TOKEN_REVOKED`, and unknown legacy principals are ignored.
- Wired the checker into acceptance and API service assembly without moving repository access into transport.
- Changed identity refresh/heartbeat revoked semantics to `TOKEN_REVOKED`.
- Added task REST/MCP mapping for identity `token_revoked`.
- Removed `owner_email` from the React registration form and request type; changed default/demo/fixture scopes to valid backend scopes.

## GREEN

- `cd backend && go test ./internal/acceptance -run 'TestAgentActivationAndLifecycle|TestSuspendedIdentityIssuedTokenCannotUseTaskOperations|TestRevokedIdentityIssuedTokenReturnsTokenRevokedForTaskOperations' -count=1`
  - PASS.
- `cd backend && go test ./internal/identity/application -run 'TestAgentStatusCheckerDistinguishesSuspendedAndRevokedAgents|TestSuspendedAgentCannotRefreshToken|TestAgentHeartbeatUpdatesLastSeenForAgentSelf' -count=1`
  - PASS.
- `cd backend && go test ./internal/transport/mcp -run TestDomainErrorsMapToStableMCPCodes/token_revoked -count=1`
  - PASS.
- `cd backend && go test ./internal/transport/rest -run TestTaskRoutesMapRevokedIdentityToken -count=1`
  - PASS.
- `cd frontend && npm test -- --run src/features/agents/agents.test.tsx`
  - PASS: 5 tests.
- `cd backend && go test ./internal/application ./internal/auth ./internal/identity/... ./internal/transport/rest ./internal/transport/mcp ./internal/acceptance -count=1`
  - PASS.
- `cd frontend && npm test -- --run`
  - PASS: 13 tests.
- `env GOCACHE=/private/tmp/agentguild-go-build-cache make build`
  - PASS. Plain `make build` was blocked by sandbox access to `~/Library/Caches/go-build`; escalation was rejected by approval service 503.
- `git diff --check`
  - PASS.

---
change: agent-task-lifecycle
verify_mode: full
verified_at: 2026-07-02
---

# Agent Task Lifecycle — Verification Report

## Summary

- **Change:** `agent-task-lifecycle`
- **Branch:** `feature/20260702/agent-task-lifecycle`
- **Verify mode:** full
- **Result:** PASS
- **Verified at:** 2026-07-02

## Checks

### 1. OpenSpec tasks completion

`openspec/changes/agent-task-lifecycle/tasks.md`:

- [x] 1.1 Task/Execution/Lease/IdempotencyRecord/audit data models
- [x] 1.2 Task publish, discover, read, permission filtering
- [x] 1.3 Concurrent Claim, 10m Lease, heartbeat, generation fencing, deadline, expiry reclaim
- [x] 1.4 State machine, concurrent claim, retry, expiry race tests
- [x] 2.1 Shared Application Service, commands, errors
- [x] 2.2 Task REST API and OpenAPI contract
- [x] 2.3 Stateless Streamable HTTP MCP Server and OAuth 2.1 auth
- [x] 2.4 Task publish/discover/read/claim/heartbeat/status MCP tools
- [x] 2.5 REST/MCP behavior, schema, scope, idempotency contract tests
- [x] 3.1 React task list, filters, read-only detail
- [x] 3.2 Lease reaper, Langfuse TraceCostProvider, cost coverage, rate limit, audit queries
- [x] 3.3 Agent polling, disconnection recovery, illegal state transition acceptance tests

### 2. Design alignment

- `openspec/changes/agent-task-lifecycle/design.md` decisions are reflected in implementation:
  - Task vs Execution separation (`backend/internal/domain`)
  - PostgreSQL conditional update + unique constraint for Claim (`backend/internal/application/claim.go`, migrations)
  - DB-time authoritative clock, 10m soft lease + 30s grace + generation fencing (`backend/internal/domain/execution.go`)
  - REST and MCP share Application Service (`backend/internal/transport/rest`, `backend/internal/transport/mcp`)
  - Stateless Streamable HTTP MCP with OAuth 2.1 (`backend/internal/transport/mcp/server.go`)
  - Idempotency by tenant/actor/operation/key (`backend/internal/postgres/idempotency.go`)
  - Required deadline, no max runtime (`backend/internal/application/service.go`)
  - TraceCostProvider async cost observation (`backend/internal/telemetry`, `backend/internal/worker/outbox.go`)

### 3. Delta spec scenario coverage

| Spec | Scenario | Evidence |
|------|----------|----------|
| task-lifecycle | Illegal state migration | `backend/internal/domain/task_test.go`, `backend/internal/application/service_test.go` |
| task-lifecycle | Role-based state migration | `backend/internal/application/service_test.go` |
| task-lifecycle | Concurrent claim | `backend/internal/postgres/claim_concurrency_test.go`, `backend/internal/acceptance/lifecycle_test.go` |
| task-lifecycle | DB constraint for single active execution | `backend/internal/postgres/repository_test.go` |
| task-lifecycle | Lease expiry / heartbeat stop | `backend/internal/postgres/reaper_test.go`, `backend/internal/application/claim_test.go` |
| task-lifecycle | Grace-period heartbeat | `backend/internal/application/claim_test.go` |
| task-lifecycle | Stale generation write | `backend/internal/application/claim_test.go`, `backend/internal/postgres/claim_concurrency_test.go` |
| task-lifecycle | Deadline exceeded | `backend/internal/application/claim_test.go`, `backend/internal/acceptance/lifecycle_test.go` |
| task-lifecycle | Missing deadline rejection | `backend/internal/application/service_test.go` |
| task-lifecycle | Idempotent claim retry | `backend/internal/postgres/idempotency_test.go` |
| task-lifecycle | Idempotency mismatch | `backend/internal/postgres/idempotency_test.go` |
| task-lifecycle | Langfuse unavailable | `backend/internal/telemetry/langfuse_test.go`, `backend/internal/worker/outbox_test.go` |
| task-lifecycle | External tool without trace | `backend/internal/telemetry/langfuse_test.go` |
| task-mcp-access | MCP claim | `backend/internal/transport/mcp/server_test.go` |
| task-mcp-access | MCP/REST equivalence | `backend/internal/transport/contract/equivalence_test.go` |
| task-mcp-access | Missing publish scope | `backend/internal/transport/mcp/server_test.go` |
| task-mcp-access | Non-holder execution mutation | `backend/internal/transport/mcp/server_test.go` |
| task-mcp-access | No lease token in params | `backend/internal/transport/mcp/server_test.go` |
| task-mcp-access | Illegal migration rejected | `backend/internal/transport/mcp/server_test.go` |
| task-mcp-access | Request_id idempotency | `backend/internal/transport/mcp/server_test.go` |
| task-mcp-access | Invalid cursor | `backend/internal/application/service_test.go` |
| task-mcp-access | Stable error codes | `backend/internal/transport/contract/equivalence_test.go` |

### 4. Build / test verification

```text
$ make verify

PASS:
- cd backend && go build ./...
- cd frontend && npm run build
- cd backend && go test -race ./... -count=1
- cd frontend && npm test -- --run (8 tests)
- cd frontend && npm run test:e2e (3 tests)
```

### 5. Security / safety

- No hard-coded secrets in source.
- `VITE_API_TOKEN` only used in development; production uses `window.AG_TOKEN` or same-origin cookie.
- OAuth scope checks in REST and MCP.
- Safe FORBIDDEN/NOT_FOUND responses for non-admin callers.

### 6. Spec drift / deferred items

- OpenSpec handoff hash changed from design phase because `tasks.md` and plan were updated during build; this is expected for delta spec adjustments.
- Deferred items accepted:
  - JWKS cache eviction deferred (production TTL/config).
  - `splitScopeString` comma separator support not a formal requirement.

## Conclusion

All OpenSpec tasks are complete, the implementation aligns with the Design Doc and delta specs, and `make verify` passes end-to-end. The change is ready for branch finishing and archive.

# Subagent Progress

- Change: `agent-task-lifecycle`
- Plan: `docs/superpowers/plans/2026-07-02-agent-task-lifecycle.md`
- Review mode: `thorough`
- TDD mode: `tdd`

## Completed Tasks

- Task 1 domain lifecycle, Task 2 postgres persistence, Task 3 application service, Task 4 claim/lease, Task 5 REST/OAuth — all checked off (plan gates + steps).
- OpenSpec 1.1/1.2/1.3/1.4/2.1/2.2 completed.
- Task 4 round-1 review APPROVED by fresh re-reviewer; fixes `169ccb5`.
- Task 5 round-2 review APPROVED; implementation `638c7b8` + fixes `3a7e329`.

## Deferred to FINAL review

- (a) Task 3 minor: fake `ListTaskRecords` lacks `PublisherAgentVersionID` filter regression coverage.
- (b) `reaper.go` batch-abort on single-row anomaly — re-evaluate when submit/accept/complete flows land (currently unreachable, MINOR).
- (c) design doc §4.1 says "Active" but impl uses "claimed" — informational, pre-Task-4 naming.
- (d) JWKS cache never evicts stale keys; production TTL/eviction deferred.
- (e) `splitScopeString` accepts both space and comma separators; comma support not a formal requirement, could be revisited.

## Current Task

- Plan task: `Completion gate: Task 6 MCP adapter`
- OpenSpec mapping: `2.3 stateless Streamable HTTP MCP Server + OAuth 2.1 auth; 2.4 MCP tools (publish/list/get/claim/start/heartbeat/get); 2.5 REST/MCP behavioral consistency, schema, scope and idempotency contract tests`
- Phase: `implementing`
- Implementer status: `dispatched`
- Confirmed technical baseline: `Go 1.26.4; PostgreSQL 18.4; official MCP Go SDK v1.6.1; OAuth 2.1 via existing auth.TokenVerifier`
- Files (planned): `internal/transport/mcp/server.go`, `internal/transport/mcp/tools.go`, `internal/transport/mcp/errors.go`, `internal/transport/mcp/server_test.go`
- Review mode: `thorough` — batch/final review to run after implementer reports DONE
- Review/fix round: `0`

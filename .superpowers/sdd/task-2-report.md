# Task 2 identity persistence report

## What I implemented

- Added PostgreSQL migration `000002_agent_identity` for `agents`, `agent_versions`, `activation_credentials`, and `identity_events`.
- Ensured identity primary and unique constraints include `tenant_id`, including `identity_events PRIMARY KEY (tenant_id, id)`.
- Added tenant-scoped application ports in `identity/application` for Store, Tx, Agent, Version, Credential, and Audit repositories.
- Implemented PostgreSQL Store.WithTx with transaction-scoped database time.
- Implemented Agent, AgentVersion, ActivationCredential, and append-only IdentityEvent persistence.
- Hardened credential consumption with conditional SQL on tenant, agent, credential id, stored hash, pending status, and expiry.
- Implemented stale-safe Agent state updates by conditioning status transitions on the domain event from-state.
- Wired migration `000002_agent_identity` into the test Postgres helper up/down lifecycle.

## Tests run and results

- `cd backend && go test ./internal/identity/postgres -run TestCredentialConsumeRequiresStoredCredentialIdentity -count=1`: PASS.
- `cd backend && go test ./internal/identity/postgres -run TestIdentityPrimaryAndUniqueConstraintsIncludeTenantID -count=1`: PASS.
- `cd backend && go test ./internal/identity/postgres -run TestAgentTenantIsolation -count=1`: PASS.
- `cd backend && go test ./internal/identity/postgres -count=1`: PASS.
- `gofmt` on changed Go files: PASS.
- `git diff --check`: PASS.

## TDD RED/GREEN evidence

Initial required command against the inherited partial worktree:

```text
cd backend && go test ./internal/identity/postgres -run TestAgentTenantIsolation -count=1
ok  	agentguild.dev/agentguild/backend/internal/identity/postgres	0.614s
```

Because the inherited partial implementation already passed that tenant test, I added focused failing tests before production fixes.

RED 1:

```text
cd backend && go test ./internal/identity/postgres -run TestCredentialConsumeRequiresStoredCredentialIdentity -count=1
--- FAIL: TestCredentialConsumeRequiresStoredCredentialIdentity
Expected error with "activation token is expired or invalid" in chain but got nil.
FAIL
```

GREEN 1:

```text
cd backend && go test ./internal/identity/postgres -run TestCredentialConsumeRequiresStoredCredentialIdentity -count=1
ok  	agentguild.dev/agentguild/backend/internal/identity/postgres	0.661s
```

RED 2:

```text
cd backend && go test ./internal/identity/postgres -run TestIdentityPrimaryAndUniqueConstraintsIncludeTenantID -count=1
--- FAIL: TestIdentityPrimaryAndUniqueConstraintsIncludeTenantID
Should be empty, but was [identity_events.identity_events_pkey]
FAIL
```

GREEN 2:

```text
cd backend && go test ./internal/identity/postgres -run TestIdentityPrimaryAndUniqueConstraintsIncludeTenantID -count=1
ok  	agentguild.dev/agentguild/backend/internal/identity/postgres	0.350s
```

Final GREEN:

```text
cd backend && go test ./internal/identity/postgres -count=1
ok  	agentguild.dev/agentguild/backend/internal/identity/postgres	0.694s
```

## Files changed

- `backend/migrations/000002_agent_identity.up.sql`
- `backend/migrations/000002_agent_identity.down.sql`
- `backend/internal/identity/application/ports.go`
- `backend/internal/identity/postgres/store.go`
- `backend/internal/identity/postgres/agent_repository.go`
- `backend/internal/identity/postgres/audit_repository.go`
- `backend/internal/identity/postgres/repository_test.go`
- `backend/internal/testdb/postgres.go`
- `.superpowers/sdd/task-2-report.md`

## Self-review findings/concerns

- No blocking concerns.
- I intentionally did not modify plan checkboxes, OpenSpec task checkboxes, or `openspec/changes/agent-onboarding-and-identity/.comet/subagent-progress.md`.
- Integration tests require PostgreSQL 18.4 via `AGENTGUILD_TEST_DATABASE_URL`, local port 55432, or Docker fallback.

## Review round 1 fixes

- Fixed `CredentialRepository.Save` for non-consumed credentials by renumbering SQL placeholders to match the six supplied arguments.
- Added `RowsAffected()` handling for the non-consumed update path; zero-row updates now return `domain.ErrNotFound`.
- Added regression coverage for saving updates to a pending activation credential.
- Did not keep optional concurrent-consumption or rollback/reapply test hardening because initial attempts exposed existing semantics outside this scoped reviewer fix.

## Review round 1 RED/GREEN command evidence

RED:

```text
cd backend && go test ./internal/identity/postgres -run TestCredentialSavePersistsPendingCredentialUpdates -count=1
--- FAIL: TestCredentialSavePersistsPendingCredentialUpdates (0.08s)
    repository_test.go:140:
        Error Trace: /Users/pengzhen/work/AgentGuild/backend/internal/identity/postgres/repository_test.go:140
        Error:       Received unexpected error:
                     ERROR: could not determine data type of parameter $3 (SQLSTATE 42P18)
        Test:        TestCredentialSavePersistsPendingCredentialUpdates
FAIL
FAIL	agentguild.dev/agentguild/backend/internal/identity/postgres	0.610s
FAIL
```

GREEN focused:

```text
cd backend && go test ./internal/identity/postgres -run TestCredentialSavePersistsPendingCredentialUpdates -count=1
ok  	agentguild.dev/agentguild/backend/internal/identity/postgres	0.606s
```

GREEN package:

```text
cd backend && go test ./internal/identity/postgres -count=1
ok  	agentguild.dev/agentguild/backend/internal/identity/postgres	1.192s
```

## Review round 1 files changed

- `backend/internal/identity/postgres/agent_repository.go`
- `backend/internal/identity/postgres/repository_test.go`
- `.superpowers/sdd/task-2-report.md`

## Review round 1 verification results

- `gofmt -w backend/internal/identity/postgres/agent_repository.go backend/internal/identity/postgres/repository_test.go`: PASS.
- `cd backend && go test ./internal/identity/postgres -run TestCredentialSavePersistsPendingCredentialUpdates -count=1`: PASS.
- `cd backend && go test ./internal/identity/postgres -count=1`: PASS.

# Task 3 Report

## Status

PASS — shared task application service, authorization policy, stable envelopes/errors,
signed pagination cursor, and atomic cancellation are implemented.

## TDD evidence

### RED

- `go test ./internal/auth -count=1`: package had no implementation for
  `Principal`, `ScopePolicy`, or `TokenVerifier`.
- `go test ./internal/application -count=1`: `PublishTask`, `ListTasks`,
  `GetTask`, `CancelTask`, `Service`, and query contracts were undefined.
- `go test ./internal/domain -run TestExecutionCancel -count=1`: cancellation
  method and terminal state were undefined.
- `go test ./internal/postgres -run TestExecutionMigrationAcceptsCancelledAsTerminal -count=1`:
  migration rejected `cancelled` executions.
- PostgreSQL compile check failed because `Tx` lacked `ListTaskRecords` after
  the application port was introduced.
- `TestGetIsTenantScopedAndCancelAtomicallyCancelsActiveExecution` caught the
  unstable `task.canceled` event spelling.
- `TestExecutionCancelCoversEveryActiveStatus` caught missing active execution
  statuses.
- `TestErrorCodeAndFieldSurviveWrapping` caught missing `domain.CodeOf` and
  `domain.FieldOf` helpers.

### GREEN

- Focused application/auth/domain/PostgreSQL suite passed.
- `go test -race ./...` passed.
- `go vet ./...` passed.
- `git diff --check` passed.

## Files

- `backend/internal/auth/principal.go`
- `backend/internal/auth/principal_test.go`
- `backend/internal/application/contracts.go`
- `backend/internal/application/service.go`
- `backend/internal/application/task_commands.go`
- `backend/internal/application/task_queries.go`
- `backend/internal/application/service_test.go`
- `backend/internal/application/ports.go`
- `backend/internal/domain/errors.go`
- `backend/internal/domain/errors_test.go`
- `backend/internal/domain/execution.go`
- `backend/internal/domain/execution_test.go`
- `backend/internal/postgres/task_repository.go`
- `backend/internal/postgres/repository_test.go`
- `backend/internal/postgres/migration_test.go`
- `backend/migrations/000001_task_lifecycle.up.sql`

## Self-check

- Application imports only domain/auth and application ports; it does not
  import PostgreSQL or transport packages.
- Commands use `Tx.Now` and follow policy → idempotency acquire → domain and
  persistence → audit/outbox → idempotency complete.
- Task bodies round-trip through `TaskRecord`; no synthesized content is used.
- Cursor payloads are HMAC signed and bind tenant, filter digest, sort version,
  keyset position, and expiry. Limit defaults to 20 and rejects values over 100.
- Get/list are tenant scoped. Cancel requires scope and publisher ownership.
- Task and every active Execution state are cancelled in one transaction.
- The Execution cancelled state and migration update were a coordinated fix for
  a proven existing spec/schema gap; they were explicitly approved by the root
  task owner.

## Concerns

- The cursor secret must be supplied from deployment configuration. An empty
  secret remains functionally signed but is not suitable for production.
- No Claim or heartbeat application behavior was added.

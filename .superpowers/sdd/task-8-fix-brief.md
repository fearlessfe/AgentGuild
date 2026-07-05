# Task 8 Fix Brief

## Findings to address

1. **CRITICAL**: Restore `SHUTDOWN_TIMEOUT` parsing in `backend/internal/config/config.go` while keeping `REPUTATION_WORKER_INTERVAL` (default `10s`).
2. **CRITICAL**: Make `Worker.RunOnce` incremental: load existing projections per `(tenantID, agentVersionID)`, apply new signals to them, then upsert.
3. **IMPORTANT**: Add regression test spanning multiple batches to verify projection accumulation.
4. **IMPORTANT**: Align `ProjectionRepository` port — add `Upsert(ctx, ProjectionRecord)` and `ListByAgentVersion(ctx, tenantID, agentVersionID)` methods and implement them in `backend/internal/reputation/postgres/projection_repository.go`.

## Review Package

`/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/review-38b4878..5ada486.diff`

## Base Commit

HEAD is currently `7915bd8`. Fix on top of this.

## Required Tests

Run focused tests:
```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/backend
go test ./internal/reputation/... ./internal/config/... ./internal/review/postgres/... -v
```

Then run full backend suite:
```bash
go test ./...
```

## Report

Write report to `/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/task-8-fix-report.md`.

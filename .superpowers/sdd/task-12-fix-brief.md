# Task 12 Fix Brief

## Findings to address

**IMPORTANT 1**: The `GET /v1/reputation` REST endpoint and MCP `reputation_get` tool are wired to a placeholder that always returns an empty `ProjectionView`. The acceptance tests bypass these public interfaces and read projections directly from the repository.

Fix:
- Implement a real `acceptanceReputationService.GetProjection` in `backend/internal/acceptance/env.go` by querying `reputationpostgres.NewProjectionRepository(env.DB)`.
- Ensure `ProjectionView` in REST/MCP transport matches `reputationdomain.Projection` fields.
- Add assertions in `TestEndToEndReviewAcceptedUpdatesProjection` and `TestCrossAgentVersionReputationIsolation` that call REST `GET /v1/reputation` and/or MCP `reputation_get` and verify counts/rates.

**IMPORTANT 2**: The revision-isolation test validates comment isolation only through the repository, not via public API.

Fix:
- In `TestRevisionRequestedNewExecutionCommentsIsolated`, after creating both reviews and comments, use `env.MCP.As("token-reviewer").ReviewGet(review2.ID)` or REST equivalent and assert the returned review does not contain the old comment text.

## Optional Minor cleanups

- Remove unused `reviewerSession()` from `env.go`.
- Update misleading comment in `CreateResubmissionExecution` to say "new execution".
- Fix or update the comment in `TestHardGatesFailedPreventsAcceptDecision`.

## Base Commit

HEAD is `fd760b8`. Fix on top of this.

## Required Tests

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/backend
go test ./internal/acceptance/... -v -run 'Review|Reputation'
go test ./... -count=1
```

## Report

Write report to `/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/task-12-fix-report.md`.

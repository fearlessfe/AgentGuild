# Task 9 Fix Brief

## Findings to address

1. **IMPORTANT**: Fix loading state in `frontend/src/features/reviews/ReviewPage.tsx` so a disabled diff query does not keep the page in loading state when review fails or `submission_id` is missing.
2. **IMPORTANT**: Prevent local comments from being overwritten on refetch in `frontend/src/features/reviews/ReviewPage.tsx` (seed server comments only once, or merge without discarding local comments; optionally disable `refetchOnWindowFocus`).

## Optional Minor cleanups

- Remove no-op `DiffRowGroup` wrapper in `DiffViewer.tsx`.
- Make comment textarea `id` unique per open form.
- Simplify `workspace` derivation in `AppShell.tsx`.
- Add demo stubs for `GET /v1/reviews/:id` and `GET /v1/submissions/:id/diff` if appropriate.

## Review Package

`/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/review-0e01d5a..8f58584.diff`

## Base Commit

HEAD is `785b5b1`. Fix on top of this.

## Required Tests

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/frontend
npm test -- --run
npm run build
```

## Report

Write report to `/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/task-9-fix-report.md`.

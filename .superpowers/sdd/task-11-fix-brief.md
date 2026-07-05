# Task 11 Fix Brief

## Finding to address

**IMPORTANT**: In `frontend/src/features/reputation/ReputationPage.tsx`, `form` and `filters` state are initialized from `searchParams` only on mount. When the URL changes later (browser back/forward, external link), the page does not refresh inputs or data.

Fix:
- Derive committed `filters` directly from `searchParams` using `useMemo`, so the query key updates automatically when the URL changes.
- Keep `form` as buffered local UI state.
- On submit, call `setSearchParams(next)`; the derived `filters` will then update the query.
- Avoid infinite render loops.

## Optional Minor fixes

- Use type-only import for `FormEvent`.
- Add explicit return type to `getReputationProjection`.
- Update `AppShell` context-bar Agent label to handle reputation workspace.
- Strengthen URL-update test to assert actual search params.

## Base Commit

HEAD is `b14e3f5`. Fix on top of this.

## Required Tests

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/frontend
npm test -- --run
npm run build
```

## Report

Write report to `/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/task-11-fix-report.md`.

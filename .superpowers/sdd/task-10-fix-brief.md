# Task 10 Fix Brief

## Finding to address

**IMPORTANT**: In `frontend/src/features/reviews/reviews.api.ts`, `addComment` and `submitDecision` send the idempotency key as `request_id` in the request body. Per global constraints, REST must use the `Idempotency-Key` HTTP header.

Fix:
- Pass idempotency key via header: `headers: { "Idempotency-Key": generateRequestId() }`.
- Remove `request_id` from the request bodies.
- Update `frontend/src/features/reviews/ReviewPage.test.tsx:429` to assert on the `Idempotency-Key` header instead of `body.request_id`.

## Optional Minor cleanups

- Remove redundant read-only summary paragraph for submitted reviews in `ReviewPage.tsx`.
- Reset `hasSeededComments.current` and `hasSeededScores.current` to `false` in a `useEffect` keyed to `reviewId`.
- Keep comment draft until mutation succeeds, or restore on error.
- Disable decision buttons while `rubricQuery.isPending` is true.

## Base Commit

HEAD is `a0138c8`. Fix on top of this.

## Required Tests

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/frontend
npm test -- --run
npm run build
```

## Report

Write report to `/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/task-10-fix-report.md`.

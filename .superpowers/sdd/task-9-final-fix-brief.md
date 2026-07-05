# Task 9 Final Fix Brief

## Finding to address

**IMPORTANT**: In `frontend/src/app/AppShell.tsx`, the `<ReviewPage />` rendered inside `ReviewWorkspace` is not keyed by `reviewId`. When navigating between reviews within the same `/reviews/:reviewId` route, React reuses the component instance. Because `hasSeededComments` ref stays `true`, server comments for the new review are never seeded, and `selectedPath` / local `comments` retain old review state.

Fix: pass `key={reviewId}` to `<ReviewPage />` in `ReviewWorkspace`, OR reset the seed ref and `selectedPath` inside `ReviewPage` when `reviewId` changes.

## Base Commit

HEAD is `fd77e41`. Fix on top of this.

## Required Tests

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/frontend
npm test -- --run
npm run build
```

Add or update a test that simulates switching `reviewId` and asserts new server comments / selected path are loaded.

## Report

Write report to `/Users/pengzhen/work/AgentGuild/.worktrees/feature/20260704/code-review-and-reputation/.superpowers/sdd/task-9-final-fix-report.md`.

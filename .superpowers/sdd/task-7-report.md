# Task 7 React Agents UI Report

## Scope

Implemented the React Agents management surface for `agent-onboarding-and-identity` Task 7:

- Typed Agents REST client and demo-mode handlers
- `/agents`, `/agents/new`, `/agents/:id` routes in `AppShell`
- Agents list, registration form, one-time activation token reveal, and detail/status controls
- Component tests for one-time token callback flow, revoked control hiding, and status filtering
- Playwright spec for agent onboarding flows

## Changed Files

- `frontend/src/api/client.ts`
- `frontend/src/api/fixtures.ts`
- `frontend/src/app/AppShell.tsx`
- `frontend/src/styles/tokens.css`
- `frontend/src/features/agents/agents.types.ts`
- `frontend/src/features/agents/agents.api.ts`
- `frontend/src/features/agents/AgentList.tsx`
- `frontend/src/features/agents/AgentRegister.tsx`
- `frontend/src/features/agents/AgentTokenReveal.tsx`
- `frontend/src/features/agents/AgentDetail.tsx`
- `frontend/src/features/agents/agents.test.tsx`
- `frontend/e2e/agent-onboarding.spec.ts`

## RED Evidence

Command:

```bash
cd frontend && npm test -- --run src/features/agents/agents.test.tsx
```

Observed failure summary:

- `Failed to resolve import "./AgentDetail" from "src/features/agents/agents.test.tsx"`
- Cause: Agents components did not exist yet, so the new behavior tests failed before implementation

## GREEN Evidence

Commands:

```bash
cd frontend && npm test -- --run src/features/agents/agents.test.tsx
cd frontend && npm test -- --run
cd frontend && npm run build
git diff --check
```

Observed passing summary:

- `src/features/agents/agents.test.tsx (3 tests) passed`
- `vitest --run` passed with `4` files and `11` tests green
- `npm run build` passed with `tsc -b && vite build`
- `git diff --check` returned clean output

## Concerns

- `cd frontend && npm run test:e2e -- e2e/agent-onboarding.spec.ts` could not complete in this sandbox because the local Vite server was not permitted to bind `127.0.0.1:5173` or `::1:5173` (`listen EPERM`). The spec file was added and kept aligned with demo-mode routes, but runtime verification of that Playwright path remains blocked by the environment rather than by a page assertion failure.

## Task 7 Review Round 1 Fix

### What changed

- Fixed agents status mutations to use the backend's colon routes: `/v1/agents/{id}:suspend`, `/v1/agents/{id}:resume`, `/v1/agents/{id}:revoke`.
- Aligned the register response contract with `RegisterAgentResponse`: the frontend now consumes `agent`, `activation_token`, and `activation_expires_at`.
- Removed unsupported `status` query usage from `/v1/agents`; the UI now fetches the full list once and filters rows client-side.
- Hid suspend/resume controls for `pending_activation`; only `active` shows suspend and only `suspended` shows resume.
- Updated demo handlers, fixtures, component tests, and Playwright assertions to match the REST contract.

### RED evidence

Command:

```bash
cd frontend && npm test -- --run src/features/agents/agents.test.tsx
```

Observed failures before the fix:

- Registration flow rendered an empty activation token because the UI expected `token` / `expires_at` instead of `activation_token` / `activation_expires_at`.
- Status filtering sent an unsupported `status` query parameter to `/v1/agents`.
- Status mutation posted to `/api/v1/agents/agent-1/suspend` instead of `/api/v1/agents/agent-1:suspend`.
- `pending_activation` detail incorrectly rendered the Suspend control.

### GREEN evidence

Commands:

```bash
cd frontend && npm test -- --run src/features/agents/agents.test.tsx
cd frontend && npm test -- --run
cd frontend && npm run build
git diff --check
```

Observed results:

- `src/features/agents/agents.test.tsx`: `5` tests passed.
- Full frontend suite: `4` files and `13` tests passed.
- `npm run build`: passed with `tsc -b && vite build`.
- `git diff --check`: clean output.

### Concerns

- Playwright E2E was not rerun in this fix round. The known local-server bind restriction (`listen EPERM` on `127.0.0.1:5173` / `::1:5173`) still applies in this sandbox, so the spec was updated but not re-verified end-to-end here.

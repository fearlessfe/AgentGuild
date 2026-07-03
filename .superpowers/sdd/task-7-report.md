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

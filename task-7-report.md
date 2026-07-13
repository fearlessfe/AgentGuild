# Task 7 default-delete consistency report

## RED

- Command: `cd frontend && npm test -- --run src/features/git/GitIntegrationScreen.test.tsx src/api/client.test.ts`
- Result: 2 failed, 20 passed.
- Expected failures:
  - `GitIntegrationScreen` called `listGitHubApps` only once, proving a successful plural delete did not refresh authoritative server state.
  - Demo plural deletion left the remaining App with `is_default: false`, proving it did not share singular delete/promotion semantics.

## GREEN

- Focused command: `cd frontend && npm test -- --run src/features/git/GitIntegrationScreen.test.tsx src/api/client.test.ts`
- Focused result: 2 files passed, 22 tests passed.
- Full unit command: `cd frontend && npm test -- --run`
- Full unit result: 16 files passed, 94 tests passed.
- Build command: `cd frontend && npm run build`
- Build result: TypeScript project build and Vite production build completed successfully.

## Final demo empty-state edge

- RED command: `cd frontend && npm test -- --run src/api/client.test.ts src/features/repositories/RepositoryOnboardingScreen.test.tsx`
- RED result: 2 failed, 22 passed. After deleting both demo Apps, onboarding omitted `github_app`; a legacy omitted value also crashed `RepositoryOnboardingScreen` while reading `configured`.
- Fix: demo onboarding now returns `{ configured: false }` when no default App exists, matching production; the screen uses optional access for the empty-state condition.
- Focused GREEN: 2 files passed, 24 tests passed.
- Full frontend unit: 16 files passed, 96 tests passed.
- Frontend build: TypeScript and Vite production build passed.

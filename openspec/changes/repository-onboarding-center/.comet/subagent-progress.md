# Subagent Progress: repository-onboarding-center

## Current Task

- Plan task: `Task 7: Dokploy-style GitHub App Install Continuation`
- OpenSpec mapping: `3.4 Add Dokploy-style GitHub App install continuation: show created-but-not-installed state, provide install redirect, and preserve App credentials when GitHub returns an installation id.`
- Stage: `done`
- Review mode: `standard`
- TDD mode: `tdd`
- Review/fix rounds: `1`

## Dispatch

- Brief: `.superpowers/sdd/task-6-brief.md`
- Report: `.superpowers/sdd/task-6-report.md`
- Base commit: `3999e46`
- Implementer: main coordinator

## Evidence

- Implementation commit: `3f2eebe fix: handle repository onboarding review findings`
- Changed files:
  - `docs/superpowers/plans/2026-07-09-repository-onboarding-center.md`
  - `openspec/changes/repository-onboarding-center/tasks.md`
  - `openspec/changes/repository-onboarding-center/.comet/subagent-progress.md`
  - `frontend/src/features/repositories/RepositoryOnboardingScreen.tsx`
  - `frontend/src/features/repositories/RepositoryOnboardingScreen.test.tsx`
- RED: not applicable; final verification task
- GREEN:
  - `cd backend && go test ./internal/git/application ./internal/git/postgres ./internal/transport/rest -count=1`
  - PASS: application, postgres, rest packages
  - `cd backend && go build ./...`
  - PASS
  - `cd frontend && npm test -- --run`
  - PASS: 14 files / 66 tests
  - `cd frontend && npm run build`
  - PASS: Vite build completed

## Reviews

- Final review: important findings from `019f4622-4471-7cf0-9acf-f2e4c710924d` (Einstein)
- Resolved feedback:
  - Show App repository listing error/empty state in onboarding UI: fixed in `3f2eebe`.
  - Key frontend added-state and local merge by `(source_type, full_name)` instead of `full_name` only: fixed in `3f2eebe`.
- Accepted minor:
  - Public resolver base URL coupling to `cfg.GitHub.BaseURL`; not blocking this build, record for follow-up configuration hardening.
- Recheck:
  - `cd frontend && npm test -- RepositoryOnboardingScreen --run` PASS: 6 tests.
  - `cd frontend && npm test -- --run src/features/repositories/RepositoryOnboardingScreen.test.tsx src/features/sync/SyncRuleScreen.test.tsx` PASS: 10 tests.
  - `cd backend && go test ./internal/git/application ./internal/git/postgres ./internal/transport/rest -count=1` PASS after final fixes.
  - `cd backend && go build ./...` PASS after final fixes.
  - `cd frontend && npm test -- --run` PASS: 14 files / 69 tests after final fixes.
  - `cd frontend && npm run build` PASS after final fixes.

## Incremental Evidence: Dokploy-style Install Continuation

- RED:
  - `cd backend && go test ./internal/git/application ./internal/transport/rest -run 'GitHubAppInstall|GitHubManifest_Installed' -count=1`
    - FAIL: `GitHubAppManager` had no `Install`; installed callback cleared stored private key.
  - `cd backend && go test ./internal/git/application ./internal/transport/rest -run 'ManifestBuildInstallURL|GitHubManifest_InstallRedirect' -count=1`
    - FAIL: `BuildInstallURL` missing and `/oauth/github/app/install` returned 404.
  - `cd frontend && npm test -- --run src/features/git/GitIntegrationScreen.test.tsx`
    - FAIL: `githubInstallUrl` missing.
  - `cd frontend && npm test -- --run src/features/repositories/RepositoryOnboardingScreen.test.tsx`
    - FAIL: repository onboarding Step 1 did not show created-but-not-installed state.
- GREEN:
  - `cd backend && go test ./internal/git/application ./internal/transport/rest -run 'GitHubAppInstall|GitHubManifest_Installed|ManifestBuildInstallURL|GitHubManifest_InstallRedirect' -count=1` PASS.
  - `cd frontend && npm test -- --run src/features/git/GitIntegrationScreen.test.tsx` PASS: 3 tests.
  - `cd frontend && npm test -- --run src/features/repositories/RepositoryOnboardingScreen.test.tsx` PASS: 7 tests.
  - `cd backend && go test ./cmd/agentguild-api ./internal/git/application ./internal/transport/rest -count=1` PASS.
  - `cd backend && go build ./...` PASS.
  - `cd frontend && npm test -- --run` PASS: 15 files / 72 tests.
  - `cd frontend && npm run build` PASS.
  - Compatibility recheck after old no-`app_slug` local configuration was observed:
    - `cd frontend && npm test -- --run src/features/git/GitIntegrationScreen.test.tsx src/features/repositories/RepositoryOnboardingScreen.test.tsx` PASS: 10 tests.
    - `cd frontend && npm run build` PASS.

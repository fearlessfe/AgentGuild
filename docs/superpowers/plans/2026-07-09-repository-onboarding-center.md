---
change: repository-onboarding-center
design-doc: docs/superpowers/specs/2026-07-09-repository-onboarding-center-design.md
base-ref: 131ef493e502d3254a0ed230e76cdf867792491a
---

# Repository Onboarding Center Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a first-class repository onboarding center where admins can inspect/install GitHub App access, add repositories from App access or public GitHub URLs, and view added repositories separately from sync rules.

**Architecture:** Add a small tenant-scoped repository inventory backend module, expose onboarding summary and mutation endpoints through REST, then add a dedicated `/repositories` console route. GitHub App installation discovery remains live through the existing GitHub App manager; only selected App repositories and public repositories are persisted.

**Tech Stack:** Go 1.26, PostgreSQL 18.4 migrations, chi REST routes, React 19.1, TypeScript 5.8, Vite, Vitest, Testing Library.

## Global Constraints

- All stored resources MUST include `tenant_id` and all access MUST be tenant-scoped.
- Repository onboarding MUST NOT create Issue sync rules.
- GitHub App private key material MUST never be returned to the frontend.
- Human mutation endpoints MUST require session auth and admin privileges where the existing sync/GitHub App surfaces require admin control.
- The console entry MUST be discoverable without using the overview/dashboard page.
- Demo mode MUST show both GitHub App repositories and public repositories.

---

## File Structure

- `backend/migrations/000012_repository_onboarding.up.sql`: create tenant-scoped onboarded repository storage.
- `backend/migrations/000012_repository_onboarding.down.sql`: drop repository onboarding storage.
- `backend/internal/testdb/postgres.go`: register migration `000012` in up/down helpers.
- `backend/internal/git/application/repository_onboarding.go`: domain-facing service, commands, views, validation.
- `backend/internal/git/application/repository_onboarding_test.go`: service tests for tenant isolation, App repo selection, public repo validation.
- `backend/internal/git/application/contracts.go`: repository store/source interfaces if they are shared.
- `backend/internal/git/postgres/onboarded_repository_repository.go`: PostgreSQL implementation.
- `backend/internal/git/postgres/onboarded_repository_repository_test.go`: integration tests for store behavior.
- `backend/internal/transport/rest/repository_onboarding_router.go`: REST handlers and JSON DTOs.
- `backend/internal/transport/rest/router.go`: wire service option and routes.
- `backend/internal/transport/rest/repository_onboarding_router_test.go`: REST tests.
- `backend/internal/transport/rest/openapi.yaml`: document new endpoints.
- `backend/cmd/agentguild-api/main.go`: construct repository onboarding service with the GitHub App manager and store.
- `frontend/src/api/client.ts`: repository onboarding types, client helpers, demo state.
- `frontend/src/features/repositories/RepositoryOnboardingScreen.tsx`: new console page.
- `frontend/src/features/repositories/RepositoryOnboardingScreen.test.tsx`: UI tests.
- `frontend/src/app/Rail.tsx`: add `仓库接入` rail item.
- `frontend/src/app/AppShell.tsx`: add route and module label.
- `frontend/src/features/sync/SyncRuleScreen.tsx`: remove public repository onboarding behavior and link to `/repositories`.
- `frontend/src/features/sync/SyncRuleScreen.test.tsx`: sync page regression for repository setup link.

---

### Task 1: Backend Repository Onboarding Service

**Files:**
- Create: `backend/internal/git/application/repository_onboarding.go`
- Create: `backend/internal/git/application/repository_onboarding_test.go`
- Modify: `backend/internal/git/application/contracts.go`

**Interfaces:**
- Produces:
  - `type OnboardedRepositoryView struct { ID, SourceType, FullName, DefaultBranch, Visibility string; CreatedAt, UpdatedAt time.Time }`
  - `type RepositoryOnboardingSummary struct { GitHubApp GitHubAppView; AppRepositories []RepositoryCandidateView; OnboardedRepositories []OnboardedRepositoryView; AppRepositoriesError string }`
  - `type RepositoryOnboardingService struct`
  - `func NewRepositoryOnboardingService(store OnboardedRepositoryStore, apps GitHubAppManager, public PublicRepositoryResolver, newID func() string) (*RepositoryOnboardingService, error)`
  - `func (s *RepositoryOnboardingService) Summary(ctx context.Context, principal Principal) (RepositoryOnboardingSummary, error)`
  - `func (s *RepositoryOnboardingService) AddGitHubAppRepository(ctx context.Context, principal Principal, fullName string) (OnboardedRepositoryView, error)`
  - `func (s *RepositoryOnboardingService) AddPublicRepository(ctx context.Context, principal Principal, input string) (OnboardedRepositoryView, error)`
  - `func (s *RepositoryOnboardingService) Remove(ctx context.Context, principal Principal, id string) error`
- Consumes:
  - Existing `GitHubAppManager.Get` and `GitHubAppManager.IssueSource`.
  - Existing `git.Repository` values from `ListInstallationRepositories`.

- [x] **Step 1: Write failing service constructor and validation tests**

Create tests covering nil dependencies and public repository normalization:

```go
func TestRepositoryOnboardingService_AddPublicRepositoryNormalizesGitHubURL(t *testing.T) {
	store := newMemoryOnboardedRepositoryStore()
	resolver := fakePublicRepositoryResolver{
		repo: git.Repository{FullName: "vercel/next.js", DefaultBranch: "canary", Visibility: "public"},
	}
	svc, err := NewRepositoryOnboardingService(store, fakeGitHubApps{}, resolver, func() string { return "repo-1" })
	require.NoError(t, err)

	view, err := svc.AddPublicRepository(context.Background(), adminPrincipal("tenant-1"), "https://github.com/vercel/next.js")

	require.NoError(t, err)
	require.Equal(t, "repo-1", view.ID)
	require.Equal(t, "public_github", view.SourceType)
	require.Equal(t, "vercel/next.js", view.FullName)
	require.Equal(t, "canary", view.DefaultBranch)
	require.Equal(t, "public", view.Visibility)
}
```

Run: `cd backend && go test ./internal/git/application -run RepositoryOnboarding -count=1`
Expected: FAIL because the service does not exist.

- [x] **Step 2: Implement service types and input normalization**

Implement source constants `github_app` and `public_github`, normalize `owner/repo` and GitHub URL inputs, and return `invalid("repo")` for invalid shapes such as `owner`, `/owner/repo`, or `https://example.com/owner/repo`.

- [x] **Step 3: Add App repository selection behavior**

Write a failing test that `AddGitHubAppRepository` checks current App installation repositories before persisting:

```go
func TestRepositoryOnboardingService_AddGitHubAppRepositoryRequiresVisibleRepo(t *testing.T) {
	svc, err := NewRepositoryOnboardingService(
		newMemoryOnboardedRepositoryStore(),
		fakeGitHubApps{repos: []git.Repository{{FullName: "acme/api", DefaultBranch: "main", Visibility: "private"}}},
		fakePublicRepositoryResolver{},
		func() string { return "repo-1" },
	)
	require.NoError(t, err)

	_, err = svc.AddGitHubAppRepository(context.Background(), adminPrincipal("tenant-1"), "acme/missing")

	require.Error(t, err)
	require.Equal(t, "not_found", domain.CodeOf(err))
}
```

Run: `cd backend && go test ./internal/git/application -run RepositoryOnboarding -count=1`
Expected: FAIL until App repository visibility is enforced.

- [x] **Step 4: Complete service tests**

Add tests for:
- `Summary` returns configured App state, candidate repositories, and onboarded inventory.
- `Summary` returns unconfigured App state with no candidate repositories when GitHub App is not configured.
- App repository listing errors are captured in `AppRepositoriesError` while preserving App configured state.
- `Remove` delegates tenant-scoped delete to the store.

Run: `cd backend && go test ./internal/git/application -run RepositoryOnboarding -count=1`
Expected: PASS.

- [x] **Step 5: Commit service layer**

```bash
git add backend/internal/git/application/repository_onboarding.go backend/internal/git/application/repository_onboarding_test.go backend/internal/git/application/contracts.go
git commit -m "feat: add repository onboarding service"
```

---

### Task 2: Persistence and Migration

**Files:**
- Create: `backend/migrations/000012_repository_onboarding.up.sql`
- Create: `backend/migrations/000012_repository_onboarding.down.sql`
- Modify: `backend/internal/testdb/postgres.go`
- Create: `backend/internal/git/postgres/onboarded_repository_repository.go`
- Create: `backend/internal/git/postgres/onboarded_repository_repository_test.go`

**Interfaces:**
- Consumes: `application.OnboardedRepository` and `application.OnboardedRepositoryStore`.
- Produces: `func NewOnboardedRepositoryRepository(pool queryer) application.OnboardedRepositoryStore`.

- [x] **Step 1: Write failing PostgreSQL repository tests**

Create integration tests that:
- insert a repository for `tenant-1`
- list only `tenant-1` repositories when `tenant-2` has a similarly named repository
- update an existing `(tenant_id, source_type, full_name)` record rather than duplicating it
- delete by `(tenant_id, id)`

Run: `cd backend && go test ./internal/git/postgres -run OnboardedRepository -count=1`
Expected: FAIL because migration and repository do not exist.

- [x] **Step 2: Add migration**

`000012_repository_onboarding.up.sql` should create:

```sql
CREATE TABLE onboarded_repositories (
    tenant_id text NOT NULL,
    id text NOT NULL,
    source_type text NOT NULL CHECK (source_type IN ('github_app', 'public_github')),
    full_name text NOT NULL,
    default_branch text NOT NULL,
    visibility text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, source_type, full_name)
);

CREATE INDEX onboarded_repositories_tenant_source_idx
    ON onboarded_repositories (tenant_id, source_type, full_name);
```

The down migration drops the index and table.

- [x] **Step 3: Register migration in testdb**

Add `000012_repository_onboarding.up.sql` after `000011` in `openAndMigrate` and `ApplyUpMigration`; add `000012_repository_onboarding.down.sql` before `000011` in `ApplyDownMigration`.

- [x] **Step 4: Implement PostgreSQL store**

Implement `Upsert`, `List`, and `Delete` with tenant filters on every query. Use `clock_timestamp()` on upsert updates to refresh `updated_at`.

- [x] **Step 5: Run persistence tests**

Run: `cd backend && go test ./internal/git/postgres -run OnboardedRepository -count=1`
Expected: PASS.

- [x] **Step 6: Commit persistence layer**

```bash
git add backend/migrations/000012_repository_onboarding.up.sql backend/migrations/000012_repository_onboarding.down.sql backend/internal/testdb/postgres.go backend/internal/git/postgres/onboarded_repository_repository.go backend/internal/git/postgres/onboarded_repository_repository_test.go
git commit -m "feat: persist onboarded repositories"
```

---

### Task 3: REST API and Server Wiring

**Files:**
- Create: `backend/internal/transport/rest/repository_onboarding_router.go`
- Create: `backend/internal/transport/rest/repository_onboarding_router_test.go`
- Modify: `backend/internal/transport/rest/router.go`
- Modify: `backend/internal/transport/rest/openapi.yaml`
- Modify: `backend/cmd/agentguild-api/main.go`

**Interfaces:**
- Consumes: `*gitapp.RepositoryOnboardingService`.
- Produces:
  - `GET /v1/repository-onboarding`
  - `POST /v1/repositories/github-app`
  - `POST /v1/repositories/public`
  - `DELETE /v1/repositories/{id}`
  - `rest.WithRepositoryOnboardingService(svc repositoryOnboardingService) Option`

- [x] **Step 1: Write failing REST tests**

Add route tests for:
- summary returns `github_app`, `app_repositories.items`, and `onboarded_repositories.items`
- public add requires admin session
- App add requires admin session
- delete requires admin session
- non-admin session receives forbidden for mutations

Run: `cd backend && go test ./internal/transport/rest -run RepositoryOnboarding -count=1`
Expected: FAIL because routes do not exist.

- [x] **Step 2: Add REST handler and DTOs**

Create JSON shapes:

```go
type repositoryOnboardingSummaryView struct {
	GitHubApp              gitapp.GitHubAppView `json:"github_app"`
	AppRepositories        repositoryItemsView  `json:"app_repositories"`
	OnboardedRepositories  repositoryItemsView  `json:"onboarded_repositories"`
	AppRepositoriesError   string               `json:"app_repositories_error,omitempty"`
}

type addRepositoryBody struct {
	Repo string `json:"repo"`
}
```

Use `writeEnvelope` for all success responses and `mapDomainError` or repository-specific field errors for failures.

- [x] **Step 3: Register routes and option**

Add `repositoryOnboarding repositoryOnboardingService` to `Server`, `WithRepositoryOnboardingService`, and routes under `/v1` when the service is present.

- [x] **Step 4: Wire main**

In `backend/cmd/agentguild-api/main.go`, construct the PostgreSQL store and repository onboarding service near existing GitHub App and sync service wiring, then pass `rest.WithRepositoryOnboardingService(...)`.

- [x] **Step 5: Update OpenAPI**

Add schemas for repository onboarding summary, repository item, add body, and the four endpoints. Document that mutation endpoints are human session/admin controlled.

- [x] **Step 6: Run REST tests**

Run: `cd backend && go test ./internal/transport/rest -run 'RepositoryOnboarding|GitHubApp' -count=1`
Expected: PASS.

- [x] **Step 7: Commit REST layer**

```bash
git add backend/internal/transport/rest/repository_onboarding_router.go backend/internal/transport/rest/repository_onboarding_router_test.go backend/internal/transport/rest/router.go backend/internal/transport/rest/openapi.yaml backend/cmd/agentguild-api/main.go
git commit -m "feat: expose repository onboarding API"
```

---

### Task 4: Frontend API Client and Repository Onboarding Screen

**Files:**
- Modify: `frontend/src/api/client.ts`
- Create: `frontend/src/features/repositories/RepositoryOnboardingScreen.tsx`
- Create: `frontend/src/features/repositories/RepositoryOnboardingScreen.test.tsx`

**Interfaces:**
- Produces frontend types:
  - `RepositorySourceType = "github_app" | "public_github"`
  - `RepositoryInventoryItem`
  - `RepositoryOnboardingSummary`
  - `getRepositoryOnboarding()`
  - `addGitHubAppRepository(repo: string)`
  - `addPublicRepository(repo: string)`
  - `removeRepository(id: string)`

- [x] **Step 1: Write failing API/demo tests through the screen**

Create a screen test in demo mode expectations:
- renders Step 1 GitHub App area
- renders Step 2 add repository area
- renders App repository candidates
- renders added repository list with both `GitHub App` and `公开仓库` source labels

Run: `cd frontend && npm test -- RepositoryOnboardingScreen --run`
Expected: FAIL because the screen does not exist.

- [x] **Step 2: Add API client types and demo state**

Extend `client.ts` demo data with:

```typescript
let demoOnboardedRepositories: RepositoryInventoryItem[] = [
  { id: "repo-inv-1", source_type: "github_app", full_name: "acme/billing-service", default_branch: "main", visibility: "private", created_at: "2026-07-09T08:00:00Z", updated_at: "2026-07-09T08:00:00Z" },
  { id: "repo-inv-2", source_type: "public_github", full_name: "vercel/next.js", default_branch: "canary", visibility: "public", created_at: "2026-07-09T08:10:00Z", updated_at: "2026-07-09T08:10:00Z" },
];
```

Handle demo paths for summary, App add, public add, and delete.

- [x] **Step 3: Build `RepositoryOnboardingScreen`**

Implement the two-step page:
- Step 1 card: GitHub App installed/uninstalled state and actions.
- Step 2 card: App repository candidate rows with add buttons and public repository input.
- Installed App summary card.
- Added repositories dense table/list with source chips and remove actions.

Use existing `PageHeader`, `Card`, `Button`, `ButtonLink`, `StatusChip`, and `DenseTable` patterns.

- [x] **Step 4: Complete screen tests**

Add tests for:
- clicking an App repository add button adds it to the added repository list
- submitting `https://github.com/rust-lang/rust` adds `rust-lang/rust`
- no sync rule creation helper is called by this screen

Run: `cd frontend && npm test -- RepositoryOnboardingScreen --run`
Expected: PASS.

- [x] **Step 5: Commit frontend onboarding screen**

```bash
git add frontend/src/api/client.ts frontend/src/features/repositories/RepositoryOnboardingScreen.tsx frontend/src/features/repositories/RepositoryOnboardingScreen.test.tsx
git commit -m "feat: add repository onboarding screen"
```

---

### Task 5: Navigation and Sync Page Integration

**Files:**
- Modify: `frontend/src/app/Rail.tsx`
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/features/onboarding/OnboardingScreen.tsx`
- Modify: `frontend/src/features/sync/SyncRuleScreen.tsx`
- Create or modify: `frontend/src/features/sync/SyncRuleScreen.test.tsx`

**Interfaces:**
- Consumes: `RepositoryOnboardingScreen`.
- Produces: authenticated route `/repositories` and rail label `仓库接入`.

- [x] **Step 1: Write failing navigation tests**

Add tests that render the app shell or relevant components and assert:
- `仓库接入` is present in persistent navigation
- `/repositories` renders the repository onboarding page
- onboarding overview's repository setup action points to `/repositories`

Run: `cd frontend && npm test -- --run`
Expected: FAIL until routes and links are wired.

- [x] **Step 2: Wire route and rail**

Add a `仓库接入` rail item with the lucide `FolderGit2` icon, add module label mapping, import `RepositoryOnboardingScreen`, and add `<Route path="/repositories" element={<RepositoryOnboardingScreen />} />`.

- [x] **Step 3: Update onboarding overview links**

Keep GitHub App installation accessible, but make repository setup actions route to `/repositories` so the user enters the two-step module.

- [x] **Step 4: Refocus sync rules page**

Remove the public repository form from `SyncRuleScreen`. Keep sync rule listing and rule actions. Add a compact card or `PageHeader` action linking to `/repositories` for repository setup.

- [x] **Step 5: Run frontend tests**

Run: `cd frontend && npm test -- --run`
Expected: PASS.

- [x] **Step 6: Commit navigation integration**

```bash
git add frontend/src/app/Rail.tsx frontend/src/app/AppShell.tsx frontend/src/features/onboarding/OnboardingScreen.tsx frontend/src/features/sync/SyncRuleScreen.tsx frontend/src/features/sync/SyncRuleScreen.test.tsx
git commit -m "feat: surface repository onboarding in console"
```

---

### Task 6: Final Verification and Documentation Sync

**Files:**
- Modify: `openspec/changes/repository-onboarding-center/tasks.md`
- Modify if implementation discovers wording drift: `openspec/changes/repository-onboarding-center/specs/**/*.md`

**Interfaces:**
- Consumes all completed tasks.
- Produces verification evidence for Comet build guard.

- [x] **Step 1: Run backend targeted tests**

Run:

```bash
cd backend && go test ./internal/git/application ./internal/git/postgres ./internal/transport/rest -count=1
```

Expected: PASS.

- [x] **Step 2: Run backend build**

Run:

```bash
cd backend && go build ./...
```

Expected: PASS.

- [x] **Step 3: Run frontend tests**

Run:

```bash
cd frontend && npm test -- --run
```

Expected: PASS.

- [x] **Step 4: Run frontend build**

Run:

```bash
cd frontend && npm run build
```

Expected: PASS.

- [x] **Step 5: Update OpenSpec task checkboxes**

Check off completed implementation tasks in `openspec/changes/repository-onboarding-center/tasks.md` only after the matching implementation and verification pass.

- [x] **Step 6: Commit verification/docs updates**

```bash
git add openspec/changes/repository-onboarding-center/tasks.md
git commit -m "chore: mark repository onboarding tasks complete"
```

---

### Task 7: Dokploy-style GitHub App Install Continuation

**Reason for increment:** Manual review against Dokploy's GitHub setup flow found the existing implementation only created the GitHub App manifest and did not complete the explicit install/authorize-repositories continuation inside this change.

**Files:**
- Modify: `backend/internal/git/application/contracts.go`
- Modify: `backend/internal/git/application/github_app.go`
- Modify: `backend/internal/git/application/github_app_test.go`
- Modify: `backend/internal/git/application/manifest.go`
- Modify: `backend/internal/git/application/manifest_test.go`
- Modify: `backend/internal/transport/rest/github_manifest_router.go`
- Modify: `backend/internal/transport/rest/github_manifest_router_test.go`
- Modify: `backend/internal/transport/rest/router.go`
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/features/git/GitIntegrationScreen.tsx`
- Create: `frontend/src/features/git/GitIntegrationScreen.test.tsx`
- Modify: `frontend/src/features/repositories/RepositoryOnboardingScreen.tsx`
- Modify: `frontend/src/features/repositories/RepositoryOnboardingScreen.test.tsx`
- Modify: `openspec/changes/repository-onboarding-center/specs/github-app-integration/spec.md`
- Modify: `openspec/changes/repository-onboarding-center/tasks.md`

- [x] **Step 1: Add failing backend tests for installation preservation**

Add tests proving that an installed callback records `installation_id` without clearing the manifest-created private key, App slug, or other saved App metadata.

- [x] **Step 2: Implement GitHub App installation persistence**

Add `GitHubAppManager.Install(ctx, tenantID, installationID)` and route `/oauth/github/app/installed` through it instead of rebuilding an incomplete `UpsertGitHubApp` command.

- [x] **Step 3: Add failing backend tests for install redirect**

Add tests proving `/oauth/github/app/install` redirects to GitHub's App installation picker with a signed state token.

- [x] **Step 4: Implement install redirect endpoint**

Add `ManifestService.BuildInstallURL` and wire `/oauth/github/app/install`.

- [x] **Step 5: Add failing frontend tests for created-but-not-installed state**

Cover both `/git-integration` and `/repositories` so admins can continue installation from either first-step surface.

- [x] **Step 6: Implement frontend install actions**

Expose `githubInstallUrl()`, show `已创建，待安装`, and provide `安装 GitHub App` buttons before showing installed connection checks.

- [x] **Step 7: Run targeted verification**

Run backend and frontend targeted tests covering the installation continuation path.

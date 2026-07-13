# Multi-GitHub-App Repository Onboarding Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow each tenant to manage multiple GitHub Apps and add a uniquely bound repository through an App-scoped searchable selector or a public repository URL.

**Architecture:** Give every GitHub App connection a tenant-scoped local ID and bind GitHub-App-backed inventory rows to that ID. Resolve GitHub drivers by `(tenant_id, repository)` for credentials, commit verification, and issue sync. Expose plural REST resources; the React UI loads all repositories for one selected App and filters them locally in an accessible combobox.

**Tech Stack:** Go 1.26.0, PostgreSQL 18.4, chi, pgx, React 19.1, TypeScript 5.8, Vite 7, Vitest, Testing Library, Playwright 1.54.

## Global Constraints

- Preserve tenant isolation on every App and repository query with composite `(tenant_id, id)` predicates.
- Never return or log private keys, webhook secrets, client secrets, installation tokens, or activation tokens.
- GitHub App repository endpoints return all repositories; the frontend performs case-insensitive local filtering and does not paginate.
- A tenant may onboard a `full_name` only once, regardless of source or App.
- A GitHub-App-backed repository binds to exactly one App; changing App requires remove then add.
- Reject deletion of an App while any repository references it.
- Preserve the singular `/v1/github-app` API as a deterministic compatibility layer over the tenant's `is_default` App.
- Follow red-green-refactor: no production change before its focused test has failed for the expected reason.

---

## File Structure

- `backend/migrations/000013_multi_github_apps.up.sql` / `.down.sql`: migrate App identity, default selection, installation account, repository binding, and uniqueness.
- `backend/internal/git/application/github_app.go`: multi-App commands, public views, driver construction, default compatibility.
- `backend/internal/git/application/repository_resolver.go`: one boundary that resolves an onboarded repository to its bound App driver/source.
- `backend/internal/git/application/manifest.go`: signed manifest/install state carries the local App connection ID.
- `backend/internal/git/github/installation.go`: fetch installation account metadata using App authentication.
- `backend/internal/git/postgres/github_app_repository.go`: tenant-scoped list/get/upsert/delete/default operations.
- `backend/internal/git/postgres/onboarded_repository_repository.go`: persist and resolve `github_app_id`.
- `backend/internal/transport/rest/github_apps_router.go`: plural App endpoints.
- `backend/internal/transport/rest/repository_onboarding_router.go`: App-scoped candidates and bound add request.
- `frontend/src/ui/SearchableSelect.tsx`: reusable accessible single-select combobox.
- `frontend/src/features/git/GitIntegrationScreen.tsx`: multi-App management list.
- `frontend/src/features/repositories/RepositoryOnboardingScreen.tsx`: unified source form and local repository search.

---

### Task 1: Migrate and Persist Multiple GitHub Apps

**Files:**
- Create: `backend/migrations/000013_multi_github_apps.up.sql`
- Create: `backend/migrations/000013_multi_github_apps.down.sql`
- Modify: `backend/internal/testdb/postgres.go:125-169`
- Modify: `backend/internal/git/application/ports.go:15-20`
- Modify: `backend/internal/git/postgres/github_app_repository.go`
- Modify: `backend/internal/git/postgres/onboarded_repository_repository.go`
- Test: `backend/internal/git/postgres/github_app_repository_test.go`
- Test: `backend/internal/git/postgres/onboarded_repository_repository_test.go`

**Interfaces:**
- Produces: `GitHubAppRepository.ListByTenant(ctx, tenantID)`, `GetByID(ctx, tenantID, id)`, `GetDefault(ctx, tenantID)`, `Upsert`, and `Delete(ctx, tenantID, id)`.
- Produces: `OnboardedRepositoryStore.GetByFullName(ctx, tenantID, fullName)` and records containing `GitHubAppID string`.

- [ ] **Step 1: Write failing PostgreSQL tests for multiple App rows and repository bindings**

```go
func TestGitHubAppRepositoryStoresTwoAppsForTenant(t *testing.T) {
    db := testdb.StartPostgres(t)
    repo := postgres.NewGitHubAppRepository(db)
    require.NoError(t, repo.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1", AppSlug: "alpha", IsDefault: true}))
    require.NoError(t, repo.Upsert(context.Background(), &application.GitHubAppRecord{ID: "gha-2", TenantID: "tenant-1", AppID: 22, PrivateKey: "key-2", AppSlug: "beta"}))

    apps, err := repo.ListByTenant(context.Background(), "tenant-1")
    require.NoError(t, err)
    require.Equal(t, []string{"gha-1", "gha-2"}, []string{apps[0].ID, apps[1].ID})
}

func TestOnboardedRepositoryPersistsGitHubAppBinding(t *testing.T) {
    db := testdb.StartPostgres(t)
    apps := postgres.NewGitHubAppRepository(db)
    require.NoError(t, apps.Upsert(context.Background(), &application.GitHubAppRecord{
        ID: "gha-1", TenantID: "tenant-1", AppID: 11, PrivateKey: "key-1", IsDefault: true,
    }))
    repos := postgres.NewOnboardedRepositoryRepository(db)
    require.NoError(t, repos.UpsertOnboardedRepository(context.Background(), &application.OnboardedRepositoryRecord{
        ID: "repo-1", TenantID: "tenant-1", SourceType: application.RepositorySourceGitHubApp,
        FullName: "acme/api", GitHubAppID: "gha-1", DefaultBranch: "main", Visibility: "private",
    }))
    got, err := repos.GetOnboardedRepositoryByFullName(context.Background(), "tenant-1", "acme/api")
    require.NoError(t, err)
    require.Equal(t, "gha-1", got.GitHubAppID)
}
```

- [ ] **Step 2: Run the focused tests and verify RED**

Run: `cd backend && go test ./internal/git/postgres -run 'TestGitHubAppRepositoryStoresTwoAppsForTenant|TestOnboardedRepositoryPersistsGitHubAppBinding' -count=1`

Expected: compile failure because `ID`, `IsDefault`, `ListByTenant`, and `GitHubAppID` do not exist.

- [ ] **Step 3: Add migration 000013 and register it in testdb**

```sql
ALTER TABLE github_apps ADD COLUMN id text;
UPDATE github_apps SET id = 'gha_' || md5(tenant_id || ':' || app_id::text);
ALTER TABLE github_apps ALTER COLUMN id SET NOT NULL;
ALTER TABLE github_apps ADD COLUMN installation_account_login text;
ALTER TABLE github_apps ADD COLUMN is_default boolean NOT NULL DEFAULT false;
UPDATE github_apps SET is_default = true;
ALTER TABLE github_apps DROP CONSTRAINT github_apps_pkey;
ALTER TABLE github_apps ADD PRIMARY KEY (tenant_id, id);
ALTER TABLE github_apps ADD CONSTRAINT github_apps_tenant_app_id_key UNIQUE (tenant_id, app_id);
CREATE UNIQUE INDEX github_apps_one_default_per_tenant
    ON github_apps (tenant_id) WHERE is_default;

ALTER TABLE onboarded_repositories ADD COLUMN github_app_id text;
UPDATE onboarded_repositories r
SET github_app_id = a.id
FROM github_apps a
WHERE r.tenant_id = a.tenant_id AND r.source_type = 'github_app';
ALTER TABLE onboarded_repositories
    ADD CONSTRAINT onboarded_repositories_github_app_fk
    FOREIGN KEY (tenant_id, github_app_id) REFERENCES github_apps (tenant_id, id) ON DELETE RESTRICT;
ALTER TABLE onboarded_repositories
    ADD CONSTRAINT onboarded_repositories_source_binding_check CHECK (
      (source_type = 'github_app' AND github_app_id IS NOT NULL) OR
      (source_type = 'public_github' AND github_app_id IS NULL)
    );
ALTER TABLE onboarded_repositories DROP CONSTRAINT onboarded_repositories_tenant_id_source_type_full_name_key;
ALTER TABLE onboarded_repositories ADD CONSTRAINT onboarded_repositories_tenant_full_name_key UNIQUE (tenant_id, full_name);
```

Add `000013_multi_github_apps.up.sql` after `000012` in `openAndMigrate` and `ApplyUpMigration`, and add its down migration before `000012` in `ApplyDownMigration`.

- [ ] **Step 4: Implement the repository contracts and SQL**

```go
type GitHubAppRepository interface {
    Upsert(context.Context, *GitHubAppRecord) error
    ListByTenant(context.Context, string) ([]GitHubAppRecord, error)
    GetByID(context.Context, string, string) (*GitHubAppRecord, error)
    GetDefault(context.Context, string) (*GitHubAppRecord, error)
    Delete(context.Context, string, string) error
}

type OnboardedRepositoryStore interface {
    ListOnboardedRepositories(context.Context, string) ([]OnboardedRepositoryRecord, error)
    GetOnboardedRepositoryByFullName(context.Context, string, string) (*OnboardedRepositoryRecord, error)
    UpsertOnboardedRepository(context.Context, *OnboardedRepositoryRecord) error
    DeleteOnboardedRepository(context.Context, string, string) error
}
```

Every SQL statement must scan/write `id`, `installation_account_login`, `is_default`, or `github_app_id` as applicable and use both tenant and local ID in predicates.

- [ ] **Step 5: Run focused and migration tests and verify GREEN**

Run: `cd backend && go test ./internal/git/postgres ./internal/testdb -count=1`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add backend/migrations/000013_multi_github_apps.* backend/internal/testdb/postgres.go backend/internal/git/application/ports.go backend/internal/git/postgres
git commit -m "feat: persist multiple github apps"
```

### Task 2: Implement Multi-App Application Services and Default Compatibility

**Files:**
- Modify: `backend/internal/git/application/contracts.go`
- Modify: `backend/internal/git/application/github_app.go`
- Modify: `backend/internal/git/application/github_app_test.go`

**Interfaces:**
- Consumes: Task 1 repository methods.
- Produces: `List`, `GetByID`, `Install(appID, installationID, accountLogin)`, `Delete(appID)`, `DriverForApp`, and `IssueSourceForApp`.
- Preserves: `Get`, `Driver`, and singular operations over the deterministic default App.

- [ ] **Step 1: Write failing service tests for list, tenant isolation, default selection, and guarded deletion**

```go
func TestGitHubAppManagerListsTenantAppsWithoutSecrets(t *testing.T) {
    manager := newGitHubAppManager(t)
    require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{ID: "gha-1", TenantID: "t1", AppID: 1, PrivateKey: key, AppSlug: "alpha"}))
    require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{ID: "gha-2", TenantID: "t1", AppID: 2, PrivateKey: key, AppSlug: "beta"}))
    views, err := manager.List(ctx, "t1")
    require.NoError(t, err)
    require.Equal(t, []string{"gha-1", "gha-2"}, []string{views[0].ID, views[1].ID})
}

func TestGitHubAppManagerRejectsDeletingBoundApp(t *testing.T) {
    err := manager.Delete(ctx, "t1", "gha-1")
    require.ErrorAs(t, err, new(*domain.Error))
    require.Equal(t, "conflict", domainErrorCode(err))
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd backend && go test ./internal/git/application -run 'TestGitHubAppManager' -count=1`

Expected: compile failure for the new multi-App methods.

- [ ] **Step 3: Implement the new records, views, and manager methods**

```go
type GitHubAppRecord struct {
    ID, TenantID, Provider string
    AppID, InstallationID int64
    PrivateKey, BaseURL string
    WebhookSecret, ClientID, ClientSecret string
    AppSlug, InstallationAccountLogin string
    IsDefault bool
    CreatedAt, UpdatedAt time.Time
}

type GitHubAppView struct {
    ID string `json:"id"`
    TenantID string `json:"tenant_id"`
    AppID int64 `json:"app_id"`
    InstallationID int64 `json:"installation_id"`
    AppSlug string `json:"app_slug,omitempty"`
    InstallationAccountLogin string `json:"installation_account_login,omitempty"`
    IsDefault bool `json:"is_default"`
    Configured bool `json:"configured"`
}
```

Generate an ID when `UpsertGitHubApp.ID` is empty. Mark the first App for a tenant as default. On deleting the default App, promote the earliest remaining App in the same repository transaction; reject deletion when the onboarded repository store reports a reference.

- [ ] **Step 4: Run application tests and verify GREEN**

Run: `cd backend && go test ./internal/git/application -run 'GitHubApp' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/git/application/contracts.go backend/internal/git/application/github_app.go backend/internal/git/application/github_app_test.go
git commit -m "feat: manage multiple github apps"
```

### Task 3: Correlate Manifest and Installation Flows to One App

**Files:**
- Create: `backend/internal/git/github/installation.go`
- Create: `backend/internal/git/github/installation_test.go`
- Modify: `backend/internal/git/application/manifest.go`
- Modify: `backend/internal/git/application/manifest_test.go`
- Modify: `backend/internal/transport/rest/github_manifest_router.go`
- Modify: `backend/internal/transport/rest/github_manifest_router_test.go`

**Interfaces:**
- Consumes: `GitHubAppManager.GetByID`, `Install`, and App credentials from Task 2.
- Produces: signed state `{tenant, github_app_id, exp}` and `InstallationAccount(ctx, installationID) (string, error)`.

- [ ] **Step 1: Write failing tests for two parallel manifest states and installation account lookup**

```go
func TestManifestStateKeepsGitHubAppID(t *testing.T) {
    state := service.SignState("tenant-1", "gha-2")
    decoded, err := service.VerifyState(state, "tenant-1")
    require.NoError(t, err)
    require.Equal(t, "gha-2", decoded.GitHubAppID)
}

func TestInstallationAccountReturnsOwnerLogin(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        require.Equal(t, "/app/installations/42", r.URL.Path)
        w.Header().Set("Content-Type", "application/json")
        _, _ = w.Write([]byte(`{"account":{"login":"acme-corp"}}`))
    }))
    defer server.Close()
    client, err := github.NewInstallationClient(github.InstallationClientConfig{
        BaseURL: server.URL, AppID: 1, PrivateKey: generateRSAPrivateKeyPEM(t), HTTPClient: server.Client(),
    })
    require.NoError(t, err)
    login, err := client.InstallationAccount(ctx, 42)
    require.NoError(t, err)
    require.Equal(t, "acme-corp", login)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `cd backend && go test ./internal/git/application ./internal/git/github ./internal/transport/rest -run 'ManifestStateKeeps|InstallationAccount|GitHubAppInstalled' -count=1`

Expected: failure because state has no App ID and installation metadata is not fetched.

- [ ] **Step 3: Implement App-scoped signed state and setup callback**

```go
type manifestState struct {
    TenantID string `json:"tenant"`
    GitHubAppID string `json:"github_app_id"`
    ExpiresAt time.Time `json:"exp"`
}
```

`BuildManifest` creates the pending local App ID and embeds it in state. `ExchangeCode` upserts credentials into that exact ID. `BuildInstallURL` and `/oauth/github/app/install` require `github_app_id`; `/installed` verifies the same ID, fetches the installation account login, and calls `Install(ctx, tenantID, appID, installationID, login)`.

The new GitHub metadata client has this constructor and method:

```go
type InstallationClientConfig struct {
    BaseURL string
    AppID int64
    PrivateKey string
    HTTPClient *http.Client
}

func NewInstallationClient(InstallationClientConfig) (*InstallationClient, error)
func (c *InstallationClient) InstallationAccount(context.Context, int64) (string, error)
```

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `cd backend && go test ./internal/git/application ./internal/git/github ./internal/transport/rest -run 'Manifest|Installation|GitHubAppInstall' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/git/github/installation* backend/internal/git/application/manifest* backend/internal/transport/rest/github_manifest_router*
git commit -m "feat: scope github install flow to app"
```

### Task 4: Bind Repository Onboarding to a Selected App

**Files:**
- Modify: `backend/internal/git/application/contracts.go`
- Modify: `backend/internal/git/application/repository_onboarding.go`
- Modify: `backend/internal/git/application/repository_onboarding_test.go`
- Modify: `backend/internal/transport/rest/repository_onboarding_router.go`
- Modify: `backend/internal/transport/rest/repository_onboarding_router_test.go`

**Interfaces:**
- Consumes: `IssueSourceForApp(ctx, tenantID, appID)` and App-bound store from Tasks 1–2.
- Produces: `ListGitHubAppRepositories(ctx, principal, appID)` and `AddGitHubAppRepository(ctx, principal, appID, fullName)`.

- [ ] **Step 1: Write failing application and REST tests**

```go
func TestAddGitHubAppRepositoryBindsSelectedApp(t *testing.T) {
    view, err := service.AddGitHubAppRepository(ctx, admin("tenant-1"), "gha-2", "acme/api")
    require.NoError(t, err)
    require.Equal(t, "gha-2", view.GitHubAppID)
}

func TestAddRepositoryRejectsDuplicateAcrossApps(t *testing.T) {
    _, _ = service.AddGitHubAppRepository(ctx, admin("tenant-1"), "gha-1", "acme/api")
    _, err := service.AddGitHubAppRepository(ctx, admin("tenant-1"), "gha-2", "acme/api")
    requireDomainCode(t, err, "conflict")
}
```

REST must assert `GET /v1/github-apps/gha-2/repositories` uses `gha-2`, and POST body `{ "github_app_id": "gha-2", "repo": "acme/api" }` returns that binding.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd backend && go test ./internal/git/application ./internal/transport/rest -run 'Repository|Repositories' -count=1`

Expected: signature/route failures because onboarding still assumes one App.

- [ ] **Step 3: Implement App-scoped candidates and add validation**

```go
type OnboardedRepositoryRecord struct {
    ID, TenantID, SourceType, FullName string
    GitHubAppID string
    DefaultBranch, Visibility string
    CreatedAt, UpdatedAt time.Time
}

func (s *RepositoryOnboardingService) AddGitHubAppRepository(
    ctx context.Context, principal Principal, appID, fullName string,
) (OnboardedRepositoryView, error)
```

Fetch the selected App's complete repository list, compare the normalized full name exactly, reject an existing tenant/full-name row, and persist `GitHubAppID`. Keep public repository binding empty.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `cd backend && go test ./internal/git/application ./internal/transport/rest -run 'RepositoryOnboarding|GitHubAppRepositories' -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/git/application/contracts.go backend/internal/git/application/repository_onboarding* backend/internal/transport/rest/repository_onboarding_router*
git commit -m "feat: bind repositories to github apps"
```

### Task 5: Resolve Runtime Git Operations by Repository Binding

**Files:**
- Create: `backend/internal/git/application/repository_resolver.go`
- Create: `backend/internal/git/application/repository_resolver_test.go`
- Modify: `backend/internal/git/application/contracts.go`
- Modify: `backend/internal/git/application/commands.go`
- Modify: `backend/internal/git/application/service_test.go`
- Modify: `backend/internal/git/application/commit_verifier.go`
- Modify: `backend/internal/git/application/commit_verifier_test.go`
- Modify: `backend/internal/sync/application/engine.go`
- Modify: `backend/internal/sync/application/engine_test.go`
- Modify: `backend/cmd/agentguild-api/main.go`

**Interfaces:**
- Produces: `RepositoryGitResolver.Driver(ctx, tenantID, fullName)` and `IssueSource(ctx, tenantID, fullName, sourceAuth)`.
- Consumes: App binding and `DriverForApp` from prior tasks.

- [ ] **Step 1: Write failing resolver and consumer tests with two recording drivers**

```go
func TestRepositoryGitResolverChoosesBoundApp(t *testing.T) {
    resolver := newResolver(map[string]string{"acme/api": "gha-2"}, map[string]git.Driver{"gha-1": driver1, "gha-2": driver2})
    got, err := resolver.Driver(ctx, "tenant-1", "acme/api")
    require.NoError(t, err)
    require.Same(t, driver2, got)
}

func TestCredentialAndVerifierUseRepositoryBoundDriver(t *testing.T) {
    resolver := &recordingRepositoryResolver{drivers: map[string]git.Driver{"acme/api": driver2}}
    credentialService := newCredentialServiceWithResolver(t, resolver)
    _, err := credentialService.IssueCredential(ctx, agentPrincipal("tenant-1"), application.IssueCredential{
        ExecutionID: "exec-1", Repo: "acme/api", BaseCommit: "base-sha",
    })
    require.NoError(t, err)
    require.Equal(t, []string{"tenant-1/acme/api"}, resolver.driverCalls)

    verifier := application.NewCommitVerifier(resolver, emptySubmissionRepository{})
    require.NoError(t, verifier.Verify(ctx, validVerifyCommit("tenant-1", "acme/api")))
    require.Equal(t, []string{"tenant-1/acme/api", "tenant-1/acme/api"}, resolver.driverCalls)
}
```

Add an engine test where `IssueSource(ctx, tenantID, rule.Repo, rule.SourceAuth)` records `rule.Repo`.

- [ ] **Step 2: Run focused tests and verify RED**

Run: `cd backend && go test ./internal/git/application ./internal/sync/application -run 'Bound|RepositoryGitResolver' -count=1`

Expected: failures because consumers resolve only by tenant.

- [ ] **Step 3: Implement and wire the resolver**

```go
type RepositoryGitResolver interface {
    Driver(ctx context.Context, tenantID, fullName string) (git.Driver, error)
    IssueSource(ctx context.Context, tenantID, fullName, sourceAuth string) (git.IssueSource, error)
}
```

For `github_app`, load the tenant/full-name inventory row and call `DriverForApp` with its binding. For `public_github`, allow only the existing public IssueSource path; Git credential issuance and private commit validation must return a stable invalid/conflict error rather than selecting an arbitrary App.

Change credential issuance and every `CommitVerifier` helper to pass repo into the resolver. Change sync `IssueSourceProvider` to accept repo and update `main.go` wiring.

- [ ] **Step 4: Run focused tests and verify GREEN**

Run: `cd backend && go test ./internal/git/application ./internal/sync/application ./cmd/agentguild-api -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/git/application backend/internal/sync/application backend/cmd/agentguild-api/main.go
git commit -m "feat: resolve git access by repository binding"
```

### Task 6: Expose Plural REST APIs and Preserve Singular Compatibility

**Files:**
- Create: `backend/internal/transport/rest/github_apps_router.go`
- Create: `backend/internal/transport/rest/github_apps_router_test.go`
- Modify: `backend/internal/transport/rest/github_app_router.go`
- Modify: `backend/internal/transport/rest/github_manifest_router.go`
- Modify: `backend/internal/transport/rest/router.go:90-95,362-385`
- Modify: `backend/internal/transport/rest/openapi.yaml`
- Test: `backend/internal/transport/rest/openapi_validation_test.go`

**Interfaces:**
- Consumes: Tasks 2–4 manager and onboarding methods.
- Produces: plural endpoints specified by the design.

- [ ] **Step 1: Write failing route and contract tests**

```go
func TestListGitHubAppsReturnsTenantApps(t *testing.T) {
    res := request(t, server, http.MethodGet, "/v1/github-apps", nil, adminSession("tenant-1"))
    require.Equal(t, http.StatusOK, res.Code)
    require.JSONEq(t, `{"data":{"items":[{"id":"gha-1","app_slug":"alpha","configured":true}]}}`, stripMeta(res.Body.Bytes()))
}

func TestDeleteBoundGitHubAppReturnsConflict(t *testing.T) {
    res := request(t, server, http.MethodDelete, "/v1/github-apps/gha-1", nil, adminSession("tenant-1"))
    require.Equal(t, http.StatusConflict, res.Code)
}
```

Also assert a foreign tenant gets 404, secrets never appear, and singular GET returns only `is_default`.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd backend && go test ./internal/transport/rest -run 'GitHubApps|GitHubAppRepositories|OpenAPI' -count=1`

Expected: 404 for plural routes and OpenAPI validation failure.

- [ ] **Step 3: Implement handlers, routes, envelopes, and OpenAPI schemas**

Register:

```go
r.Get("/github-apps", s.listGitHubApps)
r.Get("/github-apps/{id}", s.getGitHubAppByID)
r.Delete("/github-apps/{id}", s.deleteGitHubAppByID)
r.Post("/github-apps/{id}:test", s.testGitHubAppByID)
r.Get("/github-apps/{id}/repositories", s.listGitHubAppRepositories)
```

All handlers use `mustPrincipal(r).TenantID` plus `chi.URLParam(r, "id")`. Add `id`, `installation_account_login`, `is_default`, and repository `github_app_id` to OpenAPI response schemas. Keep singular handlers delegating to default methods.

- [ ] **Step 4: Run REST tests and verify GREEN**

Run: `cd backend && go test ./internal/transport/rest -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/transport/rest
git commit -m "feat: expose multi github app api"
```

### Task 7: Update Frontend API, Demo Mode, and Multi-App Management

**Files:**
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/features/git/GitIntegrationScreen.tsx`
- Modify: `frontend/src/features/git/GitIntegrationScreen.test.tsx`

**Interfaces:**
- Produces: `listGitHubApps`, `testGitHubApp(id)`, `deleteGitHubApp(id)`, `listGitHubAppRepositories(id)`, `githubInstallUrl(id)`.
- Produces demo data with two Apps and disjoint repository sets.

- [ ] **Step 1: Write failing component tests for two independent Apps**

```tsx
it("tests and deletes the selected GitHub App by id", async () => {
  vi.mocked(client.listGitHubApps).mockResolvedValue(envelope({ items: [alphaApp, betaApp] }));
  render(<GitIntegrationScreen />);
  await user.click(await screen.findByRole("button", { name: /检测 beta/ }));
  expect(client.testGitHubApp).toHaveBeenCalledWith("gha-beta");
});
```

Add assertions for `alpha · acme-corp`, pending-install state, and an App-bound delete conflict message.

- [ ] **Step 2: Run the test and verify RED**

Run: `cd frontend && npm test -- --run src/features/git/GitIntegrationScreen.test.tsx`

Expected: failure because the client and screen are singular.

- [ ] **Step 3: Implement types, client methods, demo routes, and App cards**

```ts
export type GitHubAppView = {
  id: string;
  app_id: number;
  installation_id?: number;
  app_slug: string;
  installation_account_login?: string;
  is_default: boolean;
  configured: boolean;
};

export const listGitHubApps = () => apiRequest<{ items: GitHubAppView[] }>("/v1/github-apps");
export const listGitHubAppRepositories = (id: string) =>
  apiRequest<{ items: Repository[] }>(`/v1/github-apps/${encodeURIComponent(id)}/repositories`);
```

Render one `ProviderCard` per App. Track testing/deleting state by App ID, navigate pending Apps with `githubInstallUrl(app.id)`, and keep “新增 GitHub App” mapped to the manifest URL.

- [ ] **Step 4: Run the focused frontend test and verify GREEN**

Run: `cd frontend && npm test -- --run src/features/git/GitIntegrationScreen.test.tsx`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/features/git
git commit -m "feat: manage multiple github apps in console"
```

### Task 8: Build the Searchable Repository Selector and Unified Add Form

**Files:**
- Create: `frontend/src/ui/SearchableSelect.tsx`
- Create: `frontend/src/ui/SearchableSelect.test.tsx`
- Modify: `frontend/src/ui/index.ts`
- Modify: `frontend/src/styles/components.css`
- Modify: `frontend/src/features/repositories/RepositoryOnboardingScreen.tsx`
- Modify: `frontend/src/features/repositories/RepositoryOnboardingScreen.test.tsx`

**Interfaces:**
- Consumes: Task 7 App/repository clients.
- Produces: `SearchableSelect<T>` controlled by `value`, `query`, `options`, `getOptionLabel`, and `onChange`.

- [ ] **Step 1: Write failing combobox and onboarding tests**

```tsx
it("filters repositories locally without another request", async () => {
  render(<RepositoryOnboardingScreen />);
  await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-beta");
  const input = await screen.findByRole("combobox", { name: "授权仓库" });
  await user.type(input, "WEB");
  expect(screen.getByRole("option", { name: "acme/web" })).toBeVisible();
  expect(screen.queryByRole("option", { name: "acme/api" })).not.toBeInTheDocument();
  expect(client.listGitHubAppRepositories).toHaveBeenCalledTimes(1);
});

it("adds the selected repository with its app id", async () => {
  vi.mocked(client.listGitHubApps).mockResolvedValue(envelope({ items: [betaApp] }));
  vi.mocked(client.listGitHubAppRepositories).mockResolvedValue(envelope({ items: [webRepo] }));
  render(<RepositoryOnboardingScreen />);
  await user.selectOptions(await screen.findByLabelText("GitHub App"), "gha-beta");
  await user.click(await screen.findByRole("combobox", { name: "授权仓库" }));
  await user.click(screen.getByRole("option", { name: "acme/web" }));
  await user.click(screen.getByRole("button", { name: "添加仓库" }));
  expect(client.addGitHubAppRepository).toHaveBeenCalledWith("gha-beta", "acme/web");
});
```

Add tests for App switching clearing query/selection, cached successful results, retry after failure, already-onboarded filtering, no Apps, no repositories, search no-match, and the unchanged public URL flow.

- [ ] **Step 2: Run tests and verify RED**

Run: `cd frontend && npm test -- --run src/ui/SearchableSelect.test.tsx src/features/repositories/RepositoryOnboardingScreen.test.tsx`

Expected: missing component and old candidate table behavior.

- [ ] **Step 3: Implement the accessible searchable select**

```tsx
export function SearchableSelect<T>({ label, options, value, query, onQueryChange, onChange, getOptionKey, getOptionLabel }: Props<T>) {
  return (
    <div className="searchable-select">
      <label className="field-label" htmlFor={`${label}-search`}>{label}</label>
      <input id={`${label}-search`} role="combobox" aria-expanded="true" aria-controls={`${label}-options`} value={query} onChange={(e) => onQueryChange(e.target.value)} />
      <ul id={`${label}-options`} role="listbox">
        {options.map((option) => (
          <li key={getOptionKey(option)} role="option" aria-selected={getOptionKey(option) === value} onMouseDown={() => onChange(option)}>
            {getOptionLabel(option)}
          </li>
        ))}
      </ul>
    </div>
  );
}
```

Use one highlighted option index and handle the complete keyboard contract in the input:

```tsx
onKeyDown={(event) => {
  if (event.key === "ArrowDown") {
    event.preventDefault();
    setOpen(true);
    setActiveIndex((index) => Math.min(index + 1, options.length - 1));
  } else if (event.key === "ArrowUp") {
    event.preventDefault();
    setActiveIndex((index) => Math.max(index - 1, 0));
  } else if (event.key === "Enter" && open && options[activeIndex]) {
    event.preventDefault();
    onChange(options[activeIndex]);
    setOpen(false);
  } else if (event.key === "Escape") {
    setOpen(false);
  }
}}
```

Set `aria-activedescendant` to the highlighted option ID, omit the listbox while closed, prevent opening while disabled, and render the supplied empty text in a non-option row.

- [ ] **Step 4: Implement the unified source form and per-App cache**

```ts
type RepositorySourceMode = "github_app" | "public_github";
const [sourceMode, setSourceMode] = useState<RepositorySourceMode>("github_app");
const [selectedAppID, setSelectedAppID] = useState("");
const [repositoriesByApp, setRepositoriesByApp] = useState<Record<string, Repository[]>>({});
const filtered = (repositoriesByApp[selectedAppID] ?? []).filter(
  (repo) => repo.full_name.toLowerCase().includes(repositoryQuery.trim().toLowerCase()) && !onboardedNames.has(repo.full_name),
);
```

Remove the candidate table. Render source controls, installed App selector, searchable repository selector, metadata preview, add button, public URL alternative, local load/retry errors, and the existing onboarded table.

- [ ] **Step 5: Run focused frontend tests and verify GREEN**

Run: `cd frontend && npm test -- --run src/ui/SearchableSelect.test.tsx src/features/repositories/RepositoryOnboardingScreen.test.tsx`

Expected: PASS with no React act or accessibility warnings.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/ui frontend/src/styles/components.css frontend/src/features/repositories
git commit -m "feat: add searchable repository selector"
```

### Task 9: Acceptance Coverage and Full Verification

**Files:**
- Modify: `frontend/e2e/agent-onboarding.spec.ts`
- Modify: `skill.md`
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: all prior tasks.
- Produces: end-to-end proof of two Apps, local search, App-bound add, and public URL add.

- [ ] **Step 1: Write the failing Playwright scenario**

```ts
test("selects an app, searches its repositories, and adds one", async ({ page }) => {
  await page.goto("/repository-onboarding");
  await page.getByLabel("GitHub App").selectOption("gha-beta");
  await page.getByRole("combobox", { name: "授权仓库" }).fill("web");
  await page.getByRole("option", { name: "acme/web" }).click();
  await page.getByRole("button", { name: "添加仓库" }).click();
  await expect(page.getByRole("table", { name: "已接入仓库列表" })).toContainText("acme/web");
});
```

- [ ] **Step 2: Run E2E and verify RED before demo fixtures are final**

Run: `cd frontend && npm run test:e2e -- e2e/agent-onboarding.spec.ts`

Expected: failure until the new demo interaction and selectors are wired end to end.

- [ ] **Step 3: Finish demo fixtures and documentation needed by the scenario**

Implement two explicit demo App fixtures and App-scoped repository lookup:

```ts
const demoGitHubApps: GitHubAppView[] = [
  { id: "gha-alpha", app_id: 101, installation_id: 1001, app_slug: "alpha", installation_account_login: "acme-corp", is_default: true, configured: true },
  { id: "gha-beta", app_id: 202, installation_id: 2002, app_slug: "beta", installation_account_login: "acme-labs", is_default: false, configured: true },
];
const demoRepositoriesByApp: Record<string, Repository[]> = {
  "gha-alpha": [{ full_name: "acme/api", default_branch: "main", visibility: "private" }],
  "gha-beta": [{ full_name: "acme/web", default_branch: "main", visibility: "private" }],
};
```

The demo POST checks `demoOnboardedRepositories.some((item) => item.full_name === fullName)` before insert and throws `repository already onboarded`. The demo DELETE App route throws `github app has onboarded repositories` when any item has the same `github_app_id`.

- [ ] **Step 4: Run format, build, unit, race, and E2E verification**

Run:

```bash
cd backend && gofmt -w internal/git internal/sync internal/transport/rest cmd/agentguild-api
cd backend && go build ./...
cd backend && go test -race ./... -count=1
cd frontend && npm run build
cd frontend && npm test -- --run
cd frontend && npm run test:e2e -- e2e/agent-onboarding.spec.ts
```

Expected: every command exits 0 with no warnings introduced by this change.

- [ ] **Step 5: Run repository verification**

Run: `scripts/comet-verify.sh`

Expected: exit 0.

- [ ] **Step 6: Commit final acceptance updates**

```bash
git add frontend/e2e/agent-onboarding.spec.ts frontend/src/api/client.ts skill.md AGENTS.md
git commit -m "test: cover multi app repository onboarding"
```

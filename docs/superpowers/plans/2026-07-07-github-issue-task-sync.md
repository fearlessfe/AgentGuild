---
change: github-issue-task-sync
design-doc: docs/superpowers/specs/2026-07-07-github-issue-task-sync-design.md
base-ref: 380e213eba528594f5929c1ce638bfb6959a6264
---

# GitHub Issue → Task 同步 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让平台把匹配的 GitHub Issue 周期性同步成 system 来源的 Task，并修复人类 session 读共享接口的权限缺口，前端接线真实后端。

**Architecture:** 后端沿用现有 chi REST + `application.Service` + `runWorker/repeat` worker 模式，`git/github` driver 扩展只读 Issue/Repo 能力并抽象为 `IssueSource` 接口（测试用 stub）；新增 `sync_rules` / `issue_task_map` 两表与 `github_apps` 增列承载 Manifest 一键接入；Issue 任务用固定 system publisher 常量创建，复用 `domain.NewTask` 与 `IntentCancel`。前端 git/sync 页与任务中心接线真实接口。

**Tech Stack:** Go 1.x（chi、pgx v5、golang-jwt）、PostgreSQL 18.4、React + TypeScript + Vite、React Router。

## Global Constraints

- 上游权威规格为 OpenSpec delta spec；本计划实现 HOW，WHAT 以 `openspec/changes/github-issue-task-sync/specs/**/spec.md` 为准。
- 沿用现有 envelope / error 形状（`{data, meta}`、`domain.Error{Code,Message,Field}`），不改既有路由 request/response 形状，仅新增路由。
- 私钥永不回显（沿用 `GitHubAppView` 不含 `private_key`）。
- 迁移文件命名连续递增，下一个序号从 `000010` 开始，每个迁移必须提供 `.up.sql` 与 `.down.sql`；新迁移文件名必须同时登记进 `backend/internal/testdb/postgres.go` 的 `openAndMigrate`、`ApplyUpMigration`、`ApplyDownMigration` 三处列表。
- 数据库时间统一用 `clock_timestamp()`（沿用现有表）。
- GitHub App 权限固定为：Contents 只读、Issues 读写、Checks 只读、Metadata 只读。
- 所有自动化测试（单测/集成）一律走 stub `IssueSource`，绝不调用真实 GitHub；真实 GitHub App 只用于第 8 组手动冒烟。
- 后端命令：构建 `cd backend && go build ./...`；测试 `cd backend && go test -race ./... -count=1`。数据库集成测试需要本地 Postgres（`make db-up`）或 docker，测试通过 `internal/testdb.StartPostgres` 自建隔离 schema。
- 前端：默认 `VITE_DEMO_MODE` 未设或非 `"true"` 时走真实后端（经 `/api` 代理）；`VITE_DEMO_MODE=true` 用内置 fixtures。
- Issue 任务的 system publisher 标识常量：`domain` 包新增 `SystemIssuePublisherID = "system-issue-sync"`，用作 `publisher_agent_version_id`。
- 人类 session principal 不携带 `Scopes`（见 `session_middleware.go`），因此 ScopePolicy 的 human 分支不得依赖 scope 匹配放行共享只读。

---

## 关键既有代码事实（实现前必读）

- `auth.ScopePolicy.Require`（`backend/internal/auth/principal.go:32`）当前无条件要求 `tenant_id`+`agent_id`+`agent_version_id` 非空且 `scopes` 命中。
- `auth.Principal`（`principal.go:9`）字段：`TenantID, Type, OwnerID, OwnerEmail, AgentID, AgentVersionID, Scopes, RepoScope, IsAdmin`；常量 `PrincipalTypeHuman="human"`、`PrincipalTypeAgent="agent"`。
- `review/application/policy.go` 用**自己的** `requireScope`（`policy.go:119`）已按 `principal.Type == PrincipalTypeHuman` 分支，**不调用** `ScopePolicy.Require` 处理人类，故放宽 `ScopePolicy.Require` 的 human 分支不会误放行评审写入（D1 复用点确认）。
- 读路径 `Service.ListTasks/GetTask`（`task_queries.go:31,61`）调用 `s.policy.Require(principal, "tasks:read")`；`requireLiveAgent`（`service.go:72`）对非 Agent principal 直接放行。
- `domain.NewTask(id, tenantID, publisherID, deadline)`（`domain/task.go:60`）要求四者非空、deadline 非零，初始状态 `TaskOpen`。`IntentCancel`（`task.go:134`）要求 `actor.Type==ActorPublisher && actor.ID==t.PublisherID`，允许从 open/claimed/in_progress → cancelled，且要求 `now.Before(deadline)`。
- `TaskRecord`（`application/ports.go:47`）字段：`ID, TenantID, PublisherAgentVersionID, Type, Title, Problem, Constraints []byte, Requirements []byte, Deadline, Status, ClaimedBy, StateVersion, ActiveExecutionID, CreatedAt, UpdatedAt`。`TaskRepository`（`ports.go:65`）：`InsertTask, GetTask, ListTaskRecords, UpdateTask(rec, expectVersion, activeExecID)(bool,error), ClaimTask`。
- `git.Driver`（`git/driver.go:40`）：`CreateCredential, GetCommit, CompareCommits, IsAncestor`。`github.Driver`（`git/github/driver.go`）持有 `cfg Config, key, client, now`，有 `installationToken(ctx)`、`get(ctx, token, url)(body,status,retryAfter,err)`、`apiURL(path, args...)`、`splitRepo`、`mapError`。
- `gitapp.GitHubAppManager`（`git/application/github_app.go`）有 `Upsert/Get/Delete/Driver(ctx,tenant)`；`GitHubAppRecord` 字段 `TenantID, Provider, AppID, InstallationID, PrivateKey, BaseURL, CreatedAt, UpdatedAt`。`GitHubAppRepository`（`git/application/ports.go:16`）：`Upsert, GetByTenant, Delete`。postgres 实现在 `git/postgres/github_app_repository.go`；`git.ErrGitHubAppNotConfigured` 为未配置错误。
- REST 路由在 `rest/router.go` 的 `Router()`；`WithGitHubAppManager` 已挂载 `/v1/github-app` CRUD（`github_app_router.go`）。中间件：`requireSession`（human-only）、`authenticateHumanOrAgent`（共享读）、`authenticate`（agent bearer）。人类 session principal 由 `session_middleware.go:22` 构造，带 `IsAdmin`。
- worker 在 `cmd/agentguild-api/main.go`：`runWorker(workerCtx,&wg,interval,name,fn)`；`buildGitRuntime` 返回 `gitAppManager`；`config.Config` 用 `duration(get,KEY,default)` 加载间隔。
- 迁移无生产 runner；`testdb.openAndMigrate` 顺序读取 `backend/migrations/*.up.sql`。本地手动 `psql -f` 应用。
- 前端路由 `frontend/src/app/AppShell.tsx`：`/git-integration`→`GitIntegrationScreen`、`/sync`→`SyncRuleScreen`、`/sync-result`→`SyncResultScreen`、`/tasks`→`Workbench`（含 `TaskList`+`TaskDetail`）。API 客户端 `frontend/src/api/client.ts`（`apiRequest`、`listTasks`、`demo(...)` fixtures）。`ApiNote status="planned"` 用于 `GitIntegrationScreen.tsx:40`、`SyncRuleScreen.tsx:68`、`SyncResultScreen.tsx:86`。

---

## 1. 前置：修复 ScopePolicy 对人类 principal 的读判定（TDD）

### Task 1.1: ScopePolicy 按 principal 类型分支

**Files:**
- Modify: `backend/internal/auth/principal.go:32-46`
- Test: `backend/internal/auth/principal_test.go`（新建）

**Interfaces:**
- Consumes: `auth.Principal`（既有字段）、`domain.Error`、`domain.ErrForbidden`。
- Produces: `func (ScopePolicy) Require(principal Principal, scope string) error` —— 行为变更：human principal 只要求 `TenantID` 非空即放行；agent principal 保持原校验。签名不变，供 `application.Service.policy.Require` 与其它调用方复用。

- [x] **Task 1.1 Step 1: 写失败测试**

创建 `backend/internal/auth/principal_test.go`：

```go
package auth_test

import (
	"testing"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestScopePolicyHumanReadOnlyPasses(t *testing.T) {
	p := auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeHuman}
	if err := (auth.ScopePolicy{}).Require(p, "tasks:read"); err != nil {
		t.Fatalf("human read should pass, got %v", err)
	}
}

func TestScopePolicyHumanMissingTenantRejected(t *testing.T) {
	p := auth.Principal{Type: auth.PrincipalTypeHuman}
	err := (auth.ScopePolicy{}).Require(p, "tasks:read")
	if err == nil {
		t.Fatal("human without tenant_id should be rejected")
	}
}

func TestScopePolicyAgentMissingFieldsRejected(t *testing.T) {
	p := auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeAgent, Scopes: []string{"tasks:read"}}
	err := (auth.ScopePolicy{}).Require(p, "tasks:read")
	var de *domain.Error
	if err == nil {
		t.Fatal("agent with empty agent_id should be rejected")
	}
	if !domainErrorField(err, &de, "agent_id") {
		t.Fatalf("expected agent_id field error, got %v", err)
	}
}

func TestScopePolicyAgentAuthorizedUnchanged(t *testing.T) {
	p := auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeAgent, AgentID: "a", AgentVersionID: "v", Scopes: []string{"tasks:read"}}
	if err := (auth.ScopePolicy{}).Require(p, "tasks:read"); err != nil {
		t.Fatalf("authorized agent should pass, got %v", err)
	}
}

func TestScopePolicyAgentScopeMissingForbidden(t *testing.T) {
	p := auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeAgent, AgentID: "a", AgentVersionID: "v", Scopes: []string{"tasks:publish"}}
	if err := (auth.ScopePolicy{}).Require(p, "tasks:read"); err != domain.ErrForbidden {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func domainErrorField(err error, out **domain.Error, field string) bool {
	if de, ok := err.(*domain.Error); ok {
		*out = de
		return de.Field == field
	}
	return false
}
```

- [x] **Task 1.1 Step 2: 运行测试确认失败**

Run: `cd backend && go test ./internal/auth/ -run TestScopePolicy -v`
Expected: FAIL —— human 分支测试因当前无条件要求 agent_id 而报 `agent_id is invalid`。

- [x] **Task 1.1 Step 3: 写最小实现**

替换 `backend/internal/auth/principal.go` 的 `Require`：

```go
func (ScopePolicy) Require(principal Principal, scope string) error {
	if principal.TenantID == "" {
		return &domain.Error{Code: "invalid_argument", Message: "tenant_id is invalid", Field: "tenant_id"}
	}
	if principal.Type == PrincipalTypeHuman {
		// 人类 session 天然不携带 agent_id/agent_version_id/scopes；
		// 共享只读接口不得因缺少这些字段而拒绝。写入接口的拒绝由路由中间件
		// （requireSession vs authenticate）保证，不在此放宽。
		return nil
	}
	for field, value := range map[string]string{
		"agent_id": principal.AgentID, "agent_version_id": principal.AgentVersionID,
	} {
		if value == "" {
			return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
		}
	}
	for _, candidate := range principal.Scopes {
		if candidate == scope {
			return nil
		}
	}
	return domain.ErrForbidden
}
```

- [x] **Task 1.1 Step 4: 运行测试确认通过**

Run: `cd backend && go test ./internal/auth/ -run TestScopePolicy -v`
Expected: PASS（全部 5 个用例）。

- [x] **Task 1.1 Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/auth/principal.go backend/internal/auth/principal_test.go
git commit -m "fix(auth): scope policy branches by principal type for human read"
```

### Task 1.2: 回归 —— review 复用点不受影响 + 人类读放行、写仍拒

**Files:**
- Test: `backend/internal/application/task_queries_test.go`（新建或追加）
- 参考（只读，不改）：`backend/internal/review/application/policy.go`

**Interfaces:**
- Consumes: `application.NewService`、`auth.Principal{Type:PrincipalTypeHuman}`、`testdb.StartPostgres`（若已有 service 级测试基座则复用其构造方式）。
- Produces: 回归证据 —— human `ListTasks` 放行；human `PublishTask`/`CancelTask` 因 `requireLiveAgent`/policy 语义不被本次改动放宽。

- [x] **Task 1.2 Step 1: 写测试（human 读放行 + review requireScope 不误放行）**

追加到 `backend/internal/application/task_queries_test.go`（若无则新建，包 `application_test`，复用 `service_test.go` 中的 store 构造辅助；若 `service_test.go` 用内存 store helper `newTestService(t)`，直接复用；否则用 `testdb.StartPostgres` + `postgres.NewStore`）：

```go
func TestListTasksAllowsHumanPrincipal(t *testing.T) {
	svc := newTestService(t) // 复用 service_test.go 的构造 helper
	human := auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeHuman}
	_, err := svc.ListTasks(context.Background(), human, application.ListTasks{Limit: 20})
	if err != nil {
		t.Fatalf("human ListTasks should pass, got %v", err)
	}
}
```

同时在 `backend/internal/review/application/policy_test.go`（若存在则追加，否则新建）验证 review 的 human 分支仍要求 scope：

```go
func TestReviewRequireScopeHumanStillNeedsScope(t *testing.T) {
	p := auth.Principal{TenantID: "t", Type: auth.PrincipalTypeHuman} // 无 reviews:read
	if err := (application.Policy{}).CanViewSubmission(p); err == nil {
		t.Fatal("human without reviews:read should still be forbidden")
	}
}
```

> 说明：`newTestService` 若不存在，改为读取 `service_test.go` 顶部的构造函数名并复用；不要新造 store。

- [x] **Task 1.2 Step 2: 运行确认（应直接通过，因 review policy 未改）**

Run: `cd backend && go test -race ./internal/application/ ./internal/review/application/ -run 'TestListTasksAllowsHumanPrincipal|TestReviewRequireScope' -v`
Expected: PASS。若 human ListTasks 仍失败，说明 Task 1.1 未生效，回到 1.1。

- [x] **Task 1.2 Step 3: 路由级回归 —— 人类 `POST /v1/tasks` 仍 401**

在 `backend/internal/transport/rest/` 找到既有路由测试（如 `router_test.go`）；确认存在「session cookie 无法访问 agent-only 写路由」用例，若无则追加：human session（cookie）请求 `POST /v1/tasks` 走 `authenticate` 中间件（bearer-only），无 bearer → `401 UNAUTHORIZED`。

```go
func TestPublishTaskRejectsSessionOnly(t *testing.T) {
	// 构造带 session cookie、无 Authorization 的 POST /v1/tasks 请求
	// 期望 resp.StatusCode == http.StatusUnauthorized
}
```

Run: `cd backend && go test -race ./internal/transport/rest/ -run TestPublishTaskRejectsSessionOnly -v`
Expected: PASS。

- [x] **Task 1.2 Step 4: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/application/task_queries_test.go backend/internal/review/application/policy_test.go backend/internal/transport/rest/
git commit -m "test(auth): regression for human read allow and write still 401"
```

---

## 2. GitHub 客户端能力扩展 + IssueSource 抽象 + stub（TDD）

### Task 2.1: 定义 IssueSource 抽象与 DTO

**Files:**
- Create: `backend/internal/git/issuesource.go`

**Interfaces:**
- Produces:
  - `type Repository struct { FullName string; DefaultBranch string; Visibility string }`
  - `type Issue struct { Number int; Title string; Body string; State string; Labels []string; UpdatedAt time.Time; HTMLURL string }`
  - `type IssueFilter struct { State string; Labels []string }` （State 为 "open"/"closed"/"all"）
  - `type IssueSource interface { ListInstallationRepositories(ctx context.Context) ([]Repository, error); ListIssues(ctx context.Context, repo string, filter IssueFilter, since time.Time) ([]Issue, error) }`
  - 后续 driver（Task 2.2）与 stub（Task 2.4）都实现此接口；`GitHubAppManager` 需能返回 `IssueSource`（Task 2.3）。

- [x] **Task 2.1 Step 1: 写接口与 DTO（无逻辑，先让包编译）**

创建 `backend/internal/git/issuesource.go`：

```go
package git

import (
	"context"
	"time"
)

// Repository is a repository visible to a GitHub App installation.
type Repository struct {
	FullName      string
	DefaultBranch string
	Visibility    string
}

// Issue is a GitHub issue projected to the fields the sync engine needs.
type Issue struct {
	Number    int
	Title     string
	Body      string
	State     string // "open" | "closed"
	Labels    []string
	UpdatedAt time.Time
	HTMLURL   string
}

// IssueFilter selects which issues to list.
type IssueFilter struct {
	State  string // "open" | "closed" | "all"
	Labels []string
}

// IssueSource reads repositories and issues for a tenant's installation.
type IssueSource interface {
	ListInstallationRepositories(ctx context.Context) ([]Repository, error)
	ListIssues(ctx context.Context, repo string, filter IssueFilter, since time.Time) ([]Issue, error)
}
```

- [x] **Task 2.1 Step 2: 编译确认**

Run: `cd backend && go build ./internal/git/`
Expected: PASS。

- [x] **Task 2.1 Step 3: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/git/issuesource.go
git commit -m "feat(git): add IssueSource abstraction and DTOs"
```

### Task 2.2: github.Driver 实现 ListInstallationRepositories + ListIssues（TDD，stub http）

**Files:**
- Create: `backend/internal/git/github/issues.go`
- Test: `backend/internal/git/github/issues_test.go`

**Interfaces:**
- Consumes: 既有 `Driver.installationToken(ctx)`、`Driver.get(ctx, token, url)`、`Driver.apiURL(path, args...)`、`splitRepo`、`mapError`（同包私有，直接调用）。
- Produces: `func (d *Driver) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error)` 与 `func (d *Driver) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error)`。使 `*github.Driver` 满足 `git.IssueSource`。

关键 API 细节：
- 列安装仓库：`GET /installation/repositories?per_page=100&page=N`，响应 `{ "repositories": [ { "full_name", "default_branch", "visibility"/"private" } ] }`，需按 `page` 分页直到返回空。
- 列 Issue：`GET /repos/{owner}/{repo}/issues?state={state}&labels={csv}&since={RFC3339}&per_page=100&page=N`；过滤掉带 `pull_request` 字段的项（PR 也走 issues 端点）；label CSV 用逗号连接（GitHub 语义为 AND，含标签用它，排除标签在应用层过滤）；`state` 空时默认 "open"；`since` 零值时不带该参数。

- [x] **Task 2.2 Step 1: 写失败测试（stub http.Client）**

创建 `backend/internal/git/github/issues_test.go`：

```go
package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

func newStubDriver(t *testing.T, handler http.HandlerFunc) *Driver {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	d, err := NewDriver(Config{
		AppID:          1,
		InstallationID: 2,
		BaseURL:        srv.URL,
		PrivateKey:     testRSAKeyPEM(t), // 复用同包既有测试私钥 helper；若无则用 driver_test.go 中的常量
	}, WithHTTPClient(srv.Client()), WithClock(func() time.Time { return time.Unix(1700000000, 0) }))
	if err != nil {
		t.Fatalf("driver: %v", err)
	}
	return d
}

func TestListIssuesPaginatesAndFiltersPRs(t *testing.T) {
	page := 0
	d := newStubDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/2/access_tokens" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"tok","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		if r.URL.Query().Get("since") == "" {
			t.Fatalf("expected since param")
		}
		page++
		switch page {
		case 1:
			_, _ = w.Write([]byte(`[{"number":1,"title":"a","state":"open","labels":[{"name":"agent-ready"}],"updated_at":"2026-07-01T00:00:00Z","html_url":"u1"},{"number":2,"title":"pr","state":"open","pull_request":{"url":"x"}}]`))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	})
	issues, err := d.ListIssues(context.Background(), "owner/repo", git.IssueFilter{State: "open", Labels: []string{"agent-ready"}}, time.Unix(1690000000, 0))
	if err != nil {
		t.Fatalf("list issues: %v", err)
	}
	if len(issues) != 1 || issues[0].Number != 1 || issues[0].Labels[0] != "agent-ready" {
		t.Fatalf("expected 1 non-PR issue, got %+v", issues)
	}
}

func TestListInstallationRepositoriesPaginates(t *testing.T) {
	page := 0
	d := newStubDriver(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/app/installations/2/access_tokens" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"token":"tok","expires_at":"2030-01-01T00:00:00Z"}`))
			return
		}
		page++
		if page == 1 {
			_, _ = w.Write([]byte(`{"repositories":[{"full_name":"owner/repo","default_branch":"main","visibility":"private"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"repositories":[]}`))
	})
	repos, err := d.ListInstallationRepositories(context.Background())
	if err != nil {
		t.Fatalf("list repos: %v", err)
	}
	if len(repos) != 1 || repos[0].FullName != "owner/repo" || repos[0].DefaultBranch != "main" {
		t.Fatalf("unexpected repos %+v", repos)
	}
}
```

> 若同包已有测试私钥 helper（查看 `backend/internal/git/github/driver_test.go`），复用其名字替换 `testRSAKeyPEM`。

- [x] **Task 2.2 Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/git/github/ -run 'TestListIssues|TestListInstallationRepositories' -v`
Expected: FAIL（方法未定义）。

- [x] **Task 2.2 Step 3: 写实现**

创建 `backend/internal/git/github/issues.go`：

```go
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

const issuesPerPage = 100

type repoPayload struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Visibility    string `json:"visibility"`
	Private       bool   `json:"private"`
}

type issuePayload struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	State       string `json:"state"`
	UpdatedAt   string `json:"updated_at"`
	HTMLURL     string `json:"html_url"`
	Labels      []struct{ Name string `json:"name"` } `json:"labels"`
	PullRequest *struct{ URL string `json:"url"` } `json:"pull_request,omitempty"`
}

func (d *Driver) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error) {
	token, _, err := d.installationToken(ctx)
	if err != nil {
		return nil, err
	}
	var out []git.Repository
	for page := 1; ; page++ {
		u := d.apiURL("/installation/repositories?per_page=%d&page=%d", issuesPerPage, page)
		body, status, retryAfter, err := d.get(ctx, token, u)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, mapError(status, body, retryAfter)
		}
		var payload struct {
			Repositories []repoPayload `json:"repositories"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode repositories: %w", err)
		}
		if len(payload.Repositories) == 0 {
			break
		}
		for _, r := range payload.Repositories {
			out = append(out, git.Repository{FullName: r.FullName, DefaultBranch: r.DefaultBranch, Visibility: visibility(r)})
		}
		if len(payload.Repositories) < issuesPerPage {
			break
		}
	}
	return out, nil
}

func (d *Driver) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return nil, err
	}
	token, _, err := d.installationToken(ctx)
	if err != nil {
		return nil, err
	}
	state := filter.State
	if state == "" {
		state = "open"
	}
	var out []git.Issue
	for page := 1; ; page++ {
		q := url.Values{}
		q.Set("state", state)
		q.Set("per_page", fmt.Sprintf("%d", issuesPerPage))
		q.Set("page", fmt.Sprintf("%d", page))
		if len(filter.Labels) > 0 {
			q.Set("labels", strings.Join(filter.Labels, ","))
		}
		if !since.IsZero() {
			q.Set("since", since.UTC().Format(time.RFC3339))
		}
		u := d.apiURL("/repos/%s/%s/issues?%s", owner, name, q.Encode())
		body, status, retryAfter, err := d.get(ctx, token, u)
		if err != nil {
			return nil, err
		}
		if status != http.StatusOK {
			return nil, mapError(status, body, retryAfter)
		}
		var payload []issuePayload
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("decode issues: %w", err)
		}
		if len(payload) == 0 {
			break
		}
		for _, it := range payload {
			if it.PullRequest != nil {
				continue // issues 端点也返回 PR，跳过
			}
			labels := make([]string, 0, len(it.Labels))
			for _, l := range it.Labels {
				labels = append(labels, l.Name)
			}
			updated, _ := time.Parse(time.RFC3339, it.UpdatedAt)
			out = append(out, git.Issue{Number: it.Number, Title: it.Title, Body: it.Body, State: it.State, Labels: labels, UpdatedAt: updated, HTMLURL: it.HTMLURL})
		}
		if len(payload) < issuesPerPage {
			break
		}
	}
	return out, nil
}

func visibility(r repoPayload) string {
	if r.Visibility != "" {
		return r.Visibility
	}
	if r.Private {
		return "private"
	}
	return "public"
}

var _ git.IssueSource = (*Driver)(nil)
```

- [x] **Task 2.2 Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/git/github/ -run 'TestListIssues|TestListInstallationRepositories' -v`
Expected: PASS。

- [x] **Task 2.2 Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/git/github/issues.go backend/internal/git/github/issues_test.go
git commit -m "feat(github): list installation repositories and issues"
```

### Task 2.3: GitHubAppManager 暴露 IssueSource(tenant)

**Files:**
- Modify: `backend/internal/git/application/github_app.go`（`GitHubAppManager` 接口 + `gitHubAppService`）
- 查看接口定义：`backend/internal/git/application/*.go` 中 `GitHubAppManager` interface（含 `Driver` 方法）。

**Interfaces:**
- Produces: 在 `GitHubAppManager` 接口新增 `IssueSource(ctx context.Context, tenantID string) (git.IssueSource, error)`；`gitHubAppService.IssueSource` 复用 `Driver(ctx,tenant)` 返回的 `*github.Driver`（它已实现 `git.IssueSource`）。

- [x] **Task 2.3 Step 1: 找到并扩展接口**

在 `github_app.go`（或其接口定义处）为 `GitHubAppManager` 接口增加方法，并实现：

```go
// IssueSource returns a git.IssueSource for the tenant's configured installation.
func (s *gitHubAppService) IssueSource(ctx context.Context, tenantID string) (git.IssueSource, error) {
	driver, err := s.Driver(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	source, ok := driver.(git.IssueSource)
	if !ok {
		return nil, invalid("issue_source")
	}
	return source, nil
}
```

> `Driver` 已返回 `github.NewDriver(...)`，即 `*github.Driver`，实现 `git.IssueSource`，类型断言恒成立；断言失败分支仅为防御。

- [x] **Task 2.3 Step 2: 编译**

Run: `cd backend && go build ./internal/git/...`
Expected: PASS。

- [x] **Task 2.3 Step 3: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/git/application/github_app.go
git commit -m "feat(git): expose IssueSource from GitHubAppManager"
```

### Task 2.4: stub IssueSource（供后续 sync/REST 测试复用）

**Files:**
- Create: `backend/internal/git/gittest/stub_issue_source.go`

**Interfaces:**
- Produces: `type StubIssueSource struct { Repos []git.Repository; Issues map[string][]git.Issue; Err error; Calls int }`，实现 `git.IssueSource`；`ListIssues` 返回 `Issues[repo]`，支持注入 `Err` 模拟失败。供 Task 4.5、Task 5.x 测试注入。

- [x] **Task 2.4 Step 1: 写 stub**

创建 `backend/internal/git/gittest/stub_issue_source.go`：

```go
// Package gittest provides test doubles for git integrations.
package gittest

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

// StubIssueSource is an in-memory git.IssueSource for tests. It never calls GitHub.
type StubIssueSource struct {
	Repos  []git.Repository
	Issues map[string][]git.Issue
	Err    error
	Calls  int
}

func (s *StubIssueSource) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Repos, nil
}

func (s *StubIssueSource) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error) {
	s.Calls++
	if s.Err != nil {
		return nil, s.Err
	}
	return s.Issues[repo], nil
}

var _ git.IssueSource = (*StubIssueSource)(nil)
```

- [x] **Task 2.4 Step 2: 编译**

Run: `cd backend && go build ./internal/git/...`
Expected: PASS。

- [x] **Task 2.4 Step 3: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/git/gittest/stub_issue_source.go
git commit -m "test(git): add StubIssueSource test double"
```

---

## 3. 数据模型与迁移

### Task 3.1: 迁移 —— github_apps 增列 + sync_rules + issue_task_map（含 down）

**Files:**
- Create: `backend/migrations/000010_issue_task_sync.up.sql`
- Create: `backend/migrations/000010_issue_task_sync.down.sql`
- Modify: `backend/internal/testdb/postgres.go`（三处列表追加 `000010`）

**Interfaces:**
- Produces: 表结构供 Task 4.2（sync_rules repo）与 Task 5.2（issue_task_map repo）使用。`github_apps` 新列供 Task 4b.3 落库。
- 说明：D6 tasks.md 提到「同步频率」列与「每规则水位/游标」（tasks 3.1/3.3）。按 D6 最终决策「规则仅启/停，无每规则频率；单一全局 worker」，本迁移**不建独立频率列，也不建独立游标表**；改为在 `sync_rules` 保留 `last_synced_at` 作为每规则增量水位（满足 tasks 3.3 的 per-rule 游标意图，避免过度设计）。若审阅坚持独立列，可在此文件加 `poll_interval_seconds INT`，但 worker 不消费。

- [x] **Task 3.1 Step 1: 写 up 迁移**

创建 `backend/migrations/000010_issue_task_sync.up.sql`：

```sql
-- GitHub App: columns for Manifest one-click install flow (nullable, additive)
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS webhook_secret TEXT;
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS client_id TEXT;
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS client_secret TEXT;
ALTER TABLE github_apps ADD COLUMN IF NOT EXISTS app_slug TEXT;

-- Issue -> Task sync rules (one-tenant-many-rules)
CREATE TABLE sync_rules (
    id TEXT NOT NULL PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    repo TEXT NOT NULL,
    include_labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    exclude_labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    issue_state TEXT NOT NULL DEFAULT 'open',
    task_type TEXT NOT NULL,
    default_priority TEXT NOT NULL DEFAULT 'P2',
    dedupe_strategy TEXT NOT NULL DEFAULT 'update',
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT sync_rules_issue_state_valid CHECK (issue_state IN ('open','closed','all')),
    CONSTRAINT sync_rules_dedupe_valid CHECK (dedupe_strategy IN ('update','skip'))
);
CREATE INDEX sync_rules_tenant_enabled_idx ON sync_rules (tenant_id, enabled);

-- Issue <-> Task mapping / dedupe (unique per tenant+repo+issue)
CREATE TABLE issue_task_map (
    tenant_id TEXT NOT NULL,
    repo TEXT NOT NULL,
    issue_number INTEGER NOT NULL,
    task_id TEXT NOT NULL,
    issue_state TEXT NOT NULL,
    issue_closed BOOLEAN NOT NULL DEFAULT FALSE,
    issue_url TEXT,
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, repo, issue_number)
);
CREATE INDEX issue_task_map_task_idx ON issue_task_map (tenant_id, task_id);
```

- [x] **Task 3.1 Step 2: 写 down 迁移**

创建 `backend/migrations/000010_issue_task_sync.down.sql`：

```sql
DROP TABLE IF EXISTS issue_task_map;
DROP TABLE IF EXISTS sync_rules;
ALTER TABLE github_apps DROP COLUMN IF EXISTS app_slug;
ALTER TABLE github_apps DROP COLUMN IF EXISTS client_secret;
ALTER TABLE github_apps DROP COLUMN IF EXISTS client_id;
ALTER TABLE github_apps DROP COLUMN IF EXISTS webhook_secret;
```

- [x] **Task 3.1 Step 3: 登记进 testdb**

在 `backend/internal/testdb/postgres.go`：
- `openAndMigrate` 的 up 列表末尾追加 `applyMigration(t, db, "000010_issue_task_sync.up.sql")`。
- `ApplyUpMigration` 末尾追加同一行。
- `ApplyDownMigration` **开头**追加 `applyMigration(t, db, "000010_issue_task_sync.down.sql")`（down 顺序为倒序，最新的先降）。

- [x] **Task 3.1 Step 4: 验证迁移可应用（up/down 往返）**

Run: `cd backend && go test ./internal/git/postgres/ -run TestMigration -count=1` —— 若无迁移往返测试，改为编译并在本地 psql 手动应用：

```bash
make db-up
psql "postgres://agentguild:agentguild@127.0.0.1:5432/agentguild?sslmode=disable" -f backend/migrations/000010_issue_task_sync.up.sql
psql "postgres://agentguild:agentguild@127.0.0.1:5432/agentguild?sslmode=disable" -f backend/migrations/000010_issue_task_sync.down.sql
```
Expected: 两条 psql 均无错误。

- [x] **Task 3.1 Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/migrations/000010_issue_task_sync.up.sql backend/migrations/000010_issue_task_sync.down.sql backend/internal/testdb/postgres.go
git commit -m "feat(db): migration for sync_rules, issue_task_map, github_apps manifest columns"
```

---

## 4b. GitHub App 一键接入（Manifest）+ 连接检测

> 先做 4b（接入/凭证）再做 4（规则/仓库依赖已配置的 App），故编号顺序按依赖排：4b → 4。github_apps 增列已在 Task 3.1 完成。

### Task 4b.1: github_apps 仓储读写新列

**Files:**
- Modify: `backend/internal/git/application/github_app.go`（`GitHubAppRecord`、`UpsertGitHubApp` 增字段）
- Modify: `backend/internal/git/postgres/github_app_repository.go`（Upsert/GetByTenant SQL）
- Test: `backend/internal/git/postgres/repository_test.go`（追加往返用例）

**Interfaces:**
- Produces: `GitHubAppRecord` 新增 `WebhookSecret, ClientID, ClientSecret, AppSlug string`；`UpsertGitHubApp` 同样新增这些可选字段。`GitHubAppView` 不新增私密字段（仅可选 `AppSlug string \`json:"app_slug,omitempty"\``）。

- [x] **Step 1: 写失败测试（仓储往返含新列）**

在 `backend/internal/git/postgres/repository_test.go` 追加：

```go
func TestGitHubAppRepositoryPersistsManifestColumns(t *testing.T) {
	db := testdb.StartPostgres(t)
	repo := NewGitHubAppRepository(db)
	rec := &application.GitHubAppRecord{TenantID: "t1", Provider: "github", AppID: 10, InstallationID: 20, PrivateKey: "pk", BaseURL: "https://api.github.com", WebhookSecret: "ws", ClientID: "cid", ClientSecret: "cs", AppSlug: "my-app"}
	if err := repo.Upsert(context.Background(), rec); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.GetByTenant(context.Background(), "t1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.WebhookSecret != "ws" || got.ClientID != "cid" || got.AppSlug != "my-app" {
		t.Fatalf("manifest columns not persisted: %+v", got)
	}
}
```

- [x] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/git/postgres/ -run TestGitHubAppRepositoryPersistsManifestColumns -v`
Expected: FAIL（字段不存在 / 未 scan）。

- [x] **Step 3: 实现**

`github_app.go`：给 `GitHubAppRecord` 与 `UpsertGitHubApp` 增字段；`Upsert` 方法透传新字段（provider/baseURL 默认逻辑不变）；`toGitHubAppView` 增加 `AppSlug`。给 `GitHubAppView` 加 `AppSlug string \`json:"app_slug,omitempty"\``。

`github_app_repository.go` Upsert：

```go
_, err := r.q.Exec(ctx, `
    INSERT INTO github_apps (
        tenant_id, provider, app_id, installation_id, private_key, base_url,
        webhook_secret, client_id, client_secret, app_slug, created_at, updated_at
    ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, clock_timestamp(), clock_timestamp())
    ON CONFLICT (tenant_id) DO UPDATE SET
        provider = EXCLUDED.provider,
        app_id = EXCLUDED.app_id,
        installation_id = EXCLUDED.installation_id,
        private_key = EXCLUDED.private_key,
        base_url = EXCLUDED.base_url,
        webhook_secret = EXCLUDED.webhook_secret,
        client_id = EXCLUDED.client_id,
        client_secret = EXCLUDED.client_secret,
        app_slug = EXCLUDED.app_slug,
        updated_at = clock_timestamp()`,
    record.TenantID, record.Provider, record.AppID, record.InstallationID,
    record.PrivateKey, record.BaseURL,
    nullString(record.WebhookSecret), nullString(record.ClientID), nullString(record.ClientSecret), nullString(record.AppSlug),
)
```

GetByTenant SELECT 增列并 scan 到 `*string`/`sql.NullString` 再赋值（新列可空）：

```go
var webhookSecret, clientID, clientSecret, appSlug sql.NullString
err := r.q.QueryRow(ctx, `
    SELECT tenant_id, provider, app_id, installation_id, private_key, base_url,
           webhook_secret, client_id, client_secret, app_slug, created_at, updated_at
    FROM github_apps WHERE tenant_id = $1`, tenantID).Scan(
    &record.TenantID, &record.Provider, &record.AppID, &record.InstallationID,
    &record.PrivateKey, &record.BaseURL,
    &webhookSecret, &clientID, &clientSecret, &appSlug, &record.CreatedAt, &record.UpdatedAt,
)
// ... err handling
record.WebhookSecret = webhookSecret.String
record.ClientID = clientID.String
record.ClientSecret = clientSecret.String
record.AppSlug = appSlug.String
```

加辅助（同文件）：`func nullString(s string) any { if s == "" { return nil }; return s }`，并 import `database/sql`。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/git/postgres/ -run TestGitHubAppRepositoryPersistsManifestColumns -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/git/application/github_app.go backend/internal/git/postgres/github_app_repository.go backend/internal/git/postgres/repository_test.go
git commit -m "feat(git): persist github app manifest columns"
```

### Task 4b.2: Manifest 编排服务（生成 manifest + state + conversions 解析）（TDD，stub http）

**Files:**
- Create: `backend/internal/git/application/manifest.go`
- Test: `backend/internal/git/application/manifest_test.go`

**Interfaces:**
- Produces:
  - `type ManifestService struct { ... }`，`func NewManifestService(apps GitHubAppManager, opts ManifestOptions) *ManifestService`。
  - `type ManifestOptions struct { PublicBaseURL string; StateSecret []byte; HTTPClient *http.Client; Now func() time.Time }`（`PublicBaseURL` 用于回调/setup URL；`StateSecret` HMAC 签 state）。
  - `func (m *ManifestService) BuildManifest(tenantID string) (manifestJSON string, state string, redirectURL string, err error)` —— redirectURL 指向 `https://github.com/settings/apps/new`（GHE 时基于 BaseURL host 推导，本次 github.com 即可）。
  - `func (m *ManifestService) VerifyState(state, tenantID string) error`（HMAC + tenant 绑定 + 过期）。
  - `func (m *ManifestService) ExchangeCode(ctx context.Context, tenantID, code string) (GitHubAppView, error)` —— `POST {api}/app-manifests/{code}/conversions` 解析 `{id, slug, pem, client_id, client_secret, webhook_secret}` 并 `apps.Upsert(...)`（InstallationID 先置 0，待安装回调补写）。
- Consumes: `GitHubAppManager.Upsert`、`GitHubAppManager.Get`。

Manifest JSON 权限字段（写死）：`"default_permissions": {"contents":"read","issues":"write","checks":"read","metadata":"read"}`，`"url"` 指向平台首页，`"redirect_url"` = `{PublicBaseURL}/oauth/github/app/callback`，`"setup_url"` = `{PublicBaseURL}/oauth/github/app/installed`，`"public": false`。

- [x] **Step 1: 写失败测试**

创建 `backend/internal/git/application/manifest_test.go`，覆盖：
- `BuildManifest` 返回的 manifest JSON 含四项权限与两个回调 URL；state 非空。
- `VerifyState`：同 tenant + 未过期 → nil；篡改 tenant/签名 → 错误。
- `ExchangeCode`：用 httptest stub `POST /app-manifests/{code}/conversions` 返回 `{"id":123,"slug":"my-app","pem":"-----BEGIN...","client_id":"cid","client_secret":"cs","webhook_secret":"ws"}`；断言随后 `apps.Get` 返回 `AppID==123, AppSlug=="my-app"`，且 view 不含私钥字段。用一个内存假 `GitHubAppManager`（记录 Upsert 入参）。

```go
func TestManifestVerifyStateRejectsTampered(t *testing.T) { /* ... */ }
func TestManifestBuildContainsPermissionsAndCallbacks(t *testing.T) { /* ... */ }
func TestManifestExchangeCodePersistsCredentials(t *testing.T) { /* stub conversions, assert Upsert args */ }
```

- [x] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/git/application/ -run TestManifest -v`
Expected: FAIL。

- [x] **Step 3: 实现 manifest.go**

实现上述方法。state = `base64(payload).hmac`，payload 含 `{tenant, exp}`（复用类似 `task_queries.go` encodeCursor 的 HMAC 模式）。ExchangeCode 用 `opts.HTTPClient` POST conversions（`Accept: application/vnd.github+json`），解析后调 `apps.Upsert(ctx, UpsertGitHubApp{TenantID, AppID: id, InstallationID: 0, PrivateKey: pem, ClientID, ClientSecret, WebhookSecret, AppSlug: slug, BaseURL: derivedAPIBase})`。

> `UpsertGitHubApp` 现要求 `InstallationID != 0`（`github_app.go:72`）。为支持"先建 App 后安装"，在 `Upsert` 校验中放宽：允许 `InstallationID == 0` 落库（后续安装回调补写）。修改 `github_app.go` 的 `Upsert`：移除 `if cmd.InstallationID == 0 { return invalid(...) }`，改为不校验（迁移列本身 NOT NULL，故落库时 InstallationID 用 0 占位是合法的 BIGINT）。同步更新既有 `github_app_test.go` 对该校验的断言（若存在）。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/git/application/ -run 'TestManifest|TestGitHubApp' -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/git/application/manifest.go backend/internal/git/application/manifest_test.go backend/internal/git/application/github_app.go
git commit -m "feat(git): github app manifest orchestration service"
```

### Task 4b.3: REST —— manifest / callback / 安装回调 / 连接检测

**Files:**
- Create: `backend/internal/transport/rest/github_manifest_router.go`
- Modify: `backend/internal/transport/rest/router.go`（注册路由 + Server 增字段 + Option）
- Test: `backend/internal/transport/rest/github_manifest_router_test.go`

**Interfaces:**
- Consumes: `*gitapp.ManifestService`、`gitapp.GitHubAppManager`（连接检测调 `IssueSource(...).ListInstallationRepositories`）。
- Produces:
  - `GET /oauth/github/app/manifest`（session）：302 到 GitHub，或返回自动提交表单 HTML（含 manifest JSON + state）。
  - `GET /oauth/github/app/callback?code&state`（session）：VerifyState → ExchangeCode → 重定向前端 `/git-integration?connected=1`。
  - `GET /oauth/github/app/installed?installation_id&state`（session）：写 installation_id（调 manager.Get 再 Upsert 补 InstallationID）→ 重定向 `/git-integration?installed=1`。
  - `POST /v1/github-app:test`（session）：调 `IssueSource.ListInstallationRepositories` 一次，返回 `{data:{ok:bool, repo_count:int, error?:string}}`，不回显私钥。
  - `Server` 增 `manifest *gitapp.ManifestService` 字段与 `WithGitHubManifest(*gitapp.ManifestService) Option`。

- [x] **Step 1: 写失败测试**

`github_manifest_router_test.go` 覆盖：
- `GET /oauth/github/app/manifest` 带 session → 200/302，响应含 GitHub apps/new 目标与 state。
- `GET /oauth/github/app/callback` state 不符 → 400，且不落库（用假 manager 断言 Upsert 未被调用）。
- `POST /v1/github-app:test` 成功（stub IssueSource 返回 1 repo）→ `ok:true, repo_count:1`；失败（stub Err）→ `ok:false` + error，HTTP 200（结构化结果）。

- [x] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/transport/rest/ -run TestGitHubManifest -v`
Expected: FAIL（路由未注册）。

- [x] **Step 3: 实现 handler + 注册路由**

`github_manifest_router.go` 实现 4 个 handler；`:test` 用 `s.gitHubAppManager.IssueSource(ctx, principal.TenantID)`，`ListInstallationRepositories` 出错时把 `git.ErrGitHubAppNotConfigured` 映射为 `{ok:false, error:"github app not configured"}`。

`router.go`：在 `if s.oidc != nil` 附近新增（gate 于 `s.manifest != nil`）：
```go
if s.manifest != nil {
    r.With(s.requireSession).Get("/oauth/github/app/manifest", s.githubManifest)
    r.With(s.requireSession).Get("/oauth/github/app/callback", s.githubManifestCallback)
    r.With(s.requireSession).Get("/oauth/github/app/installed", s.githubAppInstalled)
}
```
在 `if s.gitHubAppManager != nil` 块内增加：
```go
r.With(s.requireSession, s.rateLimit).Post("/github-app:test", s.testGitHubApp)
```
新增 `WithGitHubManifest` Option 与 `Server.manifest` 字段。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/transport/rest/ -run TestGitHubManifest -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/transport/rest/github_manifest_router.go backend/internal/transport/rest/router.go backend/internal/transport/rest/github_manifest_router_test.go
git commit -m "feat(rest): github app manifest install flow and connection test"
```

---

## 4. 同步规则领域/应用/存储 + REST

### Task 4.1: sync_rules 领域模型与校验（TDD，纯逻辑）

**Files:**
- Create: `backend/internal/sync/domain/rule.go`
- Test: `backend/internal/sync/domain/rule_test.go`

**Interfaces:**
- Produces:
  - `type Rule struct { ID, TenantID, Repo string; IncludeLabels, ExcludeLabels []string; IssueState string; TaskType string; DefaultPriority string; DedupeStrategy string; Enabled bool; LastSyncedAt time.Time; CreatedAt, UpdatedAt time.Time }`
  - `func NewRule(id, tenantID, repo, taskType string, ...) (*Rule, error)`（校验 repo 为 `owner/name`、issue_state ∈ {open,closed,all}、dedupe ∈ {update,skip}、task_type 非空）
  - `func (r Rule) Matches(issue git.Issue) bool` —— include 命中（若配置）且不命中任何 exclude 且 state 满足。这是 Task 5.2 引擎复用点。
  - 常量 `DedupeUpdate="update"`, `DedupeSkip="skip"`。

- [x] **Step 1: 写失败测试**

`rule_test.go` 覆盖：合法 `NewRule` 成功；非法 repo（无斜杠）报错；非法 issue_state 报错；`Matches`：含标签命中、排除标签命中→false、include 为空视为不限制、closed 过滤。

- [x] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/sync/domain/ -v`
Expected: FAIL（包不存在）。

- [x] **Step 3: 实现 rule.go**

实现结构与校验、`Matches`（用 map 做 label 集合，O(n)）。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/sync/domain/ -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/sync/domain/
git commit -m "feat(sync): rule domain model, validation and issue matcher"
```

### Task 4.2: sync 存储（postgres）+ 规则应用服务 CRUD（TDD，集成）

**Files:**
- Create: `backend/internal/sync/application/service.go`（`RuleService`：CRUD + 启停）
- Create: `backend/internal/sync/application/ports.go`（`RuleRepository`、`MapRepository` 接口 + 记录类型）
- Create: `backend/internal/sync/postgres/rule_repository.go`
- Create: `backend/internal/sync/postgres/map_repository.go`
- Create: `backend/internal/sync/postgres/store.go`
- Test: `backend/internal/sync/postgres/rule_repository_test.go`

**Interfaces:**
- Produces:
  - `RuleRepository`：`Create(ctx, *Rule) error; Get(ctx, tenantID, id) (*Rule, error); List(ctx, tenantID) ([]*Rule, error); ListEnabledAllTenants(ctx) ([]*Rule, error); Update(ctx, *Rule) error; Delete(ctx, tenantID, id) error; TouchSynced(ctx, tenantID, id, t) error`
  - `MapRepository`：`Get(ctx, tenantID, repo string, issueNumber int) (*Mapping, error); Upsert(ctx, *Mapping) error`；`type Mapping struct { TenantID, Repo string; IssueNumber int; TaskID, IssueState string; IssueClosed bool; IssueURL string; LastSyncedAt time.Time }`
  - `RuleService`：`Create/List/Get/Update/Delete/SetEnabled`，均接受 `auth.Principal`，**写操作要求 `principal.IsAdmin`**（返回 `domain.ErrForbidden` 否则），读操作要求 human + 同 tenant。视图 `RuleView`（JSON tags：`id, repo, include_labels, exclude_labels, issue_state, task_type, default_priority, dedupe_strategy, enabled, last_synced_at, created_at, updated_at`）。
- Consumes: `sync/domain.Rule`、`auth.Principal`、`domain.Error`。

- [x] **Step 1: 写失败测试（仓储往返 + service admin 授权）**

`rule_repository_test.go`（用 `testdb.StartPostgres`）：Create→Get→List→Update(enabled=false)→Delete 往返；`ListEnabledAllTenants` 只返回 enabled。
service 单测（`service_test.go`，用内存 fake repo）：非 admin human `Create` → `ErrForbidden`；admin `Create` → 成功；agent principal `Create` → 拒绝（human-only）。

- [x] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/sync/... -v`
Expected: FAIL。

- [x] **Step 3: 实现 store/repo/service**

postgres repo 用 pgx；`include_labels`/`exclude_labels` 以 JSONB 存取（`json.Marshal`→`[]byte`）。`RuleService` ID 生成用现有随机 ID 方式（参考 `application.randomID` 或 `uuid`——查看仓库既有 helper，复用之）。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/sync/... -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/sync/application/ backend/internal/sync/postgres/
git commit -m "feat(sync): rule repository and CRUD service with admin authz"
```

### Task 4.3 + 4.4 + 4.5: REST —— /v1/repositories、/v1/sync-rules CRUD、:run（TDD）

> 三个路由合为一组（同一 handler 文件、同一鉴权模型、彼此依赖同步引擎），一次审查。`:run` 依赖 Task 5.2 的引擎；若按顺序实现，可先建 4.3/4.4，`:run` 的引擎调用留到 Task 5.2 完成后接线（本任务先写 handler 骨架 + 引擎接口占位，Task 5.2 填实现）。为避免占位，**建议将本组排在 Task 5.2 之后**执行；计划顺序如此假设。

**Files:**
- Create: `backend/internal/transport/rest/sync_router.go`
- Modify: `backend/internal/transport/rest/router.go`（注册 + Server 增 `syncRules` / `syncEngine` 字段 + Options）
- Test: `backend/internal/transport/rest/sync_router_test.go`

**Interfaces:**
- Consumes: `RuleService`（4.2）、`SyncEngine`（5.2，`RunRule(ctx, tenantID, ruleID) (SyncResult, error)`）、`GitHubAppManager.IssueSource`。
- Produces:
  - `GET /v1/repositories`（session）：`{data:{items:[{full_name,default_branch,visibility}]}}`；未配置 App → `{data:{items:[]}}` + 明确错误码或直接 `mapDomainError` 返回 not_configured（按 spec 返回明确未配置错误）。
  - `GET/POST/PUT/DELETE /v1/sync-rules`、`GET /v1/sync-rules/{id}`（session；写要求 admin）。
  - `POST /v1/sync-rules/{id}:run`（session, admin）：调 `SyncEngine.RunRule`，返回 `SyncResult` 摘要 `{created,updated,skipped,cancelled,failed}`。
  - `Server` 增字段 `syncRules *syncapp.RuleService`、`syncEngine SyncEngine`，Options `WithSyncRuleService`、`WithSyncEngine`。

- [x] **Step 1: 写失败测试**

`sync_router_test.go`：admin session `POST /v1/sync-rules` → 201 + view；非 admin human → 403；agent bearer → 403/401；`GET /v1/repositories`（stub IssueSource 1 repo）→ items 长度 1；`POST /v1/sync-rules/{id}:run`（stub 引擎）→ 摘要字段齐全。

- [x] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/transport/rest/ -run TestSync -v`
Expected: FAIL。

- [x] **Step 3: 实现 handler + 注册**

在 `router.go` `/v1` 块内 gate 于 `s.syncRules != nil` 注册上述路由（全部 `requireSession`；写路由在 handler 内检查 `principal.IsAdmin`，非 admin → `writeError(...,403,"FORBIDDEN",...)`）。`/v1/repositories` gate 于 `s.gitHubAppManager != nil`。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/transport/rest/ -run TestSync -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/transport/rest/sync_router.go backend/internal/transport/rest/router.go backend/internal/transport/rest/sync_router_test.go
git commit -m "feat(rest): repositories listing and sync-rule CRUD/run endpoints"
```

---

## 5. Issue→Task 同步引擎 + worker + 来源标识

### Task 5.1: Task 创建支持 system publisher（TDD）

**Files:**
- Modify: `backend/internal/domain/task.go`（新增 `SystemIssuePublisherID` 常量）
- Create: `backend/internal/application/system_task.go`（`Service` 新方法 `PublishSystemTask` / `CancelSystemTask`）
- Test: `backend/internal/application/system_task_test.go`

**Interfaces:**
- Produces:
  - `const SystemIssuePublisherID = "system-issue-sync"`（domain 包）。
  - `type PublishSystemTask struct { TenantID, RequestID, Type, Title, Problem string; Constraints, Requirements []string; Deadline time.Time }`
  - `func (s *Service) PublishSystemTask(ctx, cmd PublishSystemTask) (TaskView, error)` —— 直接构造 principal-free 路径：用 `domain.NewTask(newID, tenantID, domain.SystemIssuePublisherID, deadline)`，`InsertTask` 时 `PublisherAgentVersionID = SystemIssuePublisherID`，不经 `policy.Require`（system 内部调用）。含幂等（用 RequestID = `repo#number` 派生）。
  - `func (s *Service) CancelSystemTask(ctx, tenantID, taskID, reason string) (TaskView, error)` —— 加载 task，构造 `actor := domain.Actor{Type: domain.ActorPublisher, ID: domain.SystemIssuePublisherID}`，`task.Apply(domain.IntentCancel, actor, now)`（合法：publisher==system id）。
  - `func (s *Service) UpdateSystemTaskContent(ctx, tenantID, taskID string, title, problem string, constraints, requirements []string) error` —— 仅当当前 status ∈ {open, draft} 才更新内容字段（引擎侧也会判，双保险）。
- Consumes: 既有 `Service.store`, `Service.newID`, `TaskRepository`（`InsertTask/GetTask/UpdateTask`）。

- [ ] **Step 1: 写失败测试**

`system_task_test.go`（复用 `service_test.go` 的 `newTestService`）：
- `PublishSystemTask` 成功创建，`view.PublisherAgentVersionID == domain.SystemIssuePublisherID`，`Status == open`。
- `CancelSystemTask` 对 open 任务 → `cancelled`（无 forbidden，验证 D8 合法性）。
- `UpdateSystemTaskContent` 对 open 任务更新 title；对 claimed 任务（先人为置 claimed）不更新内容（返回 nil 但内容不变，或返回哨兵——按实现约定，测试断言内容未变）。

- [ ] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/application/ -run TestSystemTask -v`
Expected: FAIL。

- [x] **Step 3: 实现**

`domain/task.go` 加常量。`system_task.go` 实现三方法，复用 `task_commands.go` 中的 `TaskRecord` 构造、`appendEvents`（actor type=publisher、id=system id）、`UpdateTask(record, expectVersion, "")`。deadline 由调用方（引擎）传入（Task 5.2 用默认长期限）。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/application/ -run TestSystemTask -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/domain/task.go backend/internal/application/system_task.go backend/internal/application/system_task_test.go
git commit -m "feat(core): system publisher task create/cancel/update"
```

### Task 5.2: 同步引擎（拉取→过滤→映射→去重→更新/取消/对账）（TDD，stub IssueSource）

**Files:**
- Create: `backend/internal/sync/application/engine.go`
- Test: `backend/internal/sync/application/engine_test.go`

**Interfaces:**
- Consumes: `git.IssueSource`（stub 注入）、`RuleRepository`、`MapRepository`、以及 core task 端口抽象 `TaskSink`（见下，解耦 sync 与 core application 包，避免循环 import）。
  - `type TaskSink interface { PublishSystemTask(ctx, PublishSystemTaskInput) (taskID string, err error); CancelSystemTask(ctx, tenantID, taskID, reason string) error; UpdateSystemTaskContent(ctx, tenantID, taskID string, in ContentInput) (currentStatus string, err error); TaskStatus(ctx, tenantID, taskID string) (status string, err error) }`
  - 在 `cmd/agentguild-api/main.go` 用一个 adapter 把 `*application.Service` 适配为 `TaskSink`。
- Produces:
  - `type SyncResult struct { Created, Updated, Skipped, Cancelled, Failed, Flagged int; Errors []string }`
  - `type Engine struct { ... }`, `func NewEngine(rules RuleRepository, maps MapRepository, sink TaskSink, sources IssueSourceProvider, opts EngineOptions) *Engine`
  - `IssueSourceProvider`：`IssueSource(ctx, tenantID) (git.IssueSource, error)`（生产由 `GitHubAppManager` 满足；测试注入 stub）。
  - `type EngineOptions struct { DefaultDeadline time.Duration; Now func() time.Time }`（`DefaultDeadline` 即 D9 占位期限，默认如 `365*24h`）。
  - `func (e *Engine) RunRule(ctx, tenantID, ruleID string) (SyncResult, error)`
  - `func (e *Engine) RunAllEnabled(ctx) (SyncResult, error)` —— worker 用；遍历 `ListEnabledAllTenants`，单规则失败计入 `Failed`+`Errors` 但不中断（D6 容错）。

引擎 per-issue 逻辑（对账，D7/D8）：
1. 取 `issue`（源 State 一并读回）。规则 `Matches(issue)` 为假 → skip。
2. 查 `MapRepository.Get(tenant,repo,number)`：
   - 无映射 且 issue 为 open → `PublishSystemTask`（deadline=now+DefaultDeadline，type=rule.TaskType，title=issue.Title，problem=issue.Body），写 Mapping（issue_state=open, issue_closed=false, url）。`Created++`。
   - 无映射 且 issue 已 closed → 不建任务，skip（`Skipped++`）。
   - 有映射：读 `TaskSink.TaskStatus`：
     - issue 已 closed：task ∈ {open,draft} → `CancelSystemTask`（`Cancelled++`）；task 已推进 → 仅 `Mapping.IssueClosed=true` 元数据更新（`Flagged++`）。
     - issue open 且 dedupe=update 且 task ∈ {open,draft} → `UpdateSystemTaskContent`（`Updated++`）；否则元数据更新（`Skipped++`）。
   - 每次都刷新 `Mapping.LastSyncedAt`/`IssueState`，`Upsert`。
3. 规则跑完 `TouchSynced(rule)` 更新水位（下次以 `LastSyncedAt` 作为 `since`）。

- [x] **Step 1: 写失败测试（全部用 StubIssueSource + fake repos + fake sink）**

`engine_test.go` 覆盖 spec 场景：
- 匹配 open issue → Created，sink 收到 system task（type/title 正确）。
- 排除标签命中 → 不建任务。
- 去重：第二次同一 issue → 不重复创建（Created 不增，走 update/skip）。
- 仅 open/draft 更新：task 置 claimed 后再同步内容变化 → 内容不覆盖（Flagged/Skipped，非 Updated）。
- Issue closed + task open → Cancelled；closed + task claimed → Flagged（映射标记，不取消）。
- 单规则失败：stub `Err` 注入 → `RunAllEnabled` 该规则 Failed++ 且其它规则继续。
- `since` 水位：`TouchSynced` 后下次调用 `ListIssues` 收到的 `since` 为上次时间（用 stub 记录入参断言）。

- [x] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/sync/application/ -run TestEngine -v`
Expected: FAIL。

- [x] **Step 3: 实现 engine.go**

按上述逻辑实现；`Matches` 复用 `sync/domain.Rule.Matches`（排除标签在此额外判，因 GitHub `labels` 参数只做 include 的 AND）。

- [x] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/sync/application/ -run TestEngine -v`
Expected: PASS。

- [x] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/sync/application/engine.go backend/internal/sync/application/engine_test.go
git commit -m "feat(sync): issue-to-task engine with dedupe and close reconciliation"
```

### Task 5.3: sync worker 接线 main.go + 配置间隔

**Files:**
- Modify: `backend/internal/config/config.go`（新增 `SyncWorkerInterval time.Duration`、`SyncDefaultDeadline time.Duration`、`GitHubAppPublicBaseURL string`、`GitHubAppManifestStateSecret` 复用 SessionCookieSecret）
- Modify: `backend/cmd/agentguild-api/main.go`（构造 RuleService/MapRepo/Engine、TaskSink adapter、ManifestService、注册 REST options、`runWorker` 启动 sync）
- Test: `backend/internal/config/config_test.go`（追加默认值断言）

**Interfaces:**
- Consumes: 全部上游产物。
- Produces: 运行期 sync worker（`runWorker(workerCtx, &wg, cfg.SyncWorkerInterval, "sync", func(ctx) error { _, err := engine.RunAllEnabled(ctx); return err })`）；REST 挂载 `WithSyncRuleService`、`WithSyncEngine`、`WithGitHubManifest`。

- [ ] **Step 1: 写配置默认值测试**

`config_test.go` 追加：默认 `SyncWorkerInterval == 60s`（用 `duration(get,"SYNC_WORKER_INTERVAL",60*time.Second)`）、`SyncDefaultDeadline == 365*24h`（新 `duration` 项 `SYNC_DEFAULT_DEADLINE`）。

- [ ] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/config/ -run TestLoad -v`
Expected: FAIL（字段/默认未加）。

- [ ] **Step 3: 实现 config + main 接线**

config：加字段 + `duration(...)` 加载 + `GITHUB_APP_PUBLIC_BASE_URL`（`value(get, "GITHUB_APP_PUBLIC_BASE_URL", "")`，用于回调 URL；空时 manifest handler 用请求 Host 推导）。
main.go：在 `buildGitRuntime` 之后构造：
```go
ruleRepo := syncpostgres.NewRuleRepository(pool)
mapRepo := syncpostgres.NewMapRepository(pool)
ruleSvc := syncapp.NewRuleService(ruleRepo)
sink := application.NewSyncTaskSink(service) // adapter -> TaskSink
engine := syncapp.NewEngine(ruleRepo, mapRepo, sink, gitAppManager, syncapp.EngineOptions{DefaultDeadline: cfg.SyncDefaultDeadline})
manifestSvc := gitapp.NewManifestService(gitAppManager, gitapp.ManifestOptions{PublicBaseURL: cfg.GitHubAppPublicBaseURL, StateSecret: []byte(cfg.SessionCookieSecret)})
```
REST options 追加 `WithSyncRuleService(ruleSvc)`, `WithSyncEngine(engine)`, `WithGitHubManifest(manifestSvc)`（gate 于 `gitAppManager != nil` / web enabled）。
`runWorker(workerCtx, &wg, cfg.SyncWorkerInterval, "sync", func(ctx context.Context) error { _, err := engine.RunAllEnabled(ctx); return err })`（仅当 web/App 可用时启动）。
在 `application` 包新增 `NewSyncTaskSink(*Service) syncapp.TaskSink` 的适配器（放 `backend/internal/application/sync_sink.go`，或放 main.go 内联 adapter 以避免 core→sync 依赖倒置——优先 main.go 内联 adapter，保持 core 包不 import sync 包）。

- [ ] **Step 4: 运行确认通过 + 全量构建**

Run: `cd backend && go test ./internal/config/ -run TestLoad -v && go build ./...`
Expected: PASS + 构建通过。

- [ ] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/config/config.go backend/internal/config/config_test.go backend/cmd/agentguild-api/main.go
git commit -m "feat(app): wire sync worker, engine and manifest service"
```

### Task 5.5: Issue 来源标识可读（TaskView 关联映射）

**Files:**
- Modify: `backend/internal/application/contracts.go`（`TaskView` 增 `Source *TaskSource` 可选）
- Modify: `backend/internal/application/task_queries.go`（`ListTasks`/`GetTask` join 映射填充来源）
- Modify: `backend/internal/application/ports.go`（`TaskRepository` 或新增只读端口 `IssueSourceLookup`）
- Test: `backend/internal/application/task_queries_test.go`

**Interfaces:**
- Produces: `type TaskSource struct { Kind string \`json:"kind"\`; Repo string \`json:"repo,omitempty"\`; IssueNumber int \`json:"issue_number,omitempty"\`; IssueURL string \`json:"issue_url,omitempty"\` }`；`TaskView.Source *TaskSource \`json:"source,omitempty"\``。
- 实现取舍：为避免 core application 依赖 sync 包，在 core 定义只读端口 `type IssueSourceLookup interface { LookupByTaskIDs(ctx, tenantID string, taskIDs []string) (map[string]TaskSource, error) }`，由 sync/postgres 的 `map_repository` 实现并在 main.go 注入 `Service`（新增 `Service` 可选字段 + `WithIssueSourceLookup` option / setter）。若注入为 nil，`Source` 留空（向后兼容）。

- [ ] **Step 1: 写失败测试**

`task_queries_test.go`：注入一个 fake `IssueSourceLookup`（task→source），`ListTasks` 返回项的 `Source.Kind=="issue"`, `Repo`, `IssueNumber` 正确；无映射的任务 `Source == nil`。

- [ ] **Step 2: 运行确认失败**

Run: `cd backend && go test ./internal/application/ -run TestListTasksSource -v`
Expected: FAIL。

- [ ] **Step 3: 实现**

`ListTasks`/`GetTask` 在构造 views 后，若 `s.issueSource != nil`，批量 `LookupByTaskIDs` 并填 `Source`。

- [ ] **Step 4: 运行确认通过**

Run: `cd backend && go test ./internal/application/ -run TestListTasksSource -v`
Expected: PASS。

- [ ] **Step 5: 全量后端测试 + 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add backend/internal/application/ backend/internal/sync/postgres/ backend/cmd/agentguild-api/main.go
cd backend && go test -race ./... -count=1
cd /Users/pengzhen/work/AgentGuild
git commit -m "feat(core): expose issue source identity on task view"
```

Expected: 全量 `go test -race ./...` PASS。

---

## 6. 前端接线

### Task 6.1: API 客户端新增 sync/github 方法 + 类型

**Files:**
- Modify: `frontend/src/api/client.ts`（新增类型与请求函数；补 demo 分支）
- Test: `frontend/src/api/client.test.ts`（追加）

**Interfaces:**
- Produces（导出）：
  - types: `GitHubAppView`, `Repository`, `SyncRule`, `SyncRuleInput`, `SyncResult`, `ConnectionTestResult`；`TaskView` 增可选 `source?: { kind: string; repo?: string; issue_number?: number; issue_url?: string }`。
  - functions: `getGitHubApp()`, `deleteGitHubApp()`, `testGitHubApp()`, `listRepositories()`, `listSyncRules()`, `getSyncRule(id)`, `createSyncRule(input)`, `updateSyncRule(id, input)`, `deleteSyncRule(id)`, `runSyncRule(id)`；`githubManifestUrl()` 返回 `${base}/oauth/github/app/manifest`（用于 `window.location.href` 跳转）。

- [ ] **Step 1: 写失败测试**

`client.test.ts` 追加：mock fetch，`listSyncRules()` 命中 `/v1/sync-rules` GET；`createSyncRule` 发 POST body；`testGitHubApp` 命中 `/v1/github-app:test`。

- [ ] **Step 2: 运行确认失败**

Run: `cd frontend && npm test -- --run src/api/client.test.ts`
Expected: FAIL。

- [ ] **Step 3: 实现请求函数 + demo 分支**

在 `client.ts` 追加类型与函数（沿用 `apiRequest<T>`）。为 demo 模式在 `demo(...)` 内补 `/v1/sync-rules`、`/v1/repositories`、`/v1/github-app`、`/v1/github-app:test` 的 fixtures（返回合理示例），并给 `TaskView` demo 项加 `source`。`githubManifestUrl` 直接拼接 `base`。

- [ ] **Step 4: 运行确认通过**

Run: `cd frontend && npm test -- --run src/api/client.test.ts`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add frontend/src/api/client.ts frontend/src/api/client.test.ts
git commit -m "feat(web): api client methods for github app and sync rules"
```

### Task 6.2: GitIntegrationScreen 接线（一键连接 / 已配置态 / 连接检测 / 删除）

**Files:**
- Modify: `frontend/src/features/git/GitIntegrationScreen.tsx`

**Interfaces:**
- Consumes: `getGitHubApp`, `testGitHubApp`, `deleteGitHubApp`, `githubManifestUrl`（Task 6.1）。

- [ ] **Step 1: 实现**

改为函数组件用 `useEffect` 调 `getGitHubApp()` 渲染 configured/未配置态；「连接 GitHub」按钮 `onClick` → `window.location.href = githubManifestUrl()`；「检测连接」→ `testGitHubApp()` 渲染 ok/error；「删除」→ `deleteGitHubApp()` 后刷新。移除 `ApiNote status="planned"`（连接检测已实现），保留/更新 `status="available"` 说明。

- [ ] **Step 2: 构建 + 类型检查**

Run: `cd frontend && npm run build`
Expected: PASS（tsc + vite 通过）。

- [ ] **Step 3: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add frontend/src/features/git/GitIntegrationScreen.tsx
git commit -m "feat(web): wire git integration screen to real backend"
```

### Task 6.3: SyncRuleScreen + SyncResultScreen 接线

**Files:**
- Modify: `frontend/src/features/sync/SyncRuleScreen.tsx`
- Modify: `frontend/src/features/sync/SyncResultScreen.tsx`

**Interfaces:**
- Consumes: `listRepositories`, `listSyncRules`, `createSyncRule`, `updateSyncRule`, `deleteSyncRule`, `runSyncRule`。

- [ ] **Step 1: 实现 SyncRuleScreen**

`useEffect` 加载 `listRepositories()` 填「安装仓库」表；加载 `listSyncRules()` 渲染规则；表单收集字段调 `createSyncRule/updateSyncRule`；「启用/暂停」调 `updateSyncRule({enabled})`；「预览/立即同步」调 `runSyncRule(id)` 并把结果传给同步结果展示。移除 `ApiNote status="planned"`。

- [ ] **Step 2: 实现 SyncResultScreen**

接收 `runSyncRule` 返回的 `SyncResult`（或按 rule 展示最近一次结果摘要 metrics：created/updated/skipped/cancelled/failed）。移除 `ApiNote status="planned"`（或降级为 available 说明冲突行不提供直接编辑正式 Task 的入口——保留该行为提示）。

- [ ] **Step 3: 构建**

Run: `cd frontend && npm run build`
Expected: PASS。

- [ ] **Step 4: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add frontend/src/features/sync/SyncRuleScreen.tsx frontend/src/features/sync/SyncResultScreen.tsx
git commit -m "feat(web): wire sync rule and result screens"
```

### Task 6.4: 任务中心来源标识

**Files:**
- Modify: `frontend/src/features/tasks/TaskList.tsx` 与/或 `frontend/src/features/tasks/TaskDetail.tsx`（读取 `task.source` 渲染「来源 = Issue #N @ repo」链接）

**Interfaces:**
- Consumes: `TaskView.source`（Task 6.1 类型）。

- [ ] **Step 1: 实现**

在任务行/详情展示来源标识：若 `task.source?.kind === "issue"` 渲染 `Issue #{issue_number} @ {repo}`，`issue_url` 作为外链。

- [ ] **Step 2: 构建**

Run: `cd frontend && npm run build`
Expected: PASS。

- [ ] **Step 3: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add frontend/src/features/tasks/
git commit -m "feat(web): show issue source on task center"
```

### Task 6.5: frontend/.env.example + 默认真实后端确认

**Files:**
- Create: `frontend/.env.example`

**Interfaces:** 无代码接口，配置与文档。

- [ ] **Step 1: 写 .env.example**

创建 `frontend/.env.example`：

```dotenv
# 前端默认连接真实后端（经 /api 代理）。仅在需要离线演示时开启 demo。
# VITE_DEMO_MODE=true 时使用内置 fixtures，无需后端。
# VITE_DEMO_MODE=true

# 可选：直接指定后端基址（默认 /api，交给 Vite 代理转发到 :8080）。
# VITE_API_BASE_URL=/api

# 仅本地开发：直接注入 Agent bearer token（生产走 session cookie）。
# VITE_API_TOKEN=
```

- [ ] **Step 2: 确认默认走真实后端**

检查 `client.ts:110` `apiRequest` 仅在 `VITE_DEMO_MODE === "true"` 时走 demo；`.env.example` 默认注释掉该项 → 默认真实后端。无需改代码。

- [ ] **Step 3: 前端全量测试 + 提交**

```bash
cd frontend && npm run build && npm test -- --run
cd /Users/pengzhen/work/AgentGuild
git add frontend/.env.example
git commit -m "docs(web): add .env.example defaulting to real backend"
```

Expected: 前端构建与单测 PASS。

---

## 7. 本地运行脚手架

### Task 7.1: .local/env.sh 补同步与回调变量

**Files:**
- Modify: `/Users/pengzhen/work/AgentGuild/.local/env.sh`

- [ ] **Step 1: 追加变量**

在 `.local/env.sh` 末尾追加：

```bash
# Issue -> Task 同步
export SYNC_WORKER_INTERVAL="60s"
export SYNC_DEFAULT_DEADLINE="8760h"   # 365 天占位期限（D9）

# GitHub App Manifest 一键接入回调基址（需公网可达；本地可用隧道地址）
# 例：export GITHUB_APP_PUBLIC_BASE_URL="https://<your-tunnel>.trycloudflare.com"
export GITHUB_APP_PUBLIC_BASE_URL=""
```

- [ ] **Step 2: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add .local/env.sh
git commit -m "chore(local): env vars for sync interval and manifest callback"
```

### Task 7.2 + 7.3: 本地启动 + 冒烟文档（含隧道/降级两路径）

**Files:**
- Create: `docs/local-dev-github-issue-sync.md`

> 注意：全局规则禁止随意创建 .md。本文件是本 change 的必要交付物（tasks 7.2/7.3 要求文档化本地启动与冒烟两路径），且为 change 范围内文档，允许创建。

- [ ] **Step 1: 写文档**

创建 `docs/local-dev-github-issue-sync.md`，内容含：
1. 起 Postgres：`make db-up`。
2. 应用迁移：`for f in backend/migrations/0000*_*.up.sql; do psql "$DATABASE_URL" -f "$f"; done`（或按序执行；至少 `000010`）。
3. 加载环境：`source .local/env.sh`。
4. 起后端：`cd backend && go run ./cmd/agentguild-api`（监听 `:8080`）。
5. 起前端：`cd frontend && npm run dev`（Vite，`/api` 代理到 `:8080`）。
6. 本地登录：`POST /oauth/local/login`（密码 `LOCAL_ADMIN_PASSWORD`），拿 `agentguild_session` cookie。
7. 一键接入两路径：
   - **路径 A（隧道）**：`cloudflared tunnel --url http://localhost:8080`（或 `ngrok http 8080`），把公网地址设为 `GITHUB_APP_PUBLIC_BASE_URL`，重启后端；Git 接入页点「连接 GitHub」走 Manifest。
   - **路径 B（降级）**：无公网时用手动配置 `POST /v1/github-app`（App ID、Installation ID、私钥）；效果等价。
8. 触发同步：建规则 → `POST /v1/sync-rules/{id}:run` 或等 worker tick。

- [ ] **Step 2: 提交**

```bash
cd /Users/pengzhen/work/AgentGuild
git add docs/local-dev-github-issue-sync.md
git commit -m "docs: local dev and smoke guide for github issue sync"
```

---

## 8. 手动冒烟验证（真实 GitHub App）

> 非自动化任务，按 `docs/local-dev-github-issue-sync.md` 与设计文档 Smoke 小节逐条执行。全部通过后在 tasks.md 勾选并在 verify 阶段记录证据。

- [ ] **8.1** 本地起全栈（含隧道使回调可达），本地登录拿 `agentguild_session`；全程 `GET /v1/tasks` 等读接口无 `agent_id is invalid` 报错。
- [ ] **8.2** Git 接入页「连接 GitHub」→ Manifest 创建 App → 回调落库（`GET /v1/github-app` 显示 configured）→ 安装 App → `installation_id` 落库。
- [ ] **8.3** 「检测连接」返回 ok；`GET /v1/repositories` 列出安装仓库。
- [ ] **8.4** 建一条规则（某 repo、open、含 `agent-ready`、type=code），`:run` 返回结果摘要。
- [ ] **8.5** 任务中心出现由真实 Issue 生成的 Task，含来源标识（Issue #N @ repo）。
- [ ] **8.6** 关闭该 Issue，再同步 → 未领取任务被 cancel（状态 cancelled）。
- [ ] **8.7** 重复同步不产生重复任务（去重生效，Created 不增）。

---

## Self-Review

**Spec 覆盖对照：**
- agent-access-control（人类读放行 / agent 不变 / 人类写仍 401）→ Task 1.1、1.2。
- issue-task-sync：一键接入/回调/state 校验/安装回调/手动降级 → 4b.2、4b.3；连接检测 → 4b.3；安装仓库列举 → 4.3；同步规则管理（CRUD/启停/非 admin 拒写）→ 4.1、4.2、4.4；周期同步/匹配/排除/去重/仅未推进更新/已推进不覆盖/关闭取消/关闭已领取仅标记/单规则失败容错 → 5.2；立即同步 → 4.5；Issue 来源读取与标识 → 5.5、6.4。
- local-dev-bootstrap：一键启动/本地 App 配置/默认真实后端 + demo 降级 → 6.5、7.1、7.2、7.3。

**执行顺序说明：** 组 4（REST repositories/CRUD/:run）依赖组 5.2 的 `SyncEngine`，故实现顺序为 1 → 2 → 3 → 4b → 4.1/4.2 → 5.1/5.2 → 4.3/4.4/4.5 → 5.3/5.5 → 6 → 7 → 8。tasks.md 分组编号保留，实现按依赖顺序执行。

**Placeholder 扫描：** 无 TBD/TODO；每个改动步骤给出具体代码或明确的既有复用点（`newTestService`、同包测试私钥 helper、`randomID`）。这些复用点在实现时须先 grep 确认真实名字后再使用（计划已标注）。

**类型一致性：** `IssueSource`/`Repository`/`Issue`/`IssueFilter`（git 包）在 2.1 定义，2.2/2.4/5.2 一致引用；`SystemIssuePublisherID` 常量在 5.1 定义后被 5.2 引擎与 core 一致使用；`SyncResult` 字段（Created/Updated/Skipped/Cancelled/Failed/Flagged/Errors）在 5.2 定义，4.5 REST 与 6.1 前端一致引用；`TaskView.Source` / `TaskSource` 在 5.5 定义，6.1/6.4 前端一致引用。

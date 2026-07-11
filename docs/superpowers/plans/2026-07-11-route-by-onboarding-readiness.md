---
change: route-by-onboarding-readiness
design-doc: docs/superpowers/specs/2026-07-11-route-by-onboarding-readiness-design.md
base-ref: 1f1949c6611036a0c1a16a1ff5cc565b058f85ee
---

# 按接入就绪状态分流默认入口 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让控制台按 GitHub App 安装与仓库接入事实选择默认入口，并在 Web 模式启动时强制使用合法、显式的 GitHub App 公网 origin。

**Architecture:** 前端用独立纯函数计算就绪目标，由一个 TanStack Query 驱动的路由组件负责加载与 replace navigation；根路径、登录成功和通配入口统一复用它。后端在 `config.Load` 边界解析并规范化 `GITHUB_APP_PUBLIC_BASE_URL`，manifest service 只消费校验后的值且不接触请求头。

**Tech Stack:** Go 1.26、`net/url`、testify；React 19、TypeScript、React Router 7、TanStack Query 5、Vitest、Testing Library。

## Global Constraints

- OpenSpec delta specs 是验收行为的唯一事实源。
- 不新增或修改 REST/MCP API、数据库表或依赖。
- `WEB_ENABLED=false` 时不要求 `GITHUB_APP_PUBLIC_BASE_URL`。
- Web 模式只接受无 userinfo、query、fragment、子路径的绝对 HTTP(S) origin。
- 不读取 `Host`、`X-Forwarded-Host`、`X-Forwarded-Proto` 推导公网地址。
- 每个生产代码改动必须先有能按预期失败的测试。

---

### Task 1: 严格校验并规范化 GitHub App 公网 origin

**Files:**
- Modify: `backend/internal/config/config_test.go`
- Modify: `backend/internal/config/config.go`

**Interfaces:**
- Consumes: `config.Load(get LookupEnv) (Config, error)` 与 `Config.GitHubAppPublicBaseURL`。
- Produces: `parseGitHubAppPublicBaseURL(raw string, required bool) (string, error)`；Web 模式下 `Config.GitHubAppPublicBaseURL` 始终为规范化 origin。

- [ ] **Step 1: 在 `validEnv` 中加入合法默认值，并写缺失与规范化失败测试**

```go
func TestLoadRequiresGitHubAppPublicBaseURLWhenWebEnabled(t *testing.T) {
	env := validEnv()
	delete(env, "GITHUB_APP_PUBLIC_BASE_URL")

	_, err := config.Load(func(key string) string { return env[key] })
	require.ErrorContains(t, err, "GITHUB_APP_PUBLIC_BASE_URL is required when WEB_ENABLED=true")

	env["WEB_ENABLED"] = "false"
	_, err = config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
}

func TestLoadValidatesGitHubAppPublicBaseURL(t *testing.T) {
	tests := []string{
		"/relative", "ftp://agentguild.example.com", "https:///missing-host",
		"https://user@agentguild.example.com", "https://agentguild.example.com/app",
		"https://agentguild.example.com?tenant=1", "https://agentguild.example.com#fragment",
	}
	for _, value := range tests {
		t.Run(value, func(t *testing.T) {
			env := validEnv()
			env["GITHUB_APP_PUBLIC_BASE_URL"] = value
			_, err := config.Load(func(key string) string { return env[key] })
			require.ErrorContains(t, err, "GITHUB_APP_PUBLIC_BASE_URL must be an absolute HTTP(S) origin")
		})
	}
}

func TestLoadNormalizesGitHubAppPublicBaseURL(t *testing.T) {
	env := validEnv()
	env["GITHUB_APP_PUBLIC_BASE_URL"] = "https://agentguild.example.com/"
	cfg, err := config.Load(func(key string) string { return env[key] })
	require.NoError(t, err)
	require.Equal(t, "https://agentguild.example.com", cfg.GitHubAppPublicBaseURL)
}
```

在 `validEnv()` 返回值加入：

```go
"GITHUB_APP_PUBLIC_BASE_URL": "https://agentguild.example.com",
```

- [ ] **Step 2: 运行配置测试并确认 RED**

Run: `cd backend && go test ./internal/config -run 'TestLoad(Requires|Validates|Normalizes)GitHubAppPublicBaseURL' -count=1`

Expected: FAIL；缺失值仍被接受或非法值未被拒绝。

- [ ] **Step 3: 在 `config.go` 实现最小解析并接入 `Load`**

新增 `net/url` import，并在 transport flags 解析完成后调用：

```go
cfg.GitHubAppPublicBaseURL, err = parseGitHubAppPublicBaseURL(cfg.GitHubAppPublicBaseURL, cfg.WebEnabled)
if err != nil {
	return Config{}, err
}
```

实现：

```go
func parseGitHubAppPublicBaseURL(raw string, required bool) (string, error) {
	if strings.TrimSpace(raw) == "" {
		if required {
			return "", fmt.Errorf("GITHUB_APP_PUBLIC_BASE_URL is required when WEB_ENABLED=true")
		}
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("GITHUB_APP_PUBLIC_BASE_URL must be an absolute HTTP(S) origin")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}
```

- [ ] **Step 4: 格式化并确认 GREEN**

Run: `cd backend && gofmt -w internal/config/config.go internal/config/config_test.go && go test ./internal/config -count=1`

Expected: PASS。

- [ ] **Step 5: 更新 OpenSpec task 并提交**

将 `tasks.md` 的 1.1 勾选为完成，然后：

```bash
git add backend/internal/config/config.go backend/internal/config/config_test.go openspec/changes/route-by-onboarding-readiness/tasks.md
git commit -m "feat: require GitHub App public origin"
```

### Task 2: 证明 manifest 地址只来自显式配置并更新部署文档

**Files:**
- Modify: `backend/internal/git/application/manifest_test.go`
- Modify: `docs/local-dev-github-issue-sync.md`
- Modify: `AGENTS.md`（若仓库实际跟踪该文件；否则只更新现有部署文档）

**Interfaces:**
- Consumes: `ManifestService.BuildManifest(tenantID) (manifestJSON, state, redirectURL string, err error)`。
- Produces: 回归测试证明 `url`、`redirect_url`、`setup_url` 使用配置 origin；部署文档把变量标为 Web 必填。

- [ ] **Step 1: 强化 manifest JSON 测试**

在现有 `TestManifestBuildContainsPermissionsAndCallbacks` 的 manifest 结构解析后加入：

```go
require.Equal(t, "https://guild.example.com", manifest.URL)
require.Equal(t, "https://guild.example.com/oauth/github/app/callback", manifest.RedirectURL)
require.Equal(t, "https://guild.example.com/oauth/github/app/installed", manifest.SetupURL)
```

该层没有 `*http.Request` 输入；测试通过配置一个与任意请求 Host 无关的固定 origin，即可锁定请求头无法影响输出的结构性边界。

- [ ] **Step 2: 运行 manifest 测试**

Run: `cd backend && go test ./internal/git/application -run TestManifestBuildContainsPermissionsAndCallbacks -count=1`

Expected: PASS；现有实现已经只使用配置值，此步骤建立回归覆盖，不需要为了制造 RED 修改生产代码。

- [ ] **Step 3: 更新部署说明**

在 `docs/local-dev-github-issue-sync.md` 的环境变量表和启动示例中明确：

```text
GITHUB_APP_PUBLIC_BASE_URL | WEB_ENABLED=true 时必填；AgentGuild 的公网 HTTP(S) origin，不包含路径、query 或 fragment
```

并在仓库维护的环境变量总览中同步相同约束与 HTTPS 生产示例。

- [ ] **Step 4: 运行相关测试并提交**

Run: `cd backend && go test ./internal/config ./internal/git/application -count=1`

Expected: PASS。

将 `tasks.md` 的 1.2 勾选，然后：

```bash
git add backend/internal/git/application/manifest_test.go docs/local-dev-github-issue-sync.md AGENTS.md openspec/changes/route-by-onboarding-readiness/tasks.md
git commit -m "test: lock GitHub manifest to configured origin"
```

若 `AGENTS.md` 不存在或不受版本控制，从 `git add` 中移除它，不创建重复文档。

### Task 3: 实现统一的前端 onboarding 就绪入口

**Files:**
- Create: `frontend/src/app/HomeRedirect.tsx`
- Create: `frontend/src/app/HomeRedirect.test.tsx`
- Modify: `frontend/src/app/AppShell.tsx`
- Modify: `frontend/src/features/auth/LoginPage.tsx`
- Modify: `frontend/src/features/auth/LoginPage.test.tsx`

**Interfaces:**
- Consumes: `getRepositoryOnboarding(): Promise<Envelope<RepositoryOnboardingSummary>>`。
- Produces: `onboardingDestination(summary: RepositoryOnboardingSummary): "/tasks" | "/onboarding"` 与 `HomeRedirect` 组件。

- [ ] **Step 1: 写纯判定与路由失败测试**

`HomeRedirect.test.tsx` 使用 `vi.mock("../api/client", ...)` mock `getRepositoryOnboarding`，用独立 `QueryClientProvider` 和 `MemoryRouter` 渲染组件。至少包含：

```tsx
it.each([
  [{ configured: false, installation_id: 0 }, [{ id: "repo-1" }], "/onboarding"],
  [{ configured: true, installation_id: 0 }, [{ id: "repo-1" }], "/onboarding"],
  [{ configured: true, installation_id: 42 }, [], "/onboarding"],
  [{ configured: true, installation_id: 42 }, [{ id: "repo-1" }], "/tasks"],
])("routes readiness to %s", (githubApp, repositories, expected) => {
  const summary = {
    github_app: githubApp,
    app_repositories: { items: [] },
    onboarded_repositories: { items: repositories },
  } as RepositoryOnboardingSummary;
  expect(onboardingDestination(summary)).toBe(expected);
});
```

并加入异步错误用例，mock rejection 后断言 location 为 `/onboarding`。

- [ ] **Step 2: 运行新测试并确认 RED**

Run: `cd frontend && npm test -- --run src/app/HomeRedirect.test.tsx`

Expected: FAIL，模块或导出尚不存在。

- [ ] **Step 3: 实现 `HomeRedirect.tsx`**

```tsx
import { useQuery } from "@tanstack/react-query";
import { Navigate } from "react-router-dom";
import { getRepositoryOnboarding, type RepositoryOnboardingSummary } from "../api/client";

export function onboardingDestination(summary: RepositoryOnboardingSummary): "/tasks" | "/onboarding" {
  const app = summary.github_app;
  return app.configured && typeof app.installation_id === "number" && app.installation_id > 0 &&
    summary.onboarded_repositories.items.length > 0
    ? "/tasks"
    : "/onboarding";
}

export function HomeRedirect() {
  const query = useQuery({ queryKey: ["repository-onboarding"], queryFn: getRepositoryOnboarding, retry: false });
  if (query.isPending) return <p role="status">正在检查接入状态...</p>;
  if (query.isError) return <Navigate to="/onboarding" replace />;
  return <Navigate to={onboardingDestination(query.data.data)} replace />;
}
```

- [ ] **Step 4: 接入路由与登录跳转**

在 `AppShell.tsx` import `HomeRedirect`，并改为：

```tsx
<Route path="/" element={<HomeRedirect />} />
<Route path="*" element={<HomeRedirect />} />
```

在 `LoginPage.tsx` 中把成功跳转改为：

```ts
window.location.href = "/";
```

同步修改 `LoginPage.test.tsx` 的期望为 `/`。

- [ ] **Step 5: 运行前端定向测试并确认 GREEN**

Run: `cd frontend && npm test -- --run src/app/HomeRedirect.test.tsx src/features/auth/LoginPage.test.tsx`

Expected: PASS。

- [ ] **Step 6: 勾选任务并提交**

将 `tasks.md` 的 2.1、2.2 勾选，然后：

```bash
git add frontend/src/app/HomeRedirect.tsx frontend/src/app/HomeRedirect.test.tsx frontend/src/app/AppShell.tsx frontend/src/features/auth/LoginPage.tsx frontend/src/features/auth/LoginPage.test.tsx openspec/changes/route-by-onboarding-readiness/tasks.md
git commit -m "feat: route default entry by onboarding readiness"
```

### Task 4: 全量验证与规范一致性

**Files:**
- Modify: `openspec/changes/route-by-onboarding-readiness/tasks.md`

**Interfaces:**
- Consumes: 前三项任务的提交与 OpenSpec delta specs。
- Produces: 全部构建/测试证据和完成的任务清单。

- [ ] **Step 1: 运行后端构建与相关测试**

Run: `cd backend && go build ./... && go test ./internal/config ./internal/git/application -count=1`

Expected: exit 0，所有测试 PASS。

- [ ] **Step 2: 运行前端构建与全量单元测试**

Run: `cd frontend && npm run build && npm test -- --run`

Expected: exit 0，Vitest 零失败。

- [ ] **Step 3: 运行 OpenSpec 严格校验和仓库验证**

Run: `openspec validate route-by-onboarding-readiness --strict`

Expected: `Change 'route-by-onboarding-readiness' is valid`。

Run: `scripts/comet-verify.sh`

Expected: exit 0。

- [ ] **Step 4: 按验收场景核对实现**

逐条确认：ready → `/tasks`；App 未配置/未安装/无仓库/请求失败 → `/onboarding`；显式 `/agents` 保留；Web 缺失或非法 origin 启动配置失败；manifest 只使用配置 origin。

- [ ] **Step 5: 完成任务清单并提交验证元数据**

将 `tasks.md` 的 3.1 勾选，然后：

```bash
git add openspec/changes/route-by-onboarding-readiness/tasks.md
git commit -m "chore: complete onboarding readiness verification"
```

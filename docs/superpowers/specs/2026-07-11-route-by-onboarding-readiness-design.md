---
comet_change: route-by-onboarding-readiness
role: technical-design
canonical_spec: openspec
---

# 按接入就绪状态分流默认入口

## 背景

控制台的本地登录成功路径固定进入 `/agents`，未知路由却进入 `/tasks`，默认入口不一致。系统已经通过 `GET /v1/repository-onboarding` 提供 GitHub App 和已接入仓库的租户级事实数据，因此无需新增 onboarding 状态接口或持久化字段。

GitHub App manifest 当前直接拼接可选的 `GITHUB_APP_PUBLIC_BASE_URL`。变量为空时会生成相对 callback/setup URL，导致 GitHub 拒绝 manifest。公网 origin 必须成为 Web 部署的显式、安全边界。

需求与验收场景以 OpenSpec change `route-by-onboarding-readiness` 的 delta specs 为准。

## 前端设计

### 就绪判定

新增一个纯函数接收 `RepositoryOnboardingSummary`，仅在以下条件全部成立时返回 `/tasks`：

1. `github_app.configured === true`
2. `github_app.installation_id` 是大于零的数值
3. `onboarded_repositories.items.length > 0`

其余成功响应返回 `/onboarding`。纯函数把业务规则与 React 生命周期分离，便于表驱动测试并减少路由测试中的异步噪声。

### 入口组件

入口组件通过 TanStack Query 调用现有 `getRepositoryOnboarding()`：

- pending：展示带 `role="status"` 的轻量加载反馈；
- success：调用纯函数获得目标路由并用 `<Navigate replace>` 跳转；
- error：保守地 `<Navigate to="/onboarding" replace>`。

组件不缓存新的 onboarding 状态，也不修改 API client。它使用现有查询能力和响应类型。

### 路由统一

- `/` 渲染入口组件；
- `*` 渲染入口组件，不再直接指向任务中心；
- 本地登录成功设置 `window.location.href = "/"`；
- `/agents`、`/tasks`、`/onboarding` 等显式路由保持原样。

因此，首次登录、已有 session 打开根路径以及误入未知路径都会走同一规则，而管理员仍可直接进入 Agents 管理页。

## 后端设计

### 配置解析

在 `config.Load` 内增加一个专用解析函数，对 `GITHUB_APP_PUBLIC_BASE_URL` 执行：

- `WEB_ENABLED=true` 时非空；
- `url.Parse` 成功；
- scheme 为 `http` 或 `https`；
- host 非空；
- userinfo、raw query、fragment 为空；
- path 为空或 `/`。

合法值规范化为 `scheme://host`，去除唯一允许的末尾 `/`。校验在 manifest service 构造前完成，使部署错误在进程启动阶段暴露。`WEB_ENABLED=false` 时不要求该变量，保持 MCP-only 部署兼容。

### Manifest 地址来源

`ManifestService` 继续接收已经校验和规范化的 `PublicBaseURL`，并拼接固定路径：

- `/oauth/github/app/callback`
- `/oauth/github/app/installed`

handler 不读取 `Host`、`X-Forwarded-Host`、`X-Forwarded-Proto`。manifest 单元测试直接解析 `BuildManifest` 返回的 JSON，证明生成地址完全由配置决定。由于服务层从未接收请求对象，请求头天然无法影响地址生成。

## 错误与安全边界

- 配置缺失返回 `GITHUB_APP_PUBLIC_BASE_URL is required when WEB_ENABLED=true`。
- 配置格式非法返回包含变量名的稳定错误，不回显 secret。
- 前端状态请求失败不放行到任务中心，避免把“未知”误判为“已就绪”。
- 不信任请求 Host 和转发头，避免 Host header poisoning 或代理信任配置影响 OAuth 回调。

## 测试策略

### 后端

- 在 `backend/internal/config/config_test.go` 先添加失败测试，覆盖缺失、相对 URL、非法 scheme、缺失 host、userinfo、query、fragment、子路径、合法 origin、末尾 `/` 规范化和 Web 关闭场景。
- 在 manifest service 测试中解析 JSON，断言 `url`、`redirect_url`、`setup_url` 使用规范化 origin。
- 运行 `go test ./internal/config ./internal/git/application` 和 `go build ./...`。

### 前端

- 为入口规则添加测试，覆盖 ready、未配置、未安装、无仓库和请求失败。
- 通过 MemoryRouter 验证 `/` 与未知路径分流，验证显式 `/agents` 保持可访问。
- 验证本地登录成功转到 `/`，不再直接进入 `/agents`。
- 运行相关 Vitest、全量前端单测和 `npm run build`。

## 部署与回滚

发布前为所有 `WEB_ENABLED=true` 环境设置公网 origin，例如：

```bash
GITHUB_APP_PUBLIC_BASE_URL=https://agentguild.example.com
```

先部署后端配置约束，再部署前端入口分流。回滚代码时可以保留该环境变量，旧版本会继续忽略或使用它，不需要数据迁移。

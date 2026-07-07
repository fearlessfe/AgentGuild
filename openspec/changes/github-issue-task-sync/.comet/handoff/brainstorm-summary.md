# Brainstorm Summary

- Change: github-issue-task-sync
- Date: 2026-07-07

## Confirmed Technical Approach

1. **前置修复 — ScopePolicy 类型分支**：`auth.ScopePolicy.Require` 按 `principal.Type` 分支。Agent 保持 `tenant_id`+`agent_id`+`agent_version_id`+`scopes` 校验；Human 仅要求 `tenant_id` 非空并放行既有 spec 声明的共享只读接口。写入接口对人类的既有拒绝不变。审阅 `review/application/policy.go:134` 复用点。

2. **Issue 任务建模 — 固定 system publisher**：Issue 来源 Task 用固定 tenant 级 system publisher 标识（如 `system-issue-sync`）填 `publisher_agent_version_id`，复用 `domain.NewTask`。来源标识（repo + issue_number）存于独立 Issue↔Task 映射表（去重 + 状态对账 + 来源展示）。

3. **GitHub 能力扩展**：在 `git/github` driver + `GitHubAppManager.Driver(tenant)` 基础上新增 `ListInstallationRepositories`、`ListIssues(repo, filter, since)`，复用安装令牌与 base URL 解析。sync 应用层依赖抽象 Issue 源接口，便于 stub 测试。

3a. **一键安装 — GitHub App Manifest 流程（Dokploy 式，"什么都不需要预置"）**：不要求平台预置 GitHub App，也不要求用户手输 App ID/私钥。流程：
   - 前端「连接 GitHub」按钮 → 后端 `GET /oauth/github/app/manifest`（或前端直接 POST 表单到 GitHub）以 **App Manifest**（描述权限 Contents:R、Issues:RW、Checks:R、Metadata:R + 回调/setup URL）跳转 `https://github.com/settings/apps/new?state=...`。
   - 用户在 GitHub 确认创建 → GitHub 回调 `GET /oauth/github/app/callback?code=...&state=...` → 后端用 `code` 调 `POST /app-manifests/{code}/conversions` **换回** app_id + private_key(pem) + webhook_secret + client_id/secret，落库到 `github_apps`（现有列已含 app_id/private_key/base_url；webhook_secret 等可加可空列）。
   - 换取后引导用户**安装** App 到组织/仓库：跳 `https://github.com/apps/<slug>/installations/new`，GitHub 回调 setup URL 带 `installation_id` → 后端持久化 installation_id。
   - 完成后前端「仓库与同步规则」页 `GET /v1/repositories` 即列出安装仓库。
   - **可选保留手动配置**作为高级/降级路径（现有 `POST /v1/github-app` 不动）。
   - **连接检测（纳入本 change）**：`POST /v1/github-app:test`（或 GET 变体）用已存配置调一次轻量 GitHub API（如列安装仓库）验证凭证有效，前端「检测连接」按钮接入。
   - 存储验证：现有 `github_apps` 表列（app_id/installation_id/private_key/base_url）满足轮询同步所需；manifest 额外产出的 webhook_secret/client_id/secret 若需要以可空列补充（本 change 用轮询，暂不强依赖 webhook）。

4. **同步触发 — 全局轮询 worker**：单一 sync worker 按配置固定间隔 tick，遍历所有启用规则；规则仅启/停开关（无每规则频率）。复用 `runWorker/repeat`，per-tenant 容错，单规则失败不阻塞其它。

5. **规则为一等资源 + 完整 CRUD**：`GET /v1/repositories`（实时调 GitHub）、`GET/POST/PUT/DELETE /v1/sync-rules`、`POST /v1/sync-rules/{id}:run`（立即同步便于冒烟）。人类 session 鉴权，写操作要求 admin。规则字段：仓库、包含/排除标签、Issue 状态、任务类型、默认优先级、重复策略、启用。

6. **重复/更新策略 — 仅 open/draft 可更新**：首次创建；已存在映射时仅当 Task 仍 open/draft 更新内容字段；已领取/执行/完成/取消则只更新映射元数据，不覆盖任务状态。

7. **状态对账 — Issue 关闭驱动任务结束**：每次同步检查源 Issue 状态。Issue 关闭/resolved 时：Task 仍 open/draft（未领取）→ sync 以 system publisher 身份 `cancel`；已被领取/执行中 → 不强制终止，仅在映射标记来源已关闭。
   - **域支持已验证**：`IntentCancel` 要求 actor=Publisher 且 id=PublisherID；system 任务 publisher 即 system 标识，故 worker 可合法 cancel 未领取任务，无需改 domain。

8. **deadline 字段**：`NewTask` 要求非零 deadline。任务生命周期主要由 Issue 状态驱动；deadline 用足够长的默认相对期限填充（避免与 `IntentExpire` 提前冲突）。

9. **前端接线 + 默认真实后端**：接线 `features/git/GitIntegrationScreen`、`features/sync/*`；任务中心展示 Issue 来源标识；`.env.example` 默认不启用 demo；移除相关 `ApiNote status=planned`。

10. **本地脚手架**：`.local/env.sh` 补 GitHub App + 同步间隔变量；文档化 Postgres+backend+frontend+App 配置；真实 App 冒烟。

## Key Trade-offs and Risks

- 放宽 ScopePolicy 仅跳过 human 的 agent_id 前置 → 回归测试固化「human POST /v1/tasks 仍 401」「Agent 校验不变」「review 复用点不误放行」。
- GitHub 限流/分页/失败 → 增量 `since` 水位 + 分页 + 退避;单规则失败隔离。
- 自动 cancel 仅限未领取 → 不打断进行中执行。
- system 任务与 Agent 任务共存 → 来源标识区分,不改 publisher 字段语义。
- deadline 为占位默认 → 生命周期以 Issue 状态为准,不依赖 deadline 结束。

## Local-dev Caveat (manifest callback)

- Manifest 一键安装依赖 GitHub 可回调平台的 callback/setup URL。本地开发需公网可达（隧道，如 cloudflared/ngrok）或使用手动配置降级路径（现有 `POST /v1/github-app`）。Design Doc 记为风险 + 缓解；冒烟文档给出隧道或手动配置两种路径。

## Testing Strategy

- 全部自动化测试走 stub Issue 源（不调真 GitHub）：过滤（含/排标签、状态）、映射、去重、Issue 关闭→未领取 cancel、已领取仅标记不打断、单规则失败容错、增量水位。
- ScopePolicy 单测（human 放行只读 / human 写入拒 / Agent 不变）+ review 复用点回归。
- GitHub driver 新方法单测（分页/过滤/since）用 stub http。
- 真实 GitHub App 仅用于最终手动冒烟（8 步验收）。

## Spec Patches（写回 issue-task-sync delta spec）

- 补充场景：Issue 关闭 → 未领取任务 cancel；Issue 关闭 → 已领取任务仅标记来源已关闭不改状态。
- 补充边界：system publisher 标识；Issue 来源 Task 的 deadline 默认值；仅 open/draft 可更新的重复策略场景。

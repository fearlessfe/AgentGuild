---
comet_change: github-issue-task-sync
role: technical-design
canonical_spec: openspec
---

# GitHub Issue → Task 同步（技术设计）

## Context

AgentGuild 后端已有：per-tenant GitHub App 配置（`/v1/github-app` CRUD）、git-delivery（凭证 + commit 验证）、核心 Task 生命周期（`application.Service`）。缺口是**没有任何 GitHub Issue → Task 同步能力**，且前端 sync/git 页为 mock。此外 `auth.ScopePolicy.Require` 对所有 principal 无条件要求 `agent_id`，人类 session 读 `GET /v1/tasks` 得 `400 agent_id is invalid`（与 `agent-access-control` 既有要求矛盾）。

本设计基于 OpenSpec 开阶段产物（proposal/design/specs）与 brainstorming 确认结论，落地：读路径修复 + GitHub 一键接入（Manifest 流程）+ Issue→Task 同步（轮询 + 状态对账）+ 前端接线。

上游权威规格为 OpenSpec delta spec（`specs/agent-access-control`、`specs/issue-task-sync`、`specs/local-dev-bootstrap`）。本文档描述 HOW，不重写 WHAT。

## Goals / Non-Goals

**Goals**
- 修复 ScopePolicy 使人类 session 可读共享只读接口。
- GitHub 一键接入（Manifest 创建 App → 换取凭证 → 安装 → 列仓库），无需用户手输凭证或平台预置 App；保留手动配置为降级路径。连接检测。
- 完整同步规则 CRUD + 全局轮询 worker，把匹配 Issue 同步为 system 来源 Task；去重；Issue 关闭驱动任务结束。
- 前端接线 git/sync 页与任务中心来源标识；前端默认真实后端。
- 真实 GitHub App 手动冒烟通过。

**Non-Goals**
- 审核/声望/评测、执行/提交/结果、onboarding 页接线（后续 change）。
- Agent 执行闭环（领取→提交→验证）。
- Webhook 实时同步（本次用轮询；manifest 产出的 webhook_secret 先存不强用）。
- 改造 E2E 跑真实后端；MCP 传输改动。
- 既有路由 request/response 形状变更（仅新增路由）。

## Architecture Overview

```
┌── 前端 (React) ──────────────┐        ┌── 后端 (Go/chi) ───────────────────────────┐
│ Git 接入页                    │        │ REST                                        │
│  [连接 GitHub] ───────────────┼──GET──▶│ /oauth/github/app/manifest  → 跳 GitHub     │
│  ◀── 回调 ────────────────────┼──GET──▶│ /oauth/github/app/callback  → 换 code 落库  │
│  [检测连接] ──────────────────┼─POST──▶│ /v1/github-app:test                         │
│ 仓库与同步规则页               │        │ /v1/repositories (实时列安装仓库)            │
│  规则 CRUD / 立即同步 ─────────┼─CRUD──▶│ /v1/sync-rules[, :run]                      │
│ 任务中心 (来源标识) ───────────┼──GET──▶│ /v1/tasks (human 放行)                      │
└──────────────────────────────┘        │ Sync Worker (全局轮询)                       │
                                         │  遍历启用规则 → ListIssues → 过滤 →          │
                                         │  映射 system Task / 去重 / Issue 关闭对账     │
                                         │ git/github Driver (+ListRepos +ListIssues)  │
                                         │ github_apps 表 / sync_rules 表 / issue_task_map 表 │
                                         └─────────────────────────────────────────────┘
```

## Decisions

### D1 ScopePolicy 按 principal 类型分支（前置修复）
`auth.ScopePolicy.Require` 内先判 `principal.Type`：
- Agent：保持 `tenant_id`+`agent_id`+`agent_version_id` 非空 + `scopes` 匹配。
- Human：仅要求 `tenant_id` 非空；不因 `agent_id` 空而拒绝共享只读。
写入接口对人类的拒绝由中间件/写 handler + 既有 spec 保证，不受影响。审阅 `review/application/policy.go:134` 复用点，确保不误放行评审写入。
- 单元测试：human 放行 `tasks:read`；Agent 字段空仍拒；Agent 正常授权不变；human `POST /v1/tasks` 仍 401。

### D2 Issue 任务建模 = 固定 system publisher
Issue 来源 Task 用固定 tenant 级 system publisher 标识（如常量 `system-issue-sync`）填 `publisher_agent_version_id`，复用 `domain.NewTask`。来源信息（repo、issue_number、issue 状态、最后同步）存独立 `issue_task_map` 表。任务中心通过 join 映射展示"来源=Issue #N @ repo"。
- 备选：新增 Task.source 字段（否决——改动面大）；伪造真实 system Agent（否决——语义混淆）。

### D3 GitHub 能力扩展
在 `git/github` driver 新增 `ListInstallationRepositories()`、`ListIssues(repo, filter, since)`；复用 `GitHubAppManager.Driver(tenant)` 的安装令牌与 base URL 解析。sync 应用层依赖抽象 `IssueSource` 接口（driver 为其一个实现，测试用 stub）。分页 + `since` 增量 + 429 退避。

### D4 一键接入 = GitHub App Manifest 流程
- `GET /oauth/github/app/manifest`：服务端生成 App Manifest（权限 Contents:R、Issues:RW、Checks:R、Metadata:R；redirect/setup/callback URL；state 防 CSRF），以自动提交表单方式跳 `https://github.com/settings/apps/new`。
- `GET /oauth/github/app/callback?code&state`：校验 state → 调 `POST /app-manifests/{code}/conversions` → 得 app_id、pem、webhook_secret、client_id/secret → 落 `github_apps`（app_id/private_key/base_url 复用现列；webhook_secret 等加可空列）。
- 安装引导：跳 `https://github.com/apps/<slug>/installations/new`，setup 回调带 `installation_id` → 落库。
- 手动配置（`POST /v1/github-app`）保留为高级/降级。
- 备选：平台预置单一 App + 仅安装流程（否决——需预置凭证，不符合"什么都不需要预置"）。

### D5 连接检测
`POST /v1/github-app:test`：用当前 tenant 已存配置构造 driver，调一次轻量 GitHub API（列安装仓库或 GET app）验证凭证/安装有效，返回结构化结果（ok/失败原因，不回显私钥）。

### D6 同步规则 = 一等资源 + 全局轮询
- 表 `sync_rules`：tenant、repo、include_labels、exclude_labels、issue_state、task_type、default_priority、dedupe_strategy、enabled、时间戳。
- REST：`GET /v1/repositories`（实时）、`GET/POST/PUT/DELETE /v1/sync-rules`、`POST /v1/sync-rules/{id}:run`。人类 session 鉴权，写操作要求 admin。
- Worker：单一全局 sync worker，复用 `runWorker/repeat`，按配置间隔 tick；per-tenant 遍历启用规则，单规则失败隔离（不阻塞其它）。规则仅启/停（无每规则频率）。

### D7 映射 / 去重 / 更新
- 表 `issue_task_map`：唯一键 (tenant_id, repo, issue_number) → task_id、issue_state、last_synced_at。
- 首次匹配：创建 system Task + 写映射。
- 已存在：仅当 Task 仍 open/draft 才更新内容字段（title/problem，标签→type/priority）；已推进（claimed/in_progress/completed/cancelled）只更新映射元数据，不覆盖任务状态。

### D8 Issue 状态对账
每次同步读取源 Issue 状态。Issue 关闭/resolved：
- Task 仍 open/draft（未领取）→ sync 以 **system publisher 身份** `IntentCancel`（已验证 domain：`IntentCancel` 要求 actor=Publisher 且 id=PublisherID，system 任务 publisher 即该标识，合法且无需改 domain）。
- Task 已被领取/执行中 → 不强制终止，仅在映射标记 `issue_closed`，供人工参考。

### D9 deadline 占位
`domain.NewTask` 要求非零 deadline。Issue 无 deadline；任务生命周期以 Issue 状态为准（关闭即结束）。同步创建时填一个足够长的默认相对期限（如 now + 默认天数，配置项），避免与 `IntentExpire`（deadline 到期由 system 置 expired）过早冲突。

### D10 前端接线
- `GitIntegrationScreen`：接 manifest 一键连接、`GET /v1/github-app`（已配置态）、`:test` 连接检测、`DELETE`。
- `SyncRuleScreen`/`SyncResultScreen`：规则 CRUD + 立即同步 + 结果展示。
- 任务中心：展示 Issue 来源标识。
- `frontend/.env.example` 默认不启用 demo；移除相关 `ApiNote status="planned"`。

## Data Model (new/changed)

- `github_apps`：加可空列 `webhook_secret`、`client_id`、`client_secret`、`app_slug`（供 manifest 流程与安装跳转）。
- `sync_rules`（新）：见 D6。
- `issue_task_map`（新）：见 D7。
- 迁移均提供 down。

## Error Handling

- GitHub 429/5xx：退避重试；单规则失败记录并继续（per-tenant 容错，沿用 validation worker 模式）。
- manifest code 换取失败/ state 不符：返回结构化错误，不落库。
- 缺失 GitHub 配置时 `/v1/repositories`、`:test`、`:run` 返回明确未配置错误。
- 私钥永不回显（沿用 GitHubAppView）。

## Testing Strategy

- **单元/集成全部走 stub `IssueSource`**（不调真 GitHub）：标签过滤（含/排）、状态过滤、映射创建、去重更新（仅 open/draft）、Issue 关闭→未领取 cancel、已领取仅标记不打断、单规则失败容错、`since` 增量水位。
- ScopePolicy 单测（human 放行只读 / human 写入拒 / Agent 不变）+ `review` 复用点回归。
- driver 新方法（ListRepos/ListIssues）用 stub http：分页、过滤、since。
- manifest/callback：state 校验、conversions 解析、落库（GitHub 交互 stub）。
- 真实 GitHub App 仅用于最终手动冒烟（下）。

## Risks / Trade-offs

- **[manifest 回调需公网可达]** → 缓解：本地用隧道（cloudflared/ngrok）或走手动配置降级路径；冒烟文档给两条路径。
- **[放宽 policy 误伤写入]** → 缓解：类型分支只跳过 human 的 agent_id 前置；回归固化写入仍 401、review 复用点不误放行。
- **[GitHub 限流/分页]** → 增量水位 + 分页 + 退避 + 单规则隔离。
- **[自动 cancel 打断执行]** → 仅 cancel 未领取任务。
- **[system 任务与 Agent 任务共存]** → 来源标识区分，不改 publisher 字段语义。
- **[webhook_secret 等存而不用]** → 明确本次轮询不依赖 webhook，列为可空为将来 webhook 预留。

## Migration / Rollout

1. ScopePolicy 类型分支 + 测试。
2. 迁移：`github_apps` 加列、`sync_rules`、`issue_task_map`（+down）。
3. driver 扩展（ListRepos/ListIssues）+ stub。
4. manifest/callback + `:test` + `/v1/repositories`。
5. sync 领域/应用/worker + system publisher Task 创建 + 状态对账；接 `main.go` 与配置。
6. 规则 CRUD REST。
7. 前端接线 + `.env.example` + 移除 planned 标注。
8. 本地脚手架 + 冒烟文档（含隧道）。
- 回滚：新增路由/表/worker 可关（worker 不启动即无副作用）；ScopePolicy 改动可 revert；迁移含 down。

## Smoke (真实 GitHub App)

1. 本地起全栈（含隧道使回调可达）；本地登录，全程无 `agent_id` 报错。
2. Git 接入页点「连接 GitHub」→ Manifest 创建 App → 回调落库 → 安装 → installation_id 落库。
3. 「检测连接」返回 ok。
4. 「仓库与同步规则」页列出安装仓库。
5. 建一条规则（某 repo、open、含 `agent-ready` 标签、type=code），触发立即同步。
6. 任务中心出现由真实 Issue 生成的 Task，含来源标识。
7. 关闭该 Issue，再同步 → 未领取任务被 cancel。
8. 重复同步不产生重复任务。

## Spec Patches（已写回 `specs/issue-task-sync/spec.md`）

- 补：一键接入（Manifest）与安装回调、连接检测的要求与场景。
- 补：Issue 关闭 → 未领取任务 cancel / 已领取仅标记来源已关闭。
- 补：system publisher 标识、Issue 任务 deadline 默认值、仅 open/draft 可更新的边界。

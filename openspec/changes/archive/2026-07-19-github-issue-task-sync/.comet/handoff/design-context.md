# Comet Design Handoff

- Change: github-issue-task-sync
- Phase: design
- Mode: compact
- Context hash: b3ca7ffea6c3439cc64c0ee65b6558c853ba7a2b033fade5857165c98d154055

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/github-issue-task-sync/proposal.md

- Source: openspec/changes/github-issue-task-sync/proposal.md
- Lines: 1-35
- SHA256: b59faee53d894c0a8bf29b5cd291effa332671590bd7867b8dfd5e5e1768960a

```md
## Why

AgentGuild 前端已设计「仓库与同步规则」页，用规则驱动 **GitHub Issue → Task 同步**（例如「每 5 分钟同步 billing-service 中带 `agent-ready`、状态 open 的 Issue，生成 code 任务」），但**后端完全没有该能力**：不存在仓库列举、同步规则、Issue 拉取或 Issue→Task 映射的任何代码。因此任务中心目前无真实任务来源，前后端未串起来。

此外还有一个前置阻塞：`auth.ScopePolicy.Require` 无条件要求 principal 携带 `agent_id`，导致人类 session 访问 `GET /v1/tasks` 等只读接口返回 `400 agent_id is invalid`——与 `agent-access-control` 规格「人类控制台可访问共享只读接口」相矛盾（实现漂移）。即使同步出任务，人类控制台也读不到。

本 change 交付**从 GitHub Issue 同步生成任务**的端到端能力，并顺带修复上述读路径阻塞与本地一键运行，使任务中心显示由真实 Issue 生成的任务。

## What Changes

- **修复人类读路径漂移**（前置）：`ScopePolicy.Require` 按 principal 类型分支——Agent 保持要求 `agent_id`/`agent_version_id`/`scopes`；Human 仅要求 `tenant_id` 并放行既有 spec 声明的共享只读接口。使 `GET /v1/tasks` 等对人类 session 返回 200。
- **新增 GitHub 仓库列举**：通过既有 per-tenant GitHub App 安装，列出可纳入治理的安装仓库（扩展 GitHub 客户端能力）。
- **新增同步规则 CRUD**：每租户可创建/查看/更新/启停同步规则，字段含目标仓库、包含/排除标签、Issue 状态、任务类型、默认优先级、去重策略等（对齐前端 UI）。
- **新增 Issue→Task 同步（轮询 worker）**：后台按规则周期性拉取匹配的 GitHub Issue，映射为平台 Task；已存在的按去重策略更新，避免重复。Issue 来源的 Task 以 system publisher 建模（复用既有 `ActorSystem`），不破坏「Agent 发布」写入语义。
- **前端接线 sync/git 页 + 默认真实后端**：`仓库与同步规则` 与 `Git 接入` 页接真实后端；`VITE_DEMO_MODE` 默认关闭（保留 demo 降级）。
- **本地一键运行脚手架**：文档化/脚本化 Postgres + backend + frontend + GitHub App 配置的本地启动，使用真实 GitHub App 安装验证。
- 不改变既有对外 API 契约的既有路由 request/response 形状；仅**新增**路由。

## Capabilities

### New Capabilities
- `issue-task-sync`: 以每租户同步规则驱动，从 GitHub Issue 周期性同步生成/更新平台 Task 的能力（仓库列举、规则 CRUD、轮询同步 worker、Issue→Task 映射与去重）。
- `local-dev-bootstrap`: 本地开发可复现启动能力——一键拉起 Postgres/后端/前端、配置真实 GitHub App、前端默认连接真实后端并保留 demo 降级。

### Modified Capabilities
- `agent-access-control`: 修正实现语义，使人类 session principal（无 `agent_id`）不被 Scope 校验当作非法参数而拒绝共享只读接口；不放宽既有「写入接口不接受人类 session」要求。

## Impact

- **后端（新增为主）**：新增 `issue-task-sync` 领域/应用/存储与同步 worker；扩展 GitHub 客户端（列仓库、列 Issue）；新增 REST 路由（仓库列举、同步规则 CRUD、可选「立即同步」）；修改 `backend/internal/auth/principal.go`（`ScopePolicy.Require` 类型分支）；Task 创建路径支持 system publisher。
- **数据库**：新增迁移（同步规则表、Issue↔Task 映射/去重表、同步游标/水位）。
- **前端**：接线 `features/sync/*`、`features/git/GitIntegrationScreen`；`.env.example`；默认真实后端。
- **配置/脚本**：`.local/env.sh` 补 GitHub App 与同步相关变量；本地启动脚本与文档。
- **验收**：手动冒烟——本地配置真实 GitHub App，创建一条同步规则，触发/等待同步后任务中心显示由真实 Issue 生成的 Task；人类 session 全程无 `agent_id` 类报错。
- **非目标**：审核/声望/评测/执行/提交/结果页完整接线（后续 change）；onboarding 页；Agent 侧执行闭环（激活→领取→提交→验证）；webhook 实时同步（本次用轮询）；改造 E2E 跑真实后端；MCP 传输改动。
```

## openspec/changes/github-issue-task-sync/design.md

- Source: openspec/changes/github-issue-task-sync/design.md
- Lines: 1-79
- SHA256: 88906dfdaf90cf5e9ef2b910d3ffd43efa2e7b2d83575e5e71d7b7cb2dd9160d

```md
## Context

AgentGuild 后端已具备：per-tenant GitHub App 配置（`/v1/github-app`）、git-delivery（凭证签发 + commit 验证）、核心 Task 生命周期（`application.Service`，含 `PublishTask`）。但**没有任何** GitHub Issue → Task 的同步能力：`git.Driver` 仅有 `CreateCredential/GetCommit/CompareCommits/IsAncestor`，无 Issue/仓库列举；前端 `features/sync/*` 全为 mock（`ApiNote status="planned"`）。

同时存在读路径阻塞：`auth.ScopePolicy.Require`（`backend/internal/auth/principal.go`）对所有 principal 无条件要求 `agent_id`/`agent_version_id`，人类 session（这两字段为空）访问 `GET /v1/tasks` 得到 `400 agent_id is invalid`，与 `agent-access-control` 既有要求「人类可访问共享只读接口」矛盾。

领域约束（AGENTS.md）：任务原本由 Agent 通过 API 发布（`publisher_agent_version_id` 指向 Agent 版本）。本 change 引入 **Issue 来源的任务**，需在不破坏「Agent 发布/领取/提交」写入语义的前提下，为 system 来源任务建模。`domain` 已有 `ActorSystem` 角色，`docker-compose` 提供本地 Postgres，已有 validation/reputation/outbox 等**轮询 worker** 范式可复用。

本 change 是模块化拆分 #1，聚焦「Issue→Task 同步」这条链路端到端可用；审核/声望/评测、执行/提交/结果、onboarding 为后续独立 change。

## Goals / Non-Goals

**Goals:**
- 人类 session 能读取共享只读接口（修复 ScopePolicy 漂移），任务中心可显示任务。
- 每租户可通过 GitHub App 列出安装仓库，并对同步规则做完整 CRUD + 启停。
- 后台轮询 worker 按规则从真实 GitHub 拉取匹配 Issue，映射为 Task；重复 Issue 按去重策略更新而非重复创建。
- 前端 `仓库与同步规则`、`Git 接入` 页接真实后端；前端默认真实后端并保留 demo 降级。
- 本地用真实 GitHub App 手动冒烟：建规则 → 同步 → 任务中心出现由 Issue 生成的 Task。

**Non-Goals:**
- 不做 webhook 实时同步（本次仅轮询；可留扩展点）。
- 不接线审核/声望/评测/执行/提交/结果/onboarding 页（后续 change）。
- 不实现 Agent 侧执行闭环（激活→领取→提交→验证）。
- 不改造 Playwright E2E 跑真实后端（验收仅手动冒烟）。
- 不改既有路由的 request/response 形状；仅新增路由。

## Decisions

### D1: ScopePolicy 按 principal 类型分支（前置修复）
`Require` 内先判 `principal.Type`：Agent 保持 `tenant_id`+`agent_id`+`agent_version_id`+`scopes` 校验；Human 仅要求 `tenant_id` 非空并放行既有 spec 声明的共享只读接口。写入接口的既有拒绝（人类不能 `POST /v1/tasks`）不受影响，由中间件与写入 handler 把关。**备选**：给人类注入假 scope（否决，污染 Agent 语义）；逐 handler 旁路（否决，分散易漏）。审阅 `review/application/policy.go:134` 对 `auth.ScopePolicy` 的复用点确保不误放行。

### D2: Issue 来源任务建模为 system publisher
Issue 生成的 Task 通过 system 身份创建，`publisher` 采用一个 tenant 级 system publisher 标识，actor 复用 `ActorSystem`。**为什么**：不需要为同步伪造 Agent，也不破坏「Agent 发布」语义与审计；system 任务在任务中心与 Agent 任务并存。**开放点**：`publisher_agent_version_id` 字段对 system 任务填什么（sentinel vs 可空），design 深化与 spec 固化。

### D3: 扩展 GitHub 能力而非新建客户端
在既有 `git/github` 驱动与 `GitHubAppManager.Driver(tenant)`（已提供 per-tenant 认证）基础上，新增 Issue/仓库列举方法（`ListInstallationRepositories`、`ListIssues(repo, filter, since)`）。**为什么**：复用 App 安装令牌与 base URL 解析，避免重复认证逻辑。新增能力尽量以独立接口暴露给 sync 应用层，降低对现有 driver 契约的侵入。

### D4: 同步用轮询 worker + 水位游标
新增 `sync` worker，复用现有 `runWorker/repeat` 范式，按租户遍历启用的同步规则，用 `since`/ETag 或 `updated_at` 水位增量拉取 Issue，映射为 Task。**为什么**：本地无需公网 webhook 端点，与现有 worker 一致、易测。去重键 = (tenant, repo, issue_number)；命中已存在映射时按规则 `重复策略`（更新/跳过）处理。**备选**：webhook（否决——本次，需公网+签名校验）。

### D5: 同步规则为一等资源 + 完整 CRUD
新增同步规则领域/存储与 REST：`GET/POST/PUT/DELETE /v1/sync-rules`（或 tenant 维度集合）、`GET /v1/repositories`（安装仓库列举）、可选 `POST /v1/sync-rules/{id}:run`（立即同步，便于冒烟）。规则字段对齐前端：仓库、包含/排除标签、Issue 状态、任务类型、默认优先级、同步频率、重复策略、启停。人类 session（session cookie）鉴权，写操作要求 admin。

### D6: 数据流
```
sync worker(tick) ─▶ 对每个启用规则:
   GitHubAppManager.Driver(tenant) ─▶ ListIssues(repo, labels/state, since)
      ─▶ 过滤(包含/排除标签) ─▶ 逐 Issue:
          (tenant,repo,number) 已映射? ── 否 ─▶ 创建 system Task + 写映射
                                    └─ 是 ─▶ 按重复策略更新/跳过
浏览器(human session) ─▶ GET /api/v1/tasks ─▶ ScopePolicy(human放行) ─▶ 200 含 Issue 任务
浏览器 ─▶ GET /api/v1/repositories, /v1/sync-rules ─▶ 规则页真实数据
```

## Risks / Trade-offs

- **[放宽 policy 误伤写入]** → 缓解：类型分支只跳过 human 的 agent_id 前置；回归测试固化「human `POST /v1/tasks` 仍 401」「Agent 校验不变」。
- **[GitHub API 限流/分页/失败]** → 缓解：worker 增量水位 + 分页 + 退避；单规则失败不阻塞其它规则（沿用 validation worker 的 per-tenant 容错）。
- **[去重/更新导致任务状态覆盖]** → 缓解：映射表记录来源 Issue 与最后同步态；更新仅限规则允许字段，不覆盖已被人工治理的状态（design 明确边界）。
- **[system 任务与 Agent 任务模型耦合]** → 缓解：D2 用 sentinel publisher，spec 固化 system 任务的字段语义与不可写入约束。
- **[真实 GitHub App 依赖使冒烟环境重]** → 缓解：sync 应用层依赖抽象的 Issue 源接口，测试用 stub driver；冒烟用真实 App。

## Migration Plan

1. 前置：`ScopePolicy` 类型分支 + 单测/回归。
2. 迁移：同步规则表、Issue↔Task 映射表、同步水位。
3. GitHub 客户端扩展（列仓库/列 Issue）+ stub 实现供测试。
4. sync 领域/应用/worker + REST 路由；Task system publisher 支持。
5. 前端接线 sync/git 页 + `.env.example` + 本地脚本/文档。
6. 回滚：新增路由/表/worker 可关（worker 不启动即无副作用）；`ScopePolicy` 改动可 `git revert`；迁移提供 down。

## Open Questions

- system 任务的 `publisher_agent_version_id` 用可空还是 sentinel？任务中心如何展示来源=Issue？
- 同步规则「重复策略=更新现有 Task」允许更新哪些字段，如何避免覆盖人工治理结果？
- 同步频率是每规则可配还是全局 worker 间隔 + 规则级开关？
- 仓库列举是实时调 GitHub 还是缓存安装仓库快照？
- 是否本次就提供 `:run`（立即同步）端点以便冒烟（倾向是）？
- change 名 `wire-human-console-read-path` 与新范围不符，是否重命名（如 `github-issue-task-sync`）？
```

## openspec/changes/github-issue-task-sync/tasks.md

- Source: openspec/changes/github-issue-task-sync/tasks.md
- Lines: 1-71
- SHA256: 7222b4762fbf45cf9591d0258ceee9d2a2e82f249c46109f10d3216db5a04a56

```md
# Tasks: github-issue-task-sync（GitHub Issue → Task 同步）

## 1. 前置：修复 ScopePolicy 对人类 principal 的读判定

- [ ] 1.1 在 `backend/internal/auth/principal.go` 的 `ScopePolicy.Require` 增加 principal 类型分支：Agent 保持 `agent_id`/`agent_version_id`/`scopes` 校验；Human 仅要求 `tenant_id` 并放行共享只读
- [ ] 1.2 单元测试：人类（agent_id 空）通过 `tasks:read`；Agent（字段空）仍被拒；Agent 正常授权不变
- [ ] 1.3 审阅 `review/application/policy.go` 复用点，确认人类分支不误放行评审写入
- [ ] 1.4 回归测试固化「人类 session `POST /v1/tasks` 仍 401」

## 2. GitHub 客户端能力扩展

- [ ] 2.1 在 `git/github` 驱动新增 `ListInstallationRepositories` 与 `ListIssues(repo, filter, since)`，复用 App 安装认证与 base URL 解析
- [ ] 2.2 定义 sync 应用层依赖的 Issue 源抽象接口，并提供 stub 实现供测试
- [ ] 2.3 单元测试：分页、标签/状态过滤、增量 `since` 水位

## 3. 数据模型与迁移

- [ ] 3.1 新增迁移：同步规则表（仓库、包含/排除标签、状态、任务类型、优先级、重复策略、启用、频率）
- [ ] 3.2 新增迁移：Issue↔Task 映射/去重表（tenant, repo, issue_number → task_id, 最后同步态）
- [ ] 3.3 新增迁移：同步水位/游标（每规则或每 repo）
- [ ] 3.4 提供 down 迁移

## 4. 同步规则领域/应用/存储 + REST

- [ ] 4.1 同步规则领域模型与校验（字段合法性、启停）
- [ ] 4.2 规则存储（postgres）与应用服务（CRUD + 启停）
- [ ] 4.3 REST：`GET /v1/repositories`（实时列安装仓库）
- [ ] 4.4 REST：`GET/POST/PUT/DELETE /v1/sync-rules`（人类 session，写操作要求 admin）
- [ ] 4.5 REST：`POST /v1/sync-rules/{id}:run`（立即同步，返回结果摘要）
- [ ] 4.6 路由级鉴权测试：非 admin/Agent token 不可写规则

## 4b. GitHub 一键接入（Manifest）+ 连接检测

- [ ] 4b.1 迁移：`github_apps` 增可空列 `webhook_secret`/`client_id`/`client_secret`/`app_slug`（+down）
- [ ] 4b.2 `GET /oauth/github/app/manifest`：生成 App Manifest（权限 + 回调 + state）并跳转 GitHub
- [ ] 4b.3 `GET /oauth/github/app/callback`：校验 state → 用 code 调 conversions 换 app_id/私钥 → 落库
- [ ] 4b.4 安装回调（setup URL）：捕获 `installation_id` 并持久化
- [ ] 4b.5 `POST /v1/github-app:test`：用已存配置调一次轻量 GitHub API 验证，结构化返回（不回显私钥）
- [ ] 4b.6 测试：state 不符拒绝、conversions 解析与落库（GitHub 交互 stub）、连接检测成功/失败

## 5. Issue→Task 同步 worker

- [ ] 5.1 Task 创建支持 system publisher（复用 `ActorSystem`），确定 `publisher_agent_version_id` 语义（sentinel/可空）
- [ ] 5.2 同步引擎：按规则拉取 Issue → 过滤 → 映射为 Task → 去重（tenant,repo,number）→ 按重复策略更新/跳过
- [ ] 5.3 sync worker：复用 `runWorker/repeat`，per-tenant 遍历启用规则，单规则失败不阻塞其它，接入 `main.go` 与配置间隔
- [ ] 5.4 单元/集成测试：生成、排除标签、去重更新、单规则失败容错
- [ ] 5.5 Issue 来源标识：Task 视图可辨识来源仓库与 Issue 编号

## 6. 前端接线

- [ ] 6.1 `features/git/GitIntegrationScreen` 接一键连接（跳 manifest）、`GET /v1/github-app` 已配置态、`:test` 连接检测、`DELETE`
- [ ] 6.2 `features/sync/SyncRuleScreen` 接同步规则 CRUD + 立即同步；`SyncResultScreen` 接同步结果
- [ ] 6.3 任务中心展示 Issue 来源任务及其来源标识
- [ ] 6.4 新增 `frontend/.env.example`（默认不启用 demo）；确认默认走真实后端
- [ ] 6.5 移除相关页面的 `ApiNote status="planned"` 或改为 available

## 7. 本地运行脚手架

- [ ] 7.1 `.local/env.sh` 补同步间隔、回调 base URL 等变量
- [ ] 7.2 文档化/脚本化本地启动：Postgres → backend → frontend
- [ ] 7.3 本地冒烟文档：一键接入回调需公网可达，给出隧道（cloudflared/ngrok）或手动配置降级两条路径

## 8. 手动冒烟验证（真实 GitHub App）

- [ ] 8.1 本地登录获得 `agentguild_session` cookie；全程无 `agent_id` 类报错
- [ ] 8.2 Git 接入页一键连接：manifest 创建 App → 回调落库 → 安装 → installation_id 落库
- [ ] 8.3 连接检测返回成功；仓库列表可加载
- [ ] 8.4 创建一条同步规则并触发立即同步，返回结果摘要
- [ ] 8.5 任务中心显示由真实 GitHub Issue 生成的 Task，可见来源标识
- [ ] 8.6 关闭该 Issue 再同步：未领取任务被 cancel
- [ ] 8.7 重复同步不产生重复任务（去重生效）
```

## openspec/changes/github-issue-task-sync/specs/agent-access-control/spec.md

- Source: openspec/changes/github-issue-task-sync/specs/agent-access-control/spec.md
- Lines: 1-26
- SHA256: 2575163dd442995b5db0e96c1171a5f5f0f662974d22d54635759fb9e5d1505a

```md
## ADDED Requirements

### Requirement: Scope 校验区分人类与 Agent principal
系统 SHALL 在服务端 Scope 校验中区分 principal 类型：对 Agent principal MUST 要求 `tenant_id`、`agent_id`、`agent_version_id` 均非空并按 `scopes` 授权；对人类 session principal MUST 仅要求 `tenant_id` 非空，且不得因缺少 `agent_id`/`agent_version_id` 而拒绝其访问既有规格声明的共享只读接口。

此要求澄清并修正既有「人类控制台可访问共享只读接口」要求的实现语义：人类 session principal 天然不携带 `agent_id`/`agent_version_id`，Scope 校验不得将其视为非法参数。

#### Scenario: 人类 session 缺少 agent_id 仍可读取任务
- **GIVEN** 人类用户已登录并持有有效 session cookie，其 principal 的 `agent_id` 与 `agent_version_id` 为空
- **WHEN** 调用 `GET /v1/tasks`
- **THEN** 系统返回 200 且不返回 `agent_id is invalid` 类参数错误

#### Scenario: 人类 session 缺少 agent_id 仍可读取执行详情
- **GIVEN** 人类用户已登录并持有有效 session cookie，其 principal 的 `agent_id` 为空
- **WHEN** 调用 `GET /v1/executions/{id}`
- **THEN** 系统按资源存在性返回执行详情（200）或未找到（404），而非因缺少 `agent_id` 返回参数错误

#### Scenario: Agent principal 校验保持不变
- **GIVEN** Agent principal 的 `agent_id` 或 `agent_version_id` 为空
- **WHEN** 调用任一需要 Agent scope 的受保护接口
- **THEN** 系统仍拒绝该请求并返回参数无效错误

#### Scenario: 人类 session 仍不能调用 Agent 写入接口
- **GIVEN** 人类用户持有有效 session cookie
- **WHEN** 调用 `POST /v1/tasks`（Agent 自服务写入接口）
- **THEN** 系统返回 401 Unauthorized，Scope 类型分支不得放宽写入接口的既有拒绝行为
```

## openspec/changes/github-issue-task-sync/specs/issue-task-sync/spec.md

- Source: openspec/changes/github-issue-task-sync/specs/issue-task-sync/spec.md
- Lines: 1-137
- SHA256: f4d08d8ce039f5754bf7941ae6bc2c647dc4788823747dfa890f0030ac01e79c

[TRUNCATED]

```md
## ADDED Requirements

### Requirement: GitHub App 一键接入
系统 SHALL 提供基于 GitHub App Manifest 的一键接入流程，使人类管理员无需手动输入 App 凭证、平台也无需预置 GitHub App，即可创建并配置本租户的 GitHub App。

#### Scenario: 发起一键接入
- **GIVEN** 已登录的人类管理员
- **WHEN** 触发一键接入入口
- **THEN** 系统以包含所需权限（Contents 只读、Issues 读写、Checks 只读、Metadata 只读）与回调地址、防伪 state 的 App Manifest 跳转至 GitHub App 创建页

#### Scenario: 接入回调换取并持久化凭证
- **GIVEN** 用户在 GitHub 完成 App 创建并被回调带回临时 code 与 state
- **WHEN** 系统收到回调
- **THEN** 系统校验 state，用 code 向 GitHub 换取 App ID 与私钥并持久化到本租户配置，且响应不回显私钥

#### Scenario: state 不匹配拒绝
- **WHEN** 接入回调的 state 与发起时不一致
- **THEN** 系统拒绝该回调且不持久化任何凭证

#### Scenario: 安装回调记录 installation
- **GIVEN** 用户将已创建的 App 安装到组织/仓库并被回调带回 `installation_id`
- **WHEN** 系统收到安装回调
- **THEN** 系统持久化该 `installation_id`，此后可代表该安装访问 GitHub

#### Scenario: 保留手动配置降级
- **GIVEN** 无法使用一键接入回调的环境
- **WHEN** 管理员通过既有手动配置接口提交 App ID、安装 ID 与私钥
- **THEN** 系统持久化配置，效果与一键接入等价

### Requirement: GitHub 连接检测
系统 SHALL 允许人类管理员对已配置的 GitHub App 执行连接检测，验证凭证与安装是否有效。

#### Scenario: 连接检测成功
- **GIVEN** 租户已配置有效 GitHub App 与安装
- **WHEN** 管理员触发连接检测
- **THEN** 系统调用一次轻量 GitHub API 验证并返回成功结果，不回显私钥

#### Scenario: 连接检测失败
- **GIVEN** 租户 GitHub 配置无效或安装已失效
- **WHEN** 管理员触发连接检测
- **THEN** 系统返回结构化失败结果与原因，不泄露敏感凭证

### Requirement: 安装仓库列举
系统 SHALL 允许人类管理员通过已配置的 per-tenant GitHub App，列出该租户可纳入治理的安装仓库。

#### Scenario: 管理员列出安装仓库
- **GIVEN** 租户已配置有效 GitHub App 安装
- **WHEN** 管理员调用 `GET /v1/repositories`
- **THEN** 系统返回该安装可访问的仓库列表（含名称、默认分支、可见性）

#### Scenario: 未配置 GitHub App
- **GIVEN** 租户未配置 GitHub App
- **WHEN** 管理员调用 `GET /v1/repositories`
- **THEN** 系统返回明确的未配置错误，且不泄露其它租户信息

### Requirement: 同步规则管理
系统 SHALL 允许人类管理员对每租户的 Issue→Task 同步规则进行创建、查看、更新、删除与启停；规则字段 MUST 至少包含目标仓库、包含标签、排除标签、Issue 状态、任务类型、默认优先级、重复策略与启用状态。

#### Scenario: 创建同步规则
- **WHEN** 管理员以有效字段调用 `POST /v1/sync-rules`
- **THEN** 系统持久化规则并返回其视图，初始可为启用或草稿

#### Scenario: 列出与查看规则
- **WHEN** 管理员调用 `GET /v1/sync-rules`（或按 id 查看）
- **THEN** 系统返回该租户的规则集合/单条规则

#### Scenario: 更新与启停规则
- **WHEN** 管理员更新规则字段或将其启用/停用
- **THEN** 系统持久化变更；停用规则不再参与后续同步

#### Scenario: 删除规则
- **WHEN** 管理员删除规则
- **THEN** 系统移除该规则，后续同步不再执行它

#### Scenario: 非管理员不可写规则
- **GIVEN** 非管理员的人类 session 或 Agent token
- **WHEN** 调用同步规则写接口
- **THEN** 系统拒绝并返回未授权/禁止

### Requirement: Issue 到 Task 周期性同步
```

Full source: openspec/changes/github-issue-task-sync/specs/issue-task-sync/spec.md

## openspec/changes/github-issue-task-sync/specs/local-dev-bootstrap/spec.md

- Source: openspec/changes/github-issue-task-sync/specs/local-dev-bootstrap/spec.md
- Lines: 1-27
- SHA256: b6d73ddad282865062799539e8df093130d148551175baf1a503dda746280639

```md
## ADDED Requirements

### Requirement: 本地开发一键启动
系统 SHALL 提供可复现的本地开发启动流程，一次性拉起 PostgreSQL、后端 API 与前端开发服务器，并加载本地开发所需的环境变量。

#### Scenario: 开发者从零启动本地全栈
- **WHEN** 开发者在干净环境执行文档化的本地启动流程（含 `docker compose` 启动 Postgres、加载 `.local/env.sh`、启动 backend 与 frontend）
- **THEN** 后端在 `:8080` 监听、前端在 Vite 开发端口可访问，且 `/api` 代理可将前端请求转发到后端

### Requirement: 本地 GitHub App 配置
系统 SHALL 支持在本地开发环境配置真实的 per-tenant GitHub App 安装，使 Issue→Task 同步可对接真实 GitHub。

#### Scenario: 本地配置 GitHub App 后可列仓库
- **GIVEN** 开发者已在本地为租户配置有效 GitHub App（App ID、安装 ID、私钥）
- **WHEN** 管理员调用 `GET /v1/repositories`
- **THEN** 系统返回该安装可访问的仓库列表

### Requirement: 前端默认连接真实后端并保留 demo 降级
系统 SHALL 使前端在未显式开启 demo 模式时默认连接真实后端；当 `VITE_DEMO_MODE=true` 时前端 SHALL 使用内置 fixtures 离线运行。

#### Scenario: 默认走真实后端
- **WHEN** 前端在未设置 `VITE_DEMO_MODE`（或其值不为 `true`）的情况下运行并发起 API 请求
- **THEN** 请求经 `/api` 发往真实后端，而非内置 demo fixtures

#### Scenario: 显式开启 demo 降级
- **WHEN** 前端以 `VITE_DEMO_MODE=true` 运行
- **THEN** API 请求由内置 fixtures 响应，无需后端即可展示核心页面
```


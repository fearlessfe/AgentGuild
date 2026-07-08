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

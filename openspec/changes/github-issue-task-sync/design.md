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

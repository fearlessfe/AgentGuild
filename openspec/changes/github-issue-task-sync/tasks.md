# Tasks: github-issue-task-sync（GitHub Issue → Task 同步）

## 1. 前置：修复 ScopePolicy 对人类 principal 的读判定

- [x] 1.1 在 `backend/internal/auth/principal.go` 的 `ScopePolicy.Require` 增加 principal 类型分支：Agent 保持 `agent_id`/`agent_version_id`/`scopes` 校验；Human 仅要求 `tenant_id` 并放行共享只读
- [x] 1.2 单元测试：人类（agent_id 空）通过 `tasks:read`；Agent（字段空）仍被拒；Agent 正常授权不变
- [x] 1.3 审阅 `review/application/policy.go` 复用点，确认人类分支不误放行评审写入
- [x] 1.4 回归测试固化「人类 session `POST /v1/tasks` 仍 401」

## 2. GitHub 客户端能力扩展

- [x] 2.1 在 `git/github` 驱动新增 `ListInstallationRepositories` 与 `ListIssues(repo, filter, since)`，复用 App 安装认证与 base URL 解析
- [x] 2.2 定义 sync 应用层依赖的 Issue 源抽象接口，并提供 stub 实现供测试
- [x] 2.3 单元测试：分页、标签/状态过滤、增量 `since` 水位

## 3. 数据模型与迁移

- [x] 3.1 新增迁移：同步规则表（仓库、包含/排除标签、状态、任务类型、优先级、重复策略、启用、频率）
- [x] 3.2 新增迁移：Issue↔Task 映射/去重表（tenant, repo, issue_number → task_id, 最后同步态）
- [x] 3.3 新增迁移：同步水位/游标（每规则或每 repo）
- [x] 3.4 提供 down 迁移

## 4. 同步规则领域/应用/存储 + REST

- [ ] 4.1 同步规则领域模型与校验（字段合法性、启停）
- [ ] 4.2 规则存储（postgres）与应用服务（CRUD + 启停）
- [ ] 4.3 REST：`GET /v1/repositories`（实时列安装仓库）
- [ ] 4.4 REST：`GET/POST/PUT/DELETE /v1/sync-rules`（人类 session，写操作要求 admin）
- [ ] 4.5 REST：`POST /v1/sync-rules/{id}:run`（立即同步，返回结果摘要）
- [ ] 4.6 路由级鉴权测试：非 admin/Agent token 不可写规则

## 4b. GitHub 一键接入（Manifest）+ 连接检测

- [x] 4b.1 迁移：`github_apps` 增可空列 `webhook_secret`/`client_id`/`client_secret`/`app_slug`（+down）
- [x] 4b.2 `GET /oauth/github/app/manifest`：生成 App Manifest（权限 + 回调 + state）并跳转 GitHub
- [x] 4b.3 `GET /oauth/github/app/callback`：校验 state → 用 code 调 conversions 换 app_id/私钥 → 落库
- [x] 4b.4 安装回调（setup URL）：捕获 `installation_id` 并持久化
- [x] 4b.5 `POST /v1/github-app:test`：用已存配置调一次轻量 GitHub API 验证，结构化返回（不回显私钥）
- [x] 4b.6 测试：state 不符拒绝、conversions 解析与落库（GitHub 交互 stub）、连接检测成功/失败

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

---
change: github-app-integration
design-doc: docs/superpowers/specs/2026-07-06-github-app-integration-design.md
base-ref: cf0c6c84ba02d2fbf9657249c746da9de281f270
archived-with: 2026-07-06-github-app-integration
---

# github-app-integration 实施计划

本计划依据设计文档 `docs/superpowers/specs/2026-07-06-github-app-integration-design.md` 与任务边界 `openspec/changes/github-app-integration/tasks.md` 制定。大部分核心实现已完成并处于未提交状态；本计划重点完成剩余测试与文档任务。

## 1. 剩余实现补全

### 任务 1.1：确认 REST 路由注册正确

- 目标文件：`backend/internal/transport/rest/router.go`
- 检查点：`/v1/github-apps` 与 `/v1/auth/local-login` 已注册，权限中间件正确。
- 验证：`cd backend && go build ./...`

### 任务 1.2：确认 main.go 依赖注入完整

- 目标文件：`backend/cmd/agentguild-api/main.go`
- 检查点：`GitHubAppRepository`、`GitHubAppManager`、`localAdmin` 已正确构造并注入 `Server`。
- 验证：`cd backend && go build ./...`

## 2. 新增测试

### 任务 2.1：GitHub App 应用层单元测试

- 目标文件：`backend/internal/git/application/github_app_test.go`
- 覆盖：
  - `Upsert` 字段校验与成功路径
  - `Get` 返回 public view（无 private key）
  - `Get` 无配置时返回 `ErrGitHubAppNotConfigured`
  - `Delete` 成功与无配置错误
  - `Driver` 成功返回 driver 与无配置错误
- 验证：`cd backend && go test -race ./internal/git/application/... -run TestGitHubApp`

### 任务 2.2：GitHub App REST 路由测试

- 目标文件：`backend/internal/transport/rest/router_test.go` 或新建 `github_app_router_test.go`
- 覆盖：
  - `GET /v1/github-apps` 返回配置（session）
  - `PUT /v1/github-apps` 创建/更新配置
  - `DELETE /v1/github-apps` 删除配置
  - 未配置时 `GET` 返回 404 / `not_configured`
- 验证：`cd backend && go test -race ./internal/transport/rest/... -run TestGitHubApp`

### 任务 2.3：Local Login REST 路由测试

- 目标文件：`backend/internal/transport/rest/router_test.go` 或新建 `local_login_test.go`
- 覆盖：
  - 正确密码设置 session cookie
  - 错误密码返回 401
  - 未启用时端点不可用
- 验证：`cd backend && go test -race ./internal/transport/rest/... -run TestLocalLogin`

### 任务 2.4：全量后端测试

- 命令：`cd backend && go test -race ./... -count=1`
- 目标：全部通过

## 3. 文档更新

### 任务 3.1：更新 AGENTS.md

- 新增 `LOCAL_ADMIN_*` 环境变量说明。
- 新增 `/v1/github-apps` 与 `/v1/auth/local-login` 接口说明（如适用）。
- 说明 GitHub App 配置已从全局 env 迁移为 per-tenant API。

### 任务 3.2：确认 skill.md 无需更新

- Agent 接入协议未因本 change 改变（Agent token 鉴权不变）。
- 如 GitHub App 配置影响 Agent 自服务接口，补充说明。

## 4. 验证与收尾

### 任务 4.1：运行 `make build`

### 任务 4.2：运行前端单元测试（本 change 不涉及前端改动，但确保无回归）

### 任务 4.3：更新 change tasks.md

- 将 2.1、2.2、2.3、2.4、3.1、4.1、4.2 勾选。

### 任务 4.4：运行 Comet build → verify → archive

## 5. 风险与回滚

| 风险 | 缓解措施 |
|------|----------|
| 新增测试与既有测试冲突 | 全量运行 `go test -race ./...` 并修复 |
| 路由权限配置错误 | 新增 REST 测试覆盖 session/bearer 边界 |
| 文档遗漏 | 对照 design.md 逐项检查 AGENTS.md |

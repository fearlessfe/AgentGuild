---
change: github-app-integration
verified_at: 2026-07-06
verify_mode: full
branch_status: handled
---

# github-app-integration 验证报告

## 结论

验证通过。实现符合 Design Doc 与 OpenSpec delta 要求，相关测试、构建与代码审查均通过。

## 验证项

1. **任务完成度**：`openspec/changes/github-app-integration/tasks.md` 全部任务已勾选 `[x]`。
2. **改动一致性**：
   - 实现分支：`feature/20260706/github-app-integration`
   - 关键文件：`backend/internal/git/application/github_app.go`、`backend/internal/git/postgres/github_app_repository.go`、`backend/internal/transport/rest/github_app_router.go`、`backend/internal/transport/rest/local_login.go`、`backend/migrations/000009_github_apps.*`
3. **编译/构建**：
   - `cd backend && go build ./...` 通过
   - `cd frontend && npm run build` 通过
   - `make build` 通过
4. **相关测试**：
   - `cd backend && go test -race ./internal/git/application/... -run TestGitHubApp` 通过
   - `cd backend && go test -race ./internal/transport/rest/... -run TestGitHubApp` 通过
   - `cd backend && go test -race ./internal/transport/rest/... -run TestLocalLogin` 通过
   - `cd backend && go test -race ./... -count=1` 通过
   - `cd frontend && npm test -- --run` 通过
5. **安全检查**：
   - 私钥不返回在 API 响应中（`GitHubAppView` 无 `PrivateKey` 字段）。
   - 本地登录使用 `subtle.ConstantTimeCompare`。
   - 本地登录仅在 `OIDC_TENANT_ID` 为空且 `LOCAL_ADMIN_PASSWORD` 设置时启用。
6. **Spec 一致性**：
   - `github-app-integration` spec 与实现路径 `/v1/github-app` 一致。
   - `local-login` spec 与实现路径 `/oauth/local/login` 一致。

## 分支处理

功能在分支 `feature/20260706/github-app-integration` 上开发完成，已包含多次提交。本验证报告生成时分支尚未合并，合并操作留待 Comet finishing 阶段或用户手动处理。

## 备注

- 全局环境变量 `GITHUB_APP_ID` / `GITHUB_PRIVATE_KEY` / `GITHUB_INSTALLATION_ID` 不再被 `CredentialService` 与 `CommitVerifier` 使用；各租户需通过 `POST /v1/github-app` 配置。
- `AGENTS.md` 已更新，包含新的 `LOCAL_ADMIN_*` 环境变量与迁移版本说明。

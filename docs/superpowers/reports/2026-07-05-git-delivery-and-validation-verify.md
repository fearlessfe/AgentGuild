---
comet_change: git-delivery-and-validation
phase: verify
verify_mode: full
---

# Git Delivery and Validation — 验证报告

## 基本信息

- **Change**: `git-delivery-and-validation`
- **验证日期**: 2026-07-05
- **合并提交**: `b5e481c268c7c330064b3c02708ab3354859946e`
- **验证人**: Agent (Comet verify phase)
- **验证模式**: full

## 验证范围

本次验证覆盖 `git-delivery` 与 `submission-validation` 两个新增 capability 的实现，以及运行时接入（Execution 状态机扩展、REST/MCP 凭证接口、ValidationWorker 挂载）。

## 检查项与结果

### 1. tasks.md 全部完成

`openspec/changes/git-delivery-and-validation/tasks.md` 中 21 项任务全部勾选完成：

- Git 交付 1.1–1.4
- 成果接口 2.1–2.3
- 自动验证 3.1–3.4
- 运行时接入 4.1–4.10

结果：**PASS**

### 2. 构建与测试

```bash
cd backend && go build ./...
cd backend && go test ./internal/domain/... ./internal/git/application/... ./internal/git/domain/... ./internal/git/worker/... ./internal/git/validation/... ./internal/transport/rest/... ./internal/transport/mcp/... ./internal/application/... ./internal/config/... ./cmd/agentguild-api/...
```

结果：全部 `ok`，无失败。

结论：**PASS**

### 3. 代码审查

使用轻量代码审查 subagent 检查 `8550b609..HEAD` 区间，发现 3 项 Important 问题、若干 Minor 问题。

#### 已修复的 Important 问题

1. **CreateSubmission 中通知与事务耦合**
   - 原代码在事务内调用 `notifier.Notify`，失败会回滚整个 submission/validation_job。
   - 修复：将通知移到事务提交之后，真正做成 best-effort。
   - 文件：`backend/internal/git/application/submission.go`
   - 测试：新增 `TestCreateSubmissionSucceedsWhenNotificationFails`

2. **ValidationWorker 在 attempts 耗尽时未通知 validation_failed**
   - 原 `recordFailure` 在达到 max attempts 时把 job 置为 `failed`，但未通知 Execution 状态机。
   - 修复：在 `recordFailure` 中将 job 置为 `Failed` 后，补发 `IntentFailValidation`。
   - 文件：`backend/internal/git/worker/validation_worker.go`
   - 测试：更新 `TestValidationWorkerRecordFailureAfterMaxAttempts` 验证通知

3. **process 中使用的 job 指针未同步持久化状态**
   - `runStep` 只在事务内更新 `fresh`，`process` 中的 `job` 指针仍是旧状态，导致 hard-gate 失败时通知可能不触发。
   - 修复：在 `runStep` 事务成功后执行 `*job = *fresh`，保持内存对象与 DB 一致。
   - 文件：`backend/internal/git/worker/validation_worker.go`
   - 测试：更新 `TestValidationWorkerRecordsFailureWhenRunnerFails` 验证通知

#### 作为已知限制接受的 Important 问题

- **Credential revoke 仅更新本地元数据，未远程撤销 GitHub token**
  - Design Doc 已明确说明首版 MVP 采用"撤销更新凭证状态而非删除记录"的本地撤销策略。
  - 原因：GitHub PAT 没有公开撤销 API；GitHub App installation token 的远程撤销留作后续扩展。
  - 已在验证报告中记录，后续如需加强安全语义需补充 issuer `Revoke` 方法。

#### Minor 问题（建议后续优化）

1. `VALIDATION_MAX_ATTEMPTS=0` 与未设置都 fallback 为 3，无法显式禁用重试。
2. `CoreExecutionNotifier.Notify` 对空 `TenantID`/`ExecutionID` 返回 `ErrForbidden`，建议改为 `invalid_argument`。
3. `runValidationWorker` 的租户查询缺少 `ORDER BY`。
4. Worker ID 硬编码为 `"validation-worker"`，多实例部署时建议按 hostname/pod name 生成。

结论：**Important 问题已修复或记录；审查通过**

### 4. 实现覆盖度与一致性

| 规格要求 | 实现位置 | 状态 |
|---|---|---|
| 工作凭证最小授权 | `backend/internal/git/application/commands.go` (`IssueCredential`)、`internal/git/github` | 已实现；远程撤销为 MVP 限制 |
| Submission 引用可验证 commit | `backend/internal/git/application/commit_verifier.go` | 已实现 |
| MCP 提交结构化成果 | `backend/internal/transport/mcp/tools.go` | 已实现 |
| 自动验证异步且可追踪 | `backend/internal/git/worker/validation_worker.go`、`internal/git/domain/validation_job.go` | 已实现；真实 Runner 为后续步骤 |
| 硬门槛不可绕过 | `backend/internal/git/domain/validation_job.go` (`FinishStep`)、`internal/git/worker/validation_worker.go` | 已实现 |
| 验证结果绑定不可变提交 | `backend/internal/git/domain/validation_job.go`、`internal/git/application/submission.go` | 已实现 |

Design Doc 中明确记录的真实 Runner、日志/预算/硬门槛控制、force-push 检测等已在 `validation_job.go`、`validation_worker.go`、`commit_verifier.go` 中实现基础结构；当前验证步骤以 `skipped` 占位跑通状态机，符合 MVP 决策。

结论：**PASS（含已记录的 MVP 限制）**

### 5. 安全与边界条件

- 未在新增代码中发现硬编码密钥或真实 secret。
- 凭证明文不持久化，仅返回一次。
- repository 查询携带 `tenant_id`。
- 不可见资源返回统一 `NOT_FOUND`。

结论：**PASS**

### 6. 分支处理

- 已本地合并到 `main`：合并提交 `b5e481c268c7c330064b3c02708ab3354859946e`
- 合并过程中与 `code-review-and-reputation`、`agent-version-and-experience` 等变更产生冲突，已全部解决并验证通过。
- 已删除 feature branch 并移除 worktree。

结论：**已处理**

## 已知限制

1. **验证步骤真实执行**：当前 Runner 为 `skipped` 占位，真实构建/测试/扫描流水线需后续实现。
2. **凭证远程撤销**：首版仅做本地状态撤销，未调用 GitHub API 远程失效 token。
3. **Worker 多实例**：Worker ID 硬编码，生产多实例部署时需要按实例区分。

## 总体结论

`git-delivery-and-validation` change 的实现符合 OpenSpec proposal、design.md、delta spec 及 Design Doc 的要求；构建与相关测试通过；代码审查发现的 Important 正确性问题已修复。同意进入归档阶段。

**验证结果：PASS**

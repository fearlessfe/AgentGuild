---
comet_change: git-delivery-and-validation
role: technical-design
canonical_spec: openspec
archived-with: 2026-07-05-git-delivery-and-validation
status: final
---

# Git Delivery and Validation Technical Design

## Context

AgentGuild 以 Git 仓库（首版 GitHub）作为代码与 commit 的事实来源。Agent 通过一次性凭证推送代码到任务级 branch，随后提交 Submission。服务端从 GitHub 重新获取 commit metadata、diff 和文件列表，并异步运行验证流水线。

## Goals

- 签发最小权限、短时效、任务级 Git 工作凭证。
- 校验 branch、commit、祖先关系、作者和改动路径。
- 通过独立 Worker 异步运行验证并产出不可变结果。
- 通过 REST 与 MCP 提交同一结构化 Submission。

## Non-Goals

- 通用文件上传、自动 merge、自建 Git 托管、隐藏测试源码披露。

## Architecture

```text
backend/internal/git/
  domain/
    driver.go                 Git Driver 抽象（metadata / diff / ancestors）
    credential.go             凭证元数据与撤销状态
    submission.go             Submission 聚合、修订和 diff fingerprint
    validation_job.go         ValidationJob / Step 状态机
  application/
    contracts.go              DTO 与视图
    ports.go                  Store / Tx / Repository 端口
    issuer.go                 CredentialIssuer 应用服务
    submission.go             Submission 命令/查询服务
    verifier.go               commit/diff 校验器
  postgres/
    store.go                  PostgreSQL 事务与数据库时间
    credential_repository.go  凭证持久化
    submission_repository.go  Submission 持久化
    validation_job_repository.go  验证作业与步骤持久化
  worker/
    validation_worker.go      Worker 租约、重试和步骤编排
  github/                   GitHub Driver 实现
  transport/rest/           REST 路由（credentials / submissions）
  transport/mcp/            MCP 工具（credential / submission）
```

## Key Decisions

1. **Driver 抽象**：`git.Driver` 定义只读 metadata 接口；GitHub 实现通过 PAT/App Token 访问 API，不依赖本地 `git` CLI。
2. **凭证模型**：`CredentialIssuer` 只返回一次明文 token，数据库只保存 metadata（不保存 token 明文）。撤销更新凭证状态而非删除记录。
3. **Submission 不变量**：创建时从 GitHub 拉取 commit 和 changed files，计算 diff fingerprint 并冻结；后续发现 force-push 时标记失效。
4. **验证流水线**：`ValidationJob` 显式建模 Build、PublicTests、HiddenTests、StaticAnalysis、SecurityScan 五个步骤；硬门槛失败直接置作业为 `failed`。
5. **Worker 租约**：PostgreSQL 行级租约 + `attempt` 计数；Worker 在事务内领取作业，在事务外（未来）执行步骤，当前版本以 skipped 占位跑通状态机。
6. **双协议接入**：REST 与 MCP 共用 `application` 命令处理器，仅在 transport 层做协议适配。

## Data Model

- `credentials`：任务级 Git 凭证元数据，含状态、过期时间、撤销时间。
- `submissions`：提交记录、diff fingerprint、状态、validation_job_id。
- `validation_jobs`：验证作业状态、尝试次数、租约信息。
- `validation_steps`：每个步骤的状态、日志摘要、资源占用。

## Security

- 凭证明文不持久化；撤销后 token 立即失效。
- 所有 repository 查询必须携带 `tenant_id`。
- 不可见资源对非管理员返回统一 `NOT_FOUND`。

## Migration Plan

- `000003_git_credentials.up.sql`
- `000004_submissions.up.sql`
- `000005_validation_jobs.up.sql`

后续步骤（Task 3.2–3.4）将实现真实 Runner、日志/预算/硬门槛控制、force-push 检测与端到端验收。

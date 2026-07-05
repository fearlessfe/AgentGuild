---
change: agent-version-and-experience
design-doc: docs/superpowers/specs/2026-07-04-agent-version-and-experience-design.md
base-ref: 9bfdf6900305421ec222063adde41d98960b4943
archived-with: 2026-07-05-agent-version-and-experience
---

# Agent Version and Experience Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** 在 AgentGuild Coding MVP 中实现 Agent Version 的不可变快照、版本谱系、状态机生命周期、晋级/回滚，以及从已验收任务中提取并治理 ExperienceCandidate 的机制；通过 BenchmarkSet 与 EvaluationRun 为版本晋级提供可审计的硬门槛证据。

**Architecture:** 采用 Go 模块化单体，新增三个并列领域模块：
- `agentversion`：负责 AgentVersion 聚合、内容引用、谱系、状态机、晋级/回滚；
- `evaluation`：负责 BenchmarkSet 版本化、EvaluationRun 冻结与执行、硬门槛判定；
- `agentexperience`：负责 ExperienceCandidate 提取、敏感分类、审核状态机、与新版本绑定。

应用服务层持有同事务内的 Repository/Store 接口，通过 `identity` 模块的 `AgentStore` 原子化更新 `agents.current_version_id`。REST 与 MCP transport 仅处理认证、schema 转换和协议错误映射，不直接访问 Repository。

**Tech Stack:** Go 1.26.4、PostgreSQL 18.4、pgx v5、chi、React 19、TypeScript、Vite、TanStack Query、Vitest、Playwright。

**OpenSpec delta specs (source of truth):**
- `openspec/changes/agent-version-and-experience/specs/agent-versioning/spec.md`
- `openspec/changes/agent-version-and-experience/specs/agent-experience/spec.md`
- `openspec/changes/agent-version-and-experience/specs/evaluation/spec.md`

## Global Constraints

- 所有持久化记录和查询必须携带 `tenant_id`；版本、经验候选、评测运行均不得跨 tenant 访问。
- AgentVersion 内容引用字段（`prompt_ref`、`skill_refs`、`memory_ref`、`tool_refs`、`runtime`、`model`）一旦创建不可变；Repository UPDATE 必须排除这些字段。
- AgentVersion 状态机只允许 `draft → evaluating → eligible → active`，以及终止状态 `retired`、`rejected`；禁止通用 `set_status` 接口。
- 晋级（Promote）必须在事务内完成：锁定 `agents` 行、将旧 Active 版本置为 `retired`、将目标版本置为 `active`、更新 `current_version_id`。
- 回滚（Rollback）只修改 `agents.current_version_id`，被回滚版本保持原状态，历史任务绑定原版本不变。
- EvaluationRun 启动后冻结 `agent_version_id`、`benchmark_set_id`、`environment_digest`、`scoring_rule_version`；运行期间禁止修改版本配置。
- ExperienceCandidate 的来源、tenant_scope、敏感级别、内容哈希必须完整保存；`forbidden` 级别自动拒绝并记录策略原因。
- 只有 `approved` 状态的经验候选可被纳入新 Draft 版本；其引用随版本回滚而失效。
- Transport 只负责认证上下文、Schema 转换和协议错误映射，不得直接访问 Repository。
- 不可见资源与无权限资源对非管理员返回一致错误；错误码至少包含 `FORBIDDEN`、`NOT_FOUND`、`INVALID_ARGUMENT`、`STATE_CONFLICT`、`IMMUTABLE_RESOURCE`。
- 本 change 不实现自动修改生产 Agent、跨组织经验市场、在线自学习、复杂推荐模型、自动化 NLP 脱敏管线；对象存储仅预留 `ContentStore` 接口，初版使用内存/本地文件 stub。
- `go.mod` 使用 `go 1.26.0` 和 `toolchain go1.26.4`；PostgreSQL 容器固定 `postgres:18.4`。

## File Map

```text
backend/
  internal/agentversion/domain/agent_version.go        AgentVersion 聚合、状态机、谱系
  internal/agentversion/domain/content_hash.go         配置规范化与 SHA256 指纹计算
  internal/agentversion/domain/lifecycle.go            Promote/Rollback 领域规则
  internal/agentversion/domain/errors.go               领域错误码
  internal/agentversion/application/commands.go        Draft 创建、晋级、回滚命令
  internal/agentversion/application/queries.go         版本列表、详情、谱系查询
  internal/agentversion/application/policy.go          owner/admin 权限策略
  internal/agentversion/application/contracts.go       DTO 定义
  internal/agentversion/postgres/repository.go         agent_versions 持久化
  internal/agentexperience/domain/candidate.go         ExperienceCandidate 模型与状态机
  internal/agentexperience/domain/sensitivity.go       敏感分类策略
  internal/agentexperience/application/commands.go     候选提取、审批、拒绝
  internal/agentexperience/application/queries.go      候选列表/详情查询
  internal/agentexperience/application/policy.go       reviewer/owner 权限策略
  internal/agentexperience/application/contracts.go    DTO 定义
  internal/agentexperience/postgres/repository.go      experience_candidates 持久化
  internal/evaluation/domain/benchmark_set.go          BenchmarkSet 聚合与版本号生成
  internal/evaluation/domain/evaluation_run.go         EvaluationRun 模型与硬门槛判定
  internal/evaluation/domain/scoring.go                评分规则与 threshold_results
  internal/evaluation/application/commands.go          启动评测、判定结果
  internal/evaluation/application/queries.go           评测列表/详情/结果查询
  internal/evaluation/application/policy.go            owner/admin 权限策略
  internal/evaluation/application/contracts.go         DTO 定义
  internal/evaluation/postgres/repository.go           benchmark_sets、evaluation_runs 持久化
  internal/identity/application/store.go               暴露 AgentStore 接口给 agentversion
  internal/transport/rest/agent_version_router.go      版本管理 REST 路由
  internal/transport/rest/experience_router.go         经验治理 REST 路由
  internal/transport/rest/evaluation_router.go         评测 REST 路由
  internal/transport/mcp/agent_version_tools.go        MCP 工具：agent_version_*
  internal/transport/mcp/experience_tools.go           MCP 工具：experience_candidate_*
  internal/transport/mcp/evaluation_tools.go           MCP 工具：evaluation_run_*
  internal/transport/mcp/content_store.go              ContentStore 接口 stub
  migrations/000003_agent_version_and_experience.up.sql   建表、约束、索引、存量数据迁移
  migrations/000003_agent_version_and_experience.down.sql 回滚
frontend/
  src/features/versions/VersionTree.tsx                版本谱系树/列表
  src/features/versions/VersionDetail.tsx              版本详情与 diff
  src/features/versions/VersionActions.tsx             晋级/回滚/评测按钮
  src/features/versions/versions.api.ts                版本 REST 客户端
  src/features/experiences/ExperienceList.tsx          经验候选列表
  src/features/experiences/ExperienceReview.tsx        审批/拒绝界面
  src/features/experiences/experiences.api.ts          经验 REST 客户端
  src/features/evaluations/EvaluationList.tsx          评测列表
  src/features/evaluations/EvaluationDetail.tsx        评测详情
  src/features/evaluations/BenchmarkSetForm.tsx        基准集创建
  src/features/evaluations/evaluations.api.ts          评测 REST 客户端
  e2e/agent-version-and-experience.spec.ts             验收测试
openspec/changes/agent-version-and-experience/tasks.md  任务边界勾选更新
```

archived-with: 2026-07-05-agent-version-and-experience
---

## Task 1: 建立 agentversion 领域模型与不可变版本

- [x] **Completion gate: Task 1 agentversion domain + repository**

**Files:**
- Create: `backend/internal/agentversion/domain/agent_version.go`
- Create: `backend/internal/agentversion/domain/content_hash.go`
- Create: `backend/internal/agentversion/domain/lifecycle.go`
- Create: `backend/internal/agentversion/domain/errors.go`
- Create: `backend/internal/agentversion/application/commands.go`
- Create: `backend/internal/agentversion/application/queries.go`
- Create: `backend/internal/agentversion/application/policy.go`
- Create: `backend/internal/agentversion/application/contracts.go`
- Create: `backend/internal/agentversion/postgres/repository.go`
- Test: `backend/internal/agentversion/domain/agent_version_test.go`
- Test: `backend/internal/agentversion/domain/content_hash_test.go`
- Test: `backend/internal/agentversion/domain/lifecycle_test.go`
- Test: `backend/internal/agentversion/postgres/repository_test.go`

**Interfaces:**
- Produces: `domain.NewAgentVersion(...) (*AgentVersion, error)` — 创建 Draft，自动计算 `content_hash`、`version_number`、设置 `parent_version_id`
- Produces: `agentVersion.StartEvaluation()` — `draft → evaluating`
- Produces: `agentVersion.MarkEligible()` — `evaluating → eligible`
- Produces: `agentVersion.MarkRejected(reason string)` — `evaluating → rejected`
- Produces: `agentVersion.Promote()` — `eligible → active`
- Produces: `agentVersion.Retire()` — `active → retired`
- Produces: `domain.CanRollbackTo(status VersionStatus) bool`
- Produces: `domain.ComputeContentHash(runtime, model, caps, promptRef, skillRefs, memoryRef, toolRefs) string`
- Produces: `commands.CreateDraft(ctx, cmd) (*CreateDraftResponse, error)`
- Produces: `commands.Promote(ctx, cmd) error`
- Produces: `commands.Rollback(ctx, cmd) error`
- Produces: `queries.ListVersions(ctx, tenantID, agentID) ([]VersionSummary, error)`
- Produces: `queries.GetVersion(ctx, tenantID, agentID, versionID) (*VersionDetail, error)`
- Produces: `queries.GetVersionDiff(ctx, tenantID, agentID, versionID, baseVersionID) (*VersionDiff, error)`

- [x] **Step 1: 写领域模型失败测试**

```go
func TestAgentVersionStatusMachine(t *testing.T) {
    v := domain.NewTestAgentVersion(t, domain.StatusDraft)
    require.NoError(t, v.StartEvaluation())
    require.Equal(t, domain.StatusEvaluating, v.Status())
    require.ErrorIs(t, v.Promote(), domain.ErrStateConflict) // draft 不能直接 promote
    require.NoError(t, v.MarkEligible())
    require.NoError(t, v.Promote())
    require.Equal(t, domain.StatusActive, v.Status())
}

func TestAgentVersionContentImmutable(t *testing.T) {
    v := domain.NewTestAgentVersion(t, domain.StatusDraft)
    err := v.UpdatePromptRef("sha256:new")
    require.ErrorIs(t, err, domain.ErrImmutableResource)
}

func TestSameFingerprintRejected(t *testing.T) {
    // 与当前 active 版本指纹相同则返回无变化错误
    err := domain.NewDraftFromCurrent(currentActive, sameConfig)
    require.ErrorIs(t, err, domain.ErrNoChange)
}

func TestRollbackTargetStates(t *testing.T) {
    require.True(t, domain.CanRollbackTo(domain.StatusActive))
    require.True(t, domain.CanRollbackTo(domain.StatusEligible))
    require.True(t, domain.CanRollbackTo(domain.StatusRetired))
    require.False(t, domain.CanRollbackTo(domain.StatusDraft))
}
```

- [x] **Step 2: 运行领域测试并确认失败**

Run: `cd backend && go test ./internal/agentversion/domain -count=1`

Expected: 全部失败（未实现）。

- [x] **Step 3: 实现 AgentVersion 领域模型**

Implement in `backend/internal/agentversion/domain/agent_version.go`:
- 结构体包含：ID、TenantID、AgentID、VersionNumber、ParentVersionID、Status、Runtime、Model、Capabilities、ConfigFingerprint、ContentHash、EnvironmentDigest、PromptRef、SkillRefs、MemoryRef、ToolRefs、CreatedBy、CreatedAt、PromotedAt、RetiredAt。
- 状态迁移方法内部校验合法性，非法迁移返回 `ErrStateConflict`。
- 所有 setter 仅允许在 `draft` 状态且未持久化前调用；持久化后返回 `ErrImmutableResource`。

- [x] **Step 4: 实现 content_hash 与 config_fingerprint 计算**

Implement in `backend/internal/agentversion/domain/content_hash.go`:
- 规范化 `runtime`、`model`、`capabilities`（排序后数组）、`prompt_ref`、`skill_refs`（排序）、`memory_ref`、`tool_refs`（排序）。
- 使用 SHA256 计算 `content_hash`。
- 计算 `config_fingerprint` 作为轻量变化检测（可复用 `content_hash` 前 16 位或单独摘要）。

- [x] **Step 5: 实现 lifecycle 规则**

Implement in `backend/internal/agentversion/domain/lifecycle.go`:
- Promote/Rollback 的纯领域规则（不访问 DB）。
- `CanRollbackTo(status)` 判定。
- 旧 Active 版本 Retire 的领域规则。

- [x] **Step 6: 运行领域测试并确认通过**

Run: `cd backend && go test ./internal/agentversion/domain -count=1`

Expected: PASS。

- [x] **Step 7: 实现 Repository 与 migration**

Implement in `backend/internal/agentversion/postgres/repository.go`:
- `Create(ctx, tx, version) error`
- `UpdateStatus(ctx, tx, version) error` — 只更新 status/timestamp，不更新内容字段
- `GetByID(ctx, tenantID, agentID, versionID) (*AgentVersion, error)`
- `ListByAgent(ctx, tenantID, agentID) ([]AgentVersion, error)`
- `GetLatestByAgent(ctx, tenantID, agentID) (*AgentVersion, error)`
- `LockAgent(ctx, tx, tenantID, agentID) error` — `SELECT FOR UPDATE` 锁定 `agents` 行

Create migration `backend/migrations/000003_agent_version_and_experience.up.sql`:
- 扩展 `agent_versions` 表新增 `parent_version_id`、`status`、`content_hash`、`environment_digest`、`prompt_ref`、`skill_refs`、`memory_ref`、`tool_refs`、`created_by`、`promoted_at`、`retired_at`。
- 添加 CHECK 约束：`status IN ('draft','evaluating','eligible','active','retired','rejected')`。
- 添加自引用外键：`(tenant_id, parent_version_id) -> agent_versions(tenant_id, id)`。
- 将现有初始版本状态设为 `active`，`parent_version_id = NULL`，`version_number = 1`（若表中已有数据）。

- [x] **Step 8: 写 Repository 集成测试**

Test in `backend/internal/agentversion/postgres/repository_test.go`:
- 创建 Draft 后可按 ID 查询；
- UpdateStatus 成功且不会误改内容字段；
- 按 agent 列表返回正确 version_number 顺序；
- 同一 agent 并发创建 Draft 时 version_number 不重复（事务 + 唯一索引保证）。

Run: `cd backend && go test ./internal/agentversion/postgres -count=1`

- [x] **Step 9: 实现 Application 命令与查询**

Implement in `backend/internal/agentversion/application/commands.go`:
- `CreateDraft(ctx, cmd)`: 获取当前 active 版本，计算 fingerprint，相同则 `ErrNoChange`；生成 `version_number = latest + 1`；创建 Draft 并持久化。
- `Promote(ctx, cmd)`: 在事务内锁定 agent，获取目标版本（`eligible`），获取最新 passed EvaluationRun，旧 active 置 `retired`，目标置 `active`，更新 `agents.current_version_id`。
- `Rollback(ctx, cmd)`: 在事务内锁定 agent，校验目标版本状态（active/eligible/retired）且属于该 agent，更新 `agents.current_version_id`。
- `StartEvaluation(ctx, cmd)`: 将 Draft 状态改为 `evaluating`。

Implement in `backend/internal/agentversion/application/queries.go`:
- `ListVersions`, `GetVersion`, `GetVersionDiff`（对比父版本或当前 active 版本的内容引用差异）。

Implement in `backend/internal/agentversion/application/policy.go`:
- `RequireOwnerOrAdmin(ctx, tenantID, agentID, actorID) error`。

- [x] **Step 10: 写 Application 服务集成测试**

Test in `backend/internal/agentversion/application/commands_test.go`:
- Draft → Evaluation → Eligible → Active 完整链路（配合 mock EvaluationRun）。
- 相同 fingerprint 创建 Draft 返回错误。
- 并发 Promote 只有一个成功。
- Rollback 后旧任务版本绑定不变。

Run: `cd backend && go test ./internal/agentversion/application -count=1`

- [x] **Step 11: 更新 tasks.md 勾选 Task 1**

File: `openspec/changes/agent-version-and-experience/tasks.md`

archived-with: 2026-07-05-agent-version-and-experience
---

## Task 2: 建立 evaluation 模块与硬门槛评测

- [x] **Completion gate: Task 2 evaluation domain + repository**

**Files:**
- Create: `backend/internal/evaluation/domain/benchmark_set.go`
- Create: `backend/internal/evaluation/domain/evaluation_run.go`
- Create: `backend/internal/evaluation/domain/scoring.go`
- Create: `backend/internal/evaluation/domain/errors.go`
- Create: `backend/internal/evaluation/application/commands.go`
- Create: `backend/internal/evaluation/application/queries.go`
- Create: `backend/internal/evaluation/application/policy.go`
- Create: `backend/internal/evaluation/application/contracts.go`
- Create: `backend/internal/evaluation/postgres/repository.go`
- Test: `backend/internal/evaluation/domain/*_test.go`
- Test: `backend/internal/evaluation/postgres/repository_test.go`
- Test: `backend/internal/evaluation/application/commands_test.go`

**Interfaces:**
- Produces: `domain.NewBenchmarkSet(...) (*BenchmarkSet, error)`
- Produces: `domain.NewEvaluationRun(...) (*EvaluationRun, error)` — 冻结版本与基准集
- Produces: `evaluationRun.Complete(thresholdResults, summary) error`
- Produces: `evaluationRun.IsPassed() bool`
- Produces: `domain.ApplyScoringRule(taskResults, ruleVersion) (thresholdResults, summary)`
- Produces: `commands.CreateBenchmarkSet(ctx, cmd) (*CreateBenchmarkSetResponse, error)`
- Produces: `commands.StartEvaluationRun(ctx, cmd) (*StartEvaluationRunResponse, error)`
- Produces: `commands.CompleteEvaluationRun(ctx, cmd) error`
- Produces: `queries.GetBenchmarkSet(ctx, tenantID, id)`, `queries.ListBenchmarkSets(ctx, tenantID)`
- Produces: `queries.GetEvaluationRun(ctx, tenantID, id)`, `queries.ListEvaluationRuns(ctx, tenantID, agentVersionID)`

- [x] **Step 1: 写领域模型失败测试**

```go
func TestEvaluationRunFreeze(t *testing.T) {
    run := domain.NewTestEvaluationRun(t)
    require.Equal(t, domain.StatusRunning, run.Status())
    require.Equal(t, "av-1", run.AgentVersionID())
}

func TestEvaluationRunCompletePassed(t *testing.T) {
    run := domain.NewTestEvaluationRun(t)
    require.NoError(t, run.Complete(passedThresholds(), summary()))
    require.Equal(t, domain.StatusPassed, run.Status())
}

func TestEvaluationRunCompleteFailed(t *testing.T) {
    run := domain.NewTestEvaluationRun(t)
    require.NoError(t, run.Complete(failedThresholds(), summary()))
    require.Equal(t, domain.StatusFailed, run.Status())
}

func TestBenchmarkSetVersionNumber(t *testing.T) {
    bs := domain.NewBenchmarkSet("tenant-1", "bs-1", "owner-1", 5)
    require.Equal(t, 5, bs.VersionNumber())
}
```

Run: `cd backend && go test ./internal/evaluation/domain -count=1`

Expected: 全部失败。

- [x] **Step 2: 实现 BenchmarkSet 与 EvaluationRun 领域模型**

Implement in `backend/internal/evaluation/domain/benchmark_set.go`:
- 字段：ID、TenantID、VersionNumber、Name、Description、IsActive、CreatedBy、CreatedAt。
- 同一租户内 version_number 单调递增（由 Repository 通过 `SELECT COALESCE(MAX(...),0)+1` 或序列生成）。
- 任务引用在 `benchmark_set_tasks` 中存储 `(benchmark_set_id, task_ref, ordering)`。

Implement in `backend/internal/evaluation/domain/evaluation_run.go`:
- 字段：ID、TenantID、AgentVersionID、BenchmarkSetID、Status、EnvironmentDigest、ScoringRuleVersion、ThresholdResults(JSONB)、Summary(JSONB)、StartedAt、CompletedAt。
- Status: `running` | `passed` | `failed`。
- `Complete` 方法校验当前为 `running`，根据 threshold_results 判定 passed/failed。

Implement in `backend/internal/evaluation/domain/scoring.go`:
- 初版硬门槛：安全回归 pass、通过率 ≥ 阈值、平均延迟 ≤ 阈值。
- `threshold_results` 保存每个门槛的名称、结果、证据。

- [x] **Step 3: 运行领域测试并确认通过**

Run: `cd backend && go test ./internal/evaluation/domain -count=1`

Expected: PASS。

- [x] **Step 4: 实现 Repository 与 migration 新增表**

Add to `backend/migrations/000003_agent_version_and_experience.up.sql`:
- `benchmark_sets` 表，含 `(tenant_id, version_number)` 唯一索引。
- `benchmark_set_tasks(benchmark_set_id, task_ref, ordering)`。
- `evaluation_runs` 表，含 status CHECK、外键到 `agent_versions(id)` 和 `benchmark_sets(id)`。
- `evaluation_run_results(evaluation_run_id, task_ref, score, passed, details)`。

Implement in `backend/internal/evaluation/postgres/repository.go`:
- `CreateBenchmarkSet`, `GetBenchmarkSet`, `ListBenchmarkSets`, `SetActiveBenchmarkSet`
- `CreateEvaluationRun`, `GetEvaluationRun`, `ListEvaluationRunsByVersion`, `CompleteEvaluationRun`
- `CreateEvaluationRunResult`, `ListEvaluationRunResults`

- [x] **Step 5: 写 Repository 集成测试**

Test:
- 创建 BenchmarkSet 后 version_number 正确且可标记 active；
- 创建 EvaluationRun 后状态为 running；
- Complete 后状态变为 passed/failed；
- 结果明细可查询。

Run: `cd backend && go test ./internal/evaluation/postgres -count=1`

- [x] **Step 6: 实现 Application 命令与查询**

Implement in `backend/internal/evaluation/application/commands.go`:
- `CreateBenchmarkSet(ctx, cmd)`: 生成 version_number，持久化，可选设置 is_active。
- `StartEvaluationRun(ctx, cmd)`: 校验 agent version 与 benchmark set 属于同一 tenant；创建 running 记录；将 version 状态推进为 `evaluating`。
- `CompleteEvaluationRun(ctx, cmd)`: 计算/传入 threshold_results，完成记录；调用 `agentversion` 的 `MarkEligible` 或 `MarkRejected`（通过领域事件或应用服务编排）。

说明：初版 EvaluationRun 执行器使用同步/可注入方式；实际 benchmark 任务执行可先在 `commands.StartEvaluationRun` 中同步完成并调用 `CompleteEvaluationRun`，便于测试，后续可拆分为异步 worker。

- [x] **Step 7: 写 Application 服务集成测试**

Test:
- BenchmarkSet 创建与 active 切换；
- Draft → StartEvaluationRun → Complete(passed) → version eligible；
- Draft → StartEvaluationRun → Complete(failed) → version rejected 或可重跑；
- 运行中的 EvaluationRun 禁止修改 version 内容（在 repository 层通过 version status 校验）。

Run: `cd backend && go test ./internal/evaluation/application -count=1`

- [x] **Step 8: 更新 tasks.md 勾选 Task 2 的 3.1**

archived-with: 2026-07-05-agent-version-and-experience
---

## Task 3: 建立 agentexperience 模块与经验治理

- [x] **Completion gate: Task 3 agentexperience domain + repository + integration**

**Files:**
- Create: `backend/internal/agentexperience/domain/candidate.go`
- Create: `backend/internal/agentexperience/domain/sensitivity.go`
- Create: `backend/internal/agentexperience/domain/errors.go`
- Create: `backend/internal/agentexperience/application/commands.go`
- Create: `backend/internal/agentexperience/application/queries.go`
- Create: `backend/internal/agentexperience/application/policy.go`
- Create: `backend/internal/agentexperience/application/contracts.go`
- Create: `backend/internal/agentexperience/postgres/repository.go`
- Test: `backend/internal/agentexperience/domain/*_test.go`
- Test: `backend/internal/agentexperience/postgres/repository_test.go`
- Test: `backend/internal/agentexperience/application/commands_test.go`

**Interfaces:**
- Produces: `domain.NewExperienceCandidate(...) (*ExperienceCandidate, error)`
- Produces: `candidate.Approve(reviewerID, now)` — pending_review → approved
- Produces: `candidate.Reject(reviewerID, reason, now)` — pending_review → rejected
- Produces: `domain.SensitivityPolicy.Classify(evidence []byte) SensitivityClass`
- Produces: `commands.ExtractCandidate(ctx, cmd) (*ExtractCandidateResponse, error)`
- Produces: `commands.ReviewCandidate(ctx, cmd) error`
- Produces: `queries.ListCandidates(ctx, tenantID, agentID, status)`, `queries.GetCandidate(ctx, tenantID, agentID, candidateID)`

- [x] **Step 1: 写领域模型失败测试**

```go
func TestCandidateAutoRejectForbidden(t *testing.T) {
    c, _ := domain.NewExperienceCandidate(...)
    policy := domain.MockSensitivityPolicy{Class: domain.SensitivityForbidden}
    require.NoError(t, c.ClassifyAndApply(policy))
    require.Equal(t, domain.StatusRejected, c.Status())
    require.NotEmpty(t, c.PolicyReason())
}

func TestCandidateApproveRejectStateMachine(t *testing.T) {
    c := domain.NewTestCandidate(t)
    require.ErrorIs(t, c.Approve("r1", time.Now()), domain.ErrStateConflict) // forbidden 不能 approve
    c2 := domain.NewTestCandidate(t)
    require.NoError(t, c2.Approve("r1", time.Now()))
    require.Equal(t, domain.StatusApproved, c2.Status())
}

func TestCandidateTenantScopeImmutable(t *testing.T) {
    c := domain.NewTestCandidate(t)
    err := c.SetTenantScope("other-tenant")
    require.ErrorIs(t, err, domain.ErrImmutableResource)
}
```

Run: `cd backend && go test ./internal/agentexperience/domain -count=1`

Expected: 全部失败。

- [x] **Step 2: 实现 ExperienceCandidate 领域模型**

Implement in `backend/internal/agentexperience/domain/candidate.go`:
- 字段：ID、TenantID、AgentID、SourceTaskID、SourceSubmissionID、SourceReviewID、EvidenceRef、ContentHash、ApplicableCapabilities、TenantScope、SensitivityClass、Status、PolicyReason、ReviewedBy、ReviewedAt、CreatedAt。
- Status: `pending_review` | `approved` | `rejected`。
- `ClassifyAndApply(policy)` 在创建时调用；forbidden 直接 reject。
- Approve/Reject 仅在 `pending_review` 时允许。

Implement in `backend/internal/agentexperience/domain/sensitivity.go`:
- 接口 `SensitivityPolicy` + 初版基于规则的实现：检查 evidence 中是否包含关键字/正则模式（如密码、密钥、token 模式）以判定 forbidden/restricted/internal/public。
- 预留扩展点，后续可替换为 NLP 管线。

- [x] **Step 3: 运行领域测试并确认通过**

Run: `cd backend && go test ./internal/agentexperience/domain -count=1`

Expected: PASS。

- [x] **Step 4: 实现 Repository 与 migration 新增表**

Add to `backend/migrations/000003_agent_version_and_experience.up.sql`:
- `experience_candidates` 表，含 status CHECK、tenant_id。
- 索引：`(tenant_id, agent_id, status)`、`(tenant_id, agent_id, source_submission_id)`。

Implement in `backend/internal/agentexperience/postgres/repository.go`:
- `Create`, `GetByID`, `ListByAgent`, `UpdateStatus`, `ListApprovedByAgent`。

- [x] **Step 5: 写 Repository 集成测试**

Test:
- 创建后状态为 pending_review（若策略非 forbidden）；
- forbidden 策略创建后状态为 rejected；
- 按 tenant + agent + status 查询隔离；
- 审批后状态更新。

Run: `cd backend && go test ./internal/agentexperience/postgres -count=1`

- [x] **Step 6: 实现 Application 命令与查询**

Implement in `backend/internal/agentexperience/application/commands.go`:
- `ExtractCandidate(ctx, cmd)`: 从 Accepted Submission 读取 execution，生成 evidence 摘要与 content_hash，创建 candidate。
  - 依赖：`git-delivery-and-validation` / `code-review-and-reputation` 提供的 `SubmissionStore` / `ExecutionStore` 接口（初版先定义接口，后续 change 实现具体 repository 时接入）。
- `ReviewCandidate(ctx, cmd)`: 校验 reviewer 权限，调用 Approve/Reject。

Implement in `backend/internal/agentexperience/application/queries.go`:
- `ListCandidates`, `GetCandidate`。

- [x] **Step 7: 实现经验与新版本的绑定**

Modify `backend/internal/agentversion/application/commands.go` 的 `CreateDraft`：
- 命令支持可选 `approved_experience_ids []string`。
- 仅允许状态为 `approved` 且属于同一 agent/tenant 的候选。
- 将候选的 `evidence_ref` 合并进新版本的 `memory_ref` 或 `skill_refs`（初版统一放入 `memory_ref`，多个 approved 经验合并为数组后计算 content_hash）。
- 这些引用随版本切换和回滚自然失效。

- [x] **Step 8: 写 Application 集成测试**

Test:
- 从 Accepted Submission 提取 candidate；
- forbidden 候选自动 reject；
- approved 候选可纳入新 Draft；
- 回滚后旧版本 memory_ref 不包含新经验。

Run: `cd backend && go test ./internal/agentexperience/application -count=1`

- [x] **Step 9: 更新 tasks.md 勾选 Task 3 的 2.1–2.3**

archived-with: 2026-07-05-agent-version-and-experience
---

## Task 4: REST、MCP Transport 与权限策略

- [x] **Completion gate: Task 4 transport wired and manually tested**

**Files:**
- Create: `backend/internal/transport/rest/agent_version_router.go`
- Create: `backend/internal/transport/rest/experience_router.go`
- Create: `backend/internal/transport/rest/evaluation_router.go`
- Create: `backend/internal/transport/mcp/agent_version_tools.go`
- Create: `backend/internal/transport/mcp/experience_tools.go`
- Create: `backend/internal/transport/mcp/evaluation_tools.go`
- Create: `backend/internal/transport/mcp/content_store.go`
- Update: `backend/internal/transport/rest/router.go` (wire routes)
- Update: `backend/internal/transport/mcp/server.go` (wire tools)
- Test: `backend/internal/transport/rest/agent_version_router_test.go`
- Test: `backend/internal/transport/mcp/*_test.go` (optional)

**Interfaces:**
- REST endpoints per design doc §7:
  - `GET /v1/agents/:id/versions`
  - `POST /v1/agents/:id/versions`
  - `GET /v1/agents/:id/versions/:version_id`
  - `POST /v1/agents/:id/versions/:version_id/diff`
  - `POST /v1/agents/:id/versions/:version_id/evaluations`
  - `POST /v1/agents/:id/versions/:version_id/promote`
  - `POST /v1/agents/:id/versions/:version_id/rollback`
  - `GET /v1/agents/:id/experiences`
  - `POST /v1/agents/:id/experiences`
  - `POST /v1/agents/:id/experiences/:experience_id/approve`
  - `POST /v1/agents/:id/experiences/:experience_id/reject`
  - `GET /v1/benchmarks`, `POST /v1/benchmarks`, `GET /v1/benchmarks/:id`
  - `GET /v1/evaluations`, `GET /v1/evaluations/:id`
- MCP tools per design doc §8:
  - `agent_version_list`, `agent_version_create`, `agent_version_promote`, `agent_version_rollback`
  - `experience_candidate_list`, `experience_candidate_review`
  - `evaluation_run_start`, `evaluation_run_get`

- [x] **Step 1: 实现 REST routers**

Each router:
- 从请求上下文提取 `Principal`（人类 OIDC session 或 Agent JWT）。
- 调用对应 application policy 校验 owner/admin。
- 转换 DTO，调用 command/query。
- 统一错误映射：domain/app 错误 → HTTP status + code。

- [x] **Step 2: 实现 MCP tools**

Each tool:
- 从 MCP 上下文提取 `Principal`。
- 校验权限。
- 返回结构化 JSON，避免一次性凭证暴露。

- [x] **Step 3: 预留 ContentStore 接口**

Implement in `backend/internal/transport/mcp/content_store.go`:
- 接口 `ContentStore { Get(ref string) ([]byte, error); Put(content []byte) (string, error) }`
- 初版 `MemoryContentStore` 实现，用于测试和本地开发。

- [x] **Step 4: 写 Router 集成测试**

Test:
- 创建 Draft 成功并返回 version_id；
- 非 owner 访问返回 403/404 一致错误；
- 非法状态迁移返回 409；
- 启动 EvaluationRun、Promote、Rollback 端到端（使用内存/testcontainer DB）。

Run: `cd backend && go test ./internal/transport/rest -run 'AgentVersion|Experience|Evaluation' -count=1`

- [x] **Step 5: 更新 OpenAPI 文档**

Update `backend/internal/transport/rest/openapi.yaml`:
- 新增版本、经验、评测相关 paths 和 schemas。

archived-with: 2026-07-05-agent-version-and-experience
---

## Task 5: React 管理界面

- [x] **Completion gate: Task 5 frontend pages render and E2E pass**

**Files:**
- Create: `frontend/src/features/versions/VersionTree.tsx`
- Create: `frontend/src/features/versions/VersionDetail.tsx`
- Create: `frontend/src/features/versions/VersionActions.tsx`
- Create: `frontend/src/features/versions/versions.api.ts`
- Create: `frontend/src/features/versions/versions.test.tsx`
- Create: `frontend/src/features/experiences/ExperienceList.tsx`
- Create: `frontend/src/features/experiences/ExperienceReview.tsx`
- Create: `frontend/src/features/experiences/experiences.api.ts`
- Create: `frontend/src/features/experiences/experiences.test.tsx`
- Create: `frontend/src/features/evaluations/EvaluationList.tsx`
- Create: `frontend/src/features/evaluations/EvaluationDetail.tsx`
- Create: `frontend/src/features/evaluations/BenchmarkSetForm.tsx`
- Create: `frontend/src/features/evaluations/evaluations.api.ts`
- Create: `frontend/src/features/evaluations/evaluations.test.tsx`
- Update: `frontend/src/App.tsx` 或路由配置，注册新页面
- Create: `frontend/e2e/agent-version-and-experience.spec.ts`

- [x] **Step 1: 实现 Versions 页面**

- `VersionTree`: 使用 `/v1/agents/:id/versions` 构建谱系树（按 `parent_version_id` 递归）。
- `VersionDetail`: 展示内容引用、状态、评测结果；调用 `/diff` 展示与父版本差异。
- `VersionActions`: 根据状态展示「启动评测」「晋级」「回滚」按钮。

- [x] **Step 2: 实现 Experiences 页面**

- `ExperienceList`: 列出 pending_review / approved / rejected 候选，显示来源任务和敏感级别。
- `ExperienceReview`: 审批或拒绝候选，填写原因。

- [x] **Step 3: 实现 Evaluations 页面**

- `BenchmarkSetForm`: 创建基准集，上传/选择任务列表，标记 is_active。
- `EvaluationList` / `EvaluationDetail`: 展示评测运行状态与门槛结果。

- [x] **Step 4: 写组件测试**

Run: `cd frontend && npm run test -- versions.test.tsx experiences.test.tsx evaluations.test.tsx`

Expected: PASS。

- [x] **Step 5: 写 Playwright E2E 验收测试**

In `frontend/e2e/agent-version-and-experience.spec.ts`:
- Draft → EvaluationRun passed → Eligible → Active 完整链路；
- 回滚后新任务使用旧版本；
- 经验候选 forbidden 自动拒绝；
- approved 经验纳入新版本后随回滚失效；
- 跨 tenant 访问被阻止（admin 视角验证）。

Run: `cd frontend && npx playwright test e2e/agent-version-and-experience.spec.ts`

Expected: PASS。

- [x] **Step 6: 更新 tasks.md 勾选 Task 3 的 3.2–3.3**

archived-with: 2026-07-05-agent-version-and-experience
---

## Task 6: 端到端验证与任务收尾

- [x] **Step 1: 运行全量后端测试**

Run:
```bash
cd backend
go test ./... -count=1
```

Expected: PASS（或仅与未实现依赖相关的已知 skip）。

- [x] **Step 2: 运行全量前端测试**

Run:
```bash
cd frontend
npm run test
```

Expected: PASS。

- [x] **Step 3: 运行 Playwright 验收测试**

Run:
```bash
cd frontend
npx playwright test
```

Expected: PASS（包括新 E2E）。

- [x] **Step 4: 数据库迁移可回滚验证**

Run:
```bash
cd backend
# 假设使用 migrate 工具
migrate -path migrations -database "$DATABASE_URL" down 1
migrate -path migrations -database "$DATABASE_URL" up 1
```

Expected: 成功 down/up，应用状态一致。

- [x] **Step 5: 更新 CHANGELOG / release notes（如项目有）**

- 记录新增 capability：Agent Version 不可变快照、ExperienceCandidate 治理、EvaluationRun 硬门槛。

- [x] **Step 6: 最终检查 tasks.md 全部勾选**

File: `openspec/changes/agent-version-and-experience/tasks.md`

All items should be `- [x]`.

archived-with: 2026-07-05-agent-version-and-experience
---

## Implementation Order

1. **Task 1** 与 **Task 2** 可并行启动：agentversion 的 Draft 创建依赖 evaluation 的启动评测接口较少，主要接口在 Promote 阶段需要 EvaluationRun passed 证据；可先实现 domain/repository，再联调应用服务。
2. **Task 3** 可在 Task 1 的 Draft 命令完成后开始，因为经验绑定发生在 CreateDraft 时。
3. **Task 4** 在 Task 1–3 的 application 服务完成后统一接线。
4. **Task 5** 在 Task 4 REST 接口稳定后开始。
5. **Task 6** 在所有测试通过、迁移可回滚后执行。

## Migration Steps Summary

Single migration file: `backend/migrations/000003_agent_version_and_experience.up.sql`

1. 扩展 `agent_versions` 表：新增 parent_version_id、status、content_hash、environment_digest、prompt_ref、skill_refs、memory_ref、tool_refs、created_by、promoted_at、retired_at。
2. 添加 CHECK 约束与自引用外键。
3. 存量数据迁移：将现有版本标记为 `active`，version_number 设为 1，parent_version_id NULL。
4. 新建表：
   - `experience_candidates`
   - `benchmark_sets`
   - `benchmark_set_tasks`
   - `evaluation_runs`
   - `evaluation_run_results`
5. 所有新建表添加 tenant_id 索引与外键。

Rollback: `backend/migrations/000003_agent_version_and_experience.down.sql` 删除上述新增表与列（保留原始 `agent_versions` 表核心字段）。

## Key Verification Checklist

- [x] `agentversion` 状态机单测全部通过。
- [x] `content_hash` 对相同配置返回相同值，不同配置返回不同值。
- [x] 已创建版本的内容字段无法通过 UPDATE 修改。
- [x] Promote 事务原子切换 `current_version_id`；并发 Promote 仅一个成功。
- [x] Rollback 不删除历史，旧任务仍绑定原版本。
- [x] EvaluationRun 冻结 version/benchmark/scoring_rule/environment。
- [x] 硬门槛失败时 EvaluationRun 状态为 failed 并保留证据。
- [x] forbidden 经验候选自动 reject；approved 候选可纳入 Draft。
- [x] 版本回滚后旧版本 memory_ref/skill_refs 不包含新经验。
- [x] 所有查询均受 tenant_id 隔离。
- [x] REST 与 MCP 接口均实现并手动/E2E 验证。

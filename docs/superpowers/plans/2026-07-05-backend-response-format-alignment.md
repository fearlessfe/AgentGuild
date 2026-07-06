---
change: backend-response-format-alignment
design-doc: docs/superpowers/specs/2026-07-05-backend-response-format-alignment-design.md
base-ref: 01bda2b81d2a26fa0b4dd1fe7902666ee6843241
archived-with: 2026-07-06-backend-response-format-alignment
---

# Backend Response Format Alignment — 实现计划

## 1. 目标与范围

依据 [Design Doc](../specs/2026-07-05-backend-response-format-alignment-design.md)，将 `identity`、`agentversion`、`agentexperience`、`evaluation` 四个应用层模块的 REST 响应字段统一为 `snake_case`，并补齐 `VersionDiff` 与 `EvaluationRun` 详情两种特殊响应结构，使关闭 Demo 模式后的前端控制台能够正确读取真实数据。

**本次变更不修改：**
- 领域模型行为与数据库表结构。
- 现有的路由路径、HTTP 状态码、权限检查。
- 任务/执行/提交/评审等其他模块的响应格式。

## 2. 决策依据

- 前端 TypeScript 类型统一使用 `snake_case`（见 `frontend/src/features/{agents,versions,experiences,evaluations}/*.types.ts`）。
- Go 结构体缺 `json` tag 时按 PascalCase 序列化，导致前端读到空字段或类型不匹配。
- `VersionDiff` 后端内部使用 `map[string]RefChange` 便于差异计算，但前端需要数组视图，因此仅在 REST 层新增 DTO 做转换。
- `EvaluationRun` 详情需要在现有 summary 基础上额外返回 `threshold_results` 与 `summary`，由应用层新增聚合 DTO 提供。

## 3. 任务拆解

# Task 1: 调查与清单

确认以下文件中的视图结构体与前端类型的字段映射关系：

| 后端文件 | 视图结构体 | 前端类型文件 |
|---|---|---|
| `backend/internal/identity/application/contracts.go` | `Envelope`、`Meta`、`AgentView`、`AgentVersionView`、`AccessTokenView`、`RegisterAgentResponse`、`ActivationStatusView`、`AgentPage` | `frontend/src/features/agents/agents.types.ts` |
| `backend/internal/agentversion/application/contracts.go` | `VersionSummary`、`VersionDetail` | `frontend/src/features/versions/versions.types.ts` |
| `backend/internal/agentexperience/application/contracts.go` | `CandidateSummary` | `frontend/src/features/experiences/experiences.types.ts` |
| `backend/internal/evaluation/application/contracts.go` | `BenchmarkSetSummary`、`EvaluationRunSummary` | `frontend/src/features/evaluations/evaluations.types.ts` |

**验证命令：**

```bash
cd /Users/pengzhen/work/AgentGuild/backend
go build ./...
```

# Task 2: 应用层 JSON Tag 补充

按 `json:"snake_case,omitempty"` 规范添加 tag。可选字段（指针、切片、map）加 `omitempty`，基础值类型不加。

## `backend/internal/identity/application/contracts.go`

- `Envelope[T]`：`Data` → `json:"data"`，`Meta` → `json:"meta"`。
- `Meta`：`ServerTime` → `json:"server_time"`，`ResourceVersion` → `json:"resource_version"`，`PollAfterSeconds` → `json:"poll_after_seconds"`。
- `AgentView`：
  - `ID` → `id`
  - `TenantID` → `tenant_id`
  - `Name` → `name`
  - `Description` → `description,omitempty`
  - `Status` → `status`
  - `Team` → `team,omitempty`
  - `OwnerID` → `owner_id`
  - `OwnerEmail` → `owner_email`
  - `Scopes` → `scopes,omitempty`
  - `RepoScope` → `repo_scope,omitempty`
  - `CurrentVersion` → `current_version,omitempty`
  - `LastSeenAt` → `last_seen_at,omitempty`
  - `BudgetCents` → `budget_cents,omitempty`
  - `BudgetCurrency` → `budget_currency,omitempty`
  - `CreatedAt` → `created_at`
  - `UpdatedAt` → `updated_at,omitempty`
- `AgentVersionView`：
  - `ID` → `id`
  - `TenantID` → `tenant_id`
  - `AgentID` → `agent_id`
  - `VersionNumber` → `version_number`
  - `Runtime` → `runtime`
  - `Model` → `model`
  - `Capabilities` → `capabilities,omitempty`
  - `ConfigFingerprint` → `config_fingerprint`
  - `CreatedAt` → `created_at`
- `AccessTokenView`：
  - `Token` → `token`
  - `TokenType` → `token_type`
  - `ExpiresAt` → `expires_at`
  - `AgentID` → `agent_id`
  - `AgentVersionID` → `agent_version_id`
  - `Scopes` → `scopes,omitempty`
  - `RepoScope` → `repo_scope,omitempty`
- `RegisterAgentResponse`：
  - `Agent` → `agent`
  - `ActivationToken` → `activation_token`
  - `ActivationExpiresAt` → `activation_expires_at,omitempty`
- `ActivationStatusView`：
  - `AgentID` → `agent_id`
  - `Status` → `status`
  - `ActivationStatus` → `activation_status`
  - `ActivationExpiresAt` → `activation_expires_at,omitempty`
  - `ActivatedAt` → `activated_at,omitempty`
- `AgentPage`：`Items` → `items`

## `backend/internal/agentversion/application/contracts.go`

- `VersionSummary`：
  - `ID` → `id`
  - `VersionNumber` → `version_number`
  - `Status` → `status`
  - `ParentVersionID` → `parent_version_id,omitempty`
  - `ConfigFingerprint` → `config_fingerprint`
  - `CreatedAt` → `created_at`
  - `PromotedAt` → `promoted_at,omitempty`
  - `RetiredAt` → `retired_at,omitempty`
- `VersionDetail`：
  - `ID` → `id`
  - `TenantID` → `tenant_id`
  - `AgentID` → `agent_id`
  - `VersionNumber` → `version_number`
  - `ParentVersionID` → `parent_version_id,omitempty`
  - `Status` → `status`
  - `Runtime` → `runtime`
  - `Model` → `model`
  - `Capabilities` → `capabilities,omitempty`
  - `ConfigFingerprint` → `config_fingerprint`
  - `ContentHash` → `content_hash`
  - `EnvironmentDigest` → `environment_digest`
  - `PromptRef` → `prompt_ref,omitempty`
  - `SkillRefs` → `skill_refs,omitempty`
  - `MemoryRef` → `memory_ref,omitempty`
  - `ToolRefs` → `tool_refs,omitempty`
  - `CreatedBy` → `created_by`
  - `CreatedAt` → `created_at`
  - `PromotedAt` → `promoted_at,omitempty`
  - `RetiredAt` → `retired_at,omitempty`
- `VersionDiff`、`RefChange` 保持内部表达，不添加 JSON tag；REST 层通过专用 DTO 转换。

## `backend/internal/agentexperience/application/contracts.go`

- `CandidateSummary`：
  - `ID` → `id`
  - `TenantID` → `tenant_id`
  - `AgentID` → `agent_id`
  - `SourceTaskID` → `source_task_id,omitempty`
  - `SourceSubmissionID` → `source_submission_id,omitempty`
  - `SourceReviewID` → `source_review_id,omitempty`
  - `EvidenceRef` → `evidence_ref`
  - `ContentHash` → `content_hash`
  - `ApplicableCapabilities` → `applicable_capabilities,omitempty`
  - `TenantScope` → `tenant_scope`
  - `SensitivityClass` → `sensitivity_class`
  - `Status` → `status`
  - `PolicyReason` → `policy_reason,omitempty`
  - `ReviewedBy` → `reviewed_by,omitempty`
  - `ReviewedAt` → `reviewed_at,omitempty`
  - `CreatedAt` → `created_at`

## `backend/internal/evaluation/application/contracts.go`

- `BenchmarkSetSummary`：
  - `ID` → `id`
  - `TenantID` → `tenant_id`
  - `VersionNumber` → `version_number`
  - `Name` → `name`
  - `Description` → `description,omitempty`
  - `IsActive` → `is_active`
  - `CreatedBy` → `created_by`
  - `CreatedAt` → `created_at`
- `EvaluationRunSummary`：
  - `ID` → `id`
  - `TenantID` → `tenant_id`
  - `AgentVersionID` → `agent_version_id`
  - `BenchmarkSetID` → `benchmark_set_id`
  - `Status` → `status`
  - `EnvironmentDigest` → `environment_digest`
  - `ScoringRuleVersion` → `scoring_rule_version`
  - `StartedAt` → `started_at`
  - `CompletedAt` → `completed_at,omitempty`

#### 3.2.5 领域值对象 JSON Tag（为 `EvaluationRunDetail` 序列化做准备）

- `backend/internal/evaluation/domain/evaluation_run.go`
  - `ThresholdResult.Name` → `json:"name"`
  - `ThresholdResult.Passed` → `json:"passed"`
  - `ThresholdResult.Evidence` → `json:"evidence,omitempty"`
  - `EvaluationSummary.PassRate` → `json:"pass_rate"`
  - `EvaluationSummary.AvgLatencyMs` → `json:"avg_latency_ms"`
  - `EvaluationSummary.CostCents` → `json:"cost_cents"`
  - `EvaluationSummary.SecurityPassed` → `json:"security_passed"`
  - `EvaluationSummary.Extra` → `json:"extra,omitempty"`

**验证命令：**

```bash
cd /Users/pengzhen/work/AgentGuild/backend
go test -race ./internal/identity/... ./internal/agentversion/... ./internal/agentexperience/... ./internal/evaluation/...
```

# Task 3: 结构对齐

## `VersionDiff` REST DTO

在 `backend/internal/transport/rest/agent_version_router.go` 同包新增：

```go
type versionDiffView struct {
    BaseVersionID       string          `json:"base_version_id"`
    TargetVersionID     string          `json:"target_version_id"`
    AddedCapabilities   []string        `json:"added_capabilities,omitempty"`
    RemovedCapabilities []string        `json:"removed_capabilities,omitempty"`
    ChangedRefs         []refChangeView `json:"changed_refs,omitempty"`
}

type refChangeView struct {
    Field string  `json:"field"`
    From  *string `json:"from,omitempty"`
    To    *string `json:"to,omitempty"`
}
```

新增转换函数（名称可调整）：

```go
func toVersionDiffView(targetVersionID string, diff *agentversionapp.VersionDiff) *versionDiffView
```

转换规则：
- `diff.BaseVersionID` → `BaseVersionID`。
- URL 参数 `version_id` → `TargetVersionID`。
- `diff.Added` 中 key 以 `capability:` 为前缀的条目提取 capability 名称，加入 `AddedCapabilities`；以 `skill:` / `tool:` 为前缀的条目也作为 capability 名称加入（保持与现有 diff 语义一致）。
- `diff.Removed` 同理提取为 `RemovedCapabilities`。
- `diff.Changed` 中每个 key 转为一个 `refChangeView`，`From`/`To` 取 `RefChange.From`/`RefChange.To`；空字符串使用 `omitempty` 省略。

修改 `diffAgentVersion`：

```go
result, err := s.versions.GetVersionDiff(...)
// ...
view := toVersionDiffView(chi.URLParam(r, "version_id"), result)
writeJSON(w, http.StatusOK, view)
```

## `EvaluationRunDetail` 应用层 DTO

在 `backend/internal/evaluation/application/contracts.go` 新增：

```go
type EvaluationRunDetail struct {
    ID                 string                 `json:"id"`
    TenantID           string                 `json:"tenant_id"`
    AgentVersionID     string                 `json:"agent_version_id"`
    BenchmarkSetID     string                 `json:"benchmark_set_id"`
    Status             string                 `json:"status"`
    EnvironmentDigest  string                 `json:"environment_digest"`
    ScoringRuleVersion string                 `json:"scoring_rule_version"`
    ThresholdResults   []domain.ThresholdResult `json:"threshold_results,omitempty"`
    Summary            domain.EvaluationSummary `json:"summary"`
    StartedAt          time.Time              `json:"started_at"`
    CompletedAt        *time.Time             `json:"completed_at,omitempty"`
}
```

在 `backend/internal/evaluation/application/queries.go` 新增方法：

```go
func (s *EvaluationService) GetEvaluationRunDetail(
    ctx context.Context,
    principal identityapp.Principal,
    tenantID, id string,
) (*EvaluationRunDetail, error)
```

实现步骤：
1. 调用 `s.GetEvaluationRun(ctx, principal, tenantID, id)` 复用权限与存在性校验。
2. 从返回的 `*domain.EvaluationRun` 读取 `ThresholdResults()` 与 `Summary()`。
3. 填充 `EvaluationRunDetail` 并返回。

新增辅助函数：

```go
func toEvaluationRunDetail(run *domain.EvaluationRun) EvaluationRunDetail
```

修改 `backend/internal/transport/rest/evaluation_router.go` 中的 `getEvaluation`：

```go
result, err := s.evaluations.GetEvaluationRunDetail(r.Context(), principal, principal.TenantID, chi.URLParam(r, "id"))
// ...
writeJSON(w, http.StatusOK, result)
```

**验证命令：**

```bash
cd /Users/pengzhen/work/AgentGuild/backend
go test -race ./internal/transport/rest/...
```

# Task 4: 测试补充

1. **更新现有测试断言**
   - 在 `backend/internal/transport/rest/` 下搜索包含 PascalCase 字段名的 `JSONEq` 或 `Contains` 断言，替换为 snake_case。
   - 重点检查 `identity_router_test.go`、`agent_version_router_test.go`（如存在）、`evaluation_router_test.go`（如存在）。

2. **新增 diff 转换测试**
   - 在 `backend/internal/transport/rest/agent_version_router_test.go`（不存在则创建）中：
     - 构造 fake `VersionDiff`（含 Added/Removed/Changed）。
     - 调用 `toVersionDiffView`。
     - 断言 JSON 序列化后包含 `base_version_id`、`target_version_id`、`added_capabilities`、`removed_capabilities`、`changed_refs`。

3. **新增 evaluation detail 聚合测试**
   - 在 `backend/internal/transport/rest/evaluation_router_test.go`（不存在则创建）中：
     - mock `EvaluationService.GetEvaluationRunDetail` 返回包含 `ThresholdResults` 与 `Summary` 的 `EvaluationRunDetail`。
     - 断言 `/v1/evaluations/{id}` 响应包含 `threshold_results` 与 `summary`。

**验证命令：**

```bash
cd /Users/pengzhen/work/AgentGuild/backend
go test -race ./internal/transport/rest/...
```

# Task 5: 前端兼容性验证

- 确认 `frontend/src/features/{agents,versions,experiences,evaluations}/*.types.ts` 中的字段名与后端输出一致。
- 运行前端单元测试。

**验证命令：**

```bash
cd /Users/pengzhen/work/AgentGuild/frontend
npm test -- --run
npm run build
```

# Task 6: 文档与收尾

1. 更新 `openspec/changes/backend-response-format-alignment/tasks.md`：
   - 将 3.1–3.5 中已完成的任务标记为 `[x]`。
2. 更新本计划文件中的进度记录（如需要）。
3. 运行全量后端测试：

```bash
cd /Users/pengzhen/work/AgentGuild/backend
go test -race ./...
```

4. 运行项目级构建验证：

```bash
cd /Users/pengzhen/work/AgentGuild
make build
make test
```

## 4. 风险与回退

| 风险 | 缓解措施 |
|---|---|
| 修改 `Envelope`/`Meta` tag 后影响所有 identity 接口的顶层键名 | 前端 `Envelope<T>` 已使用 `data`/`meta`，符合预期；如有其他旧客户端依赖 `Data`/`Meta` 将面临 breaking change，需按设计 Doc 执行。 |
| `VersionDiff` 内部使用 map，REST DTO 遗漏某种 key 前缀 | 转换函数只处理 `capability:` / `skill:` / `tool:`，与 `diffVersions` 当前实现一致；如后续新增差异维度，需同步更新 DTO。 |
| `EvaluationRunDetail` 直接暴露 domain 的 `ThresholdResult`/`EvaluationSummary` | 已为这两个值对象添加 JSON tag；若担心 domain 被其他序列化场景误用，可在 REST 层再包一层 view struct。 |
| 测试数据库依赖 | 使用 `make db-up` 或让 `internal/testdb` 自动拉取临时 PostgreSQL 18.4 容器。 |

## 5. 验收标准

- `go test -race ./...` 通过。
- `npm test -- --run` 通过。
- `make build` 通过。
- 关闭 Demo 模式后，前端 Agent 列表、版本列表、版本 diff、经验候选、评测详情页面能正确显示后端字段，无 `undefined` 或空字段异常。

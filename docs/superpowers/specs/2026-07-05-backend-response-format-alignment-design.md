---
comet_change: backend-response-format-alignment
role: technical-design
canonical_spec: openspec
archived-with: 2026-07-06-backend-response-format-alignment
status: final
---

# Backend Response Format Alignment — Design Doc

## 背景

前端 TypeScript 类型统一使用 snake_case 字段名，但后端 `agent-identity`、`agent-versioning`、`agent-experience`、`evaluation` 四个应用层模块的视图结构体缺少 `json` tag，Go 默认按 PascalCase 序列化。关闭 Demo 模式后，前端会读到空字段或类型不匹配。

## 目标

- 四个模块所有 REST 响应字段统一为 snake_case，与前端类型一致。
- `VersionDiff` 接口返回前端友好的能力差异结构。
- `EvaluationRun` 详情接口返回聚合结果（threshold_results + summary）。
- 不修改领域模型、不新增数据库表。

## 非目标

- 不改变 Agent/版本/经验/评测的领域行为。
- 不引入新的 JSON 序列化库。
- 不为 Agent 端（非控制台）新增接口。

## 设计方案

### 1. JSON tag 统一

在以下文件的结构体字段上添加 `json:"snake_case,omitempty"` tag：

- `backend/internal/identity/application/contracts.go`
  - `AgentView`、`AgentVersionView`、`AccessTokenView`、`RegisterAgentResponse`、`ActivationStatusView`、`AgentPage`、`Meta`
- `backend/internal/agentversion/application/contracts.go`
  - `VersionSummary`、`VersionDetail`、`VersionDiff`、`RefChange`、`CreateDraftResponse`、`VersionPage`
- `backend/internal/agentexperience/application/contracts.go`
  - `CandidateSummary`、`ExtractCandidateResponse`、`ExperienceCandidatePage`
- `backend/internal/evaluation/application/contracts.go`
  - `BenchmarkSetSummary`、`EvaluationRunSummary`、`EvaluationRunDetail`、`CreateBenchmarkSetResponse`、`StartEvaluationRunResponse`、`BenchmarkSetPage`、`EvaluationRunPage`

注意：
- `RegisterAgentResponse.Data.Agent` 已包含 `AgentView`，但内部还嵌套 `*domain.AgentVersion` 的 `CurrentVersion`。REST 层 `/v1/agents` 返回的是 `Envelope[AgentPage]`，序列化 `AgentView` 即可；嵌套 domain 结构体不会被前端直接使用，但仍需确认不破坏测试。
- 对 `time.Time` 字段统一使用 `json:"created_at"` 等 tag；Go JSON encoder 会按 RFC3339 序列化。

### 2. VersionDiff REST DTO

应用层 `VersionDiff` 保持内部表达：

```go
type VersionDiff struct {
    BaseVersionID string
    Added         map[string]RefChange
    Removed       map[string]RefChange
    Changed       map[string]RefChange
}
```

REST 层新增 DTO（可放在 `agent_version_router.go` 同包或应用层）：

```go
type versionDiffView struct {
    BaseVersionID       string           `json:"base_version_id"`
    TargetVersionID     string           `json:"target_version_id"`
    AddedCapabilities   []string         `json:"added_capabilities"`
    RemovedCapabilities []string         `json:"removed_capabilities"`
    ChangedRefs         []refChangeView  `json:"changed_refs"`
}

type refChangeView struct {
    Field string  `json:"field"`
    From  *string `json:"from,omitempty"`
    To    *string `json:"to,omitempty"`
}
```

转换逻辑：
- `Added` map 的 key 列表 → `AddedCapabilities`。
- `Removed` map 的 key 列表 → `RemovedCapabilities`。
- `Changed` map 每个 entry → `ChangedRefs` 元素，`field` 为 key，`from/to` 取自 `RefChange`。

### 3. EvaluationRun 详情 DTO

应用层新增查询 DTO 和查询方法：

```go
type EvaluationRunDetail struct {
    ID                 string
    TenantID           string
    AgentVersionID     string
    BenchmarkSetID     string
    Status             string
    EnvironmentDigest  string
    ScoringRuleVersion string
    ThresholdResults   []domain.ThresholdResult
    Summary            domain.EvaluationSummary
    StartedAt          time.Time
    CompletedAt        *time.Time
}
```

REST `/v1/evaluations/{id}` 当前返回 `GetEvaluationRunSummary`，改为返回 `EvaluationRunDetail`。应用层 `EvaluationService` 新增：

```go
func (s *EvaluationService) GetEvaluationRunDetail(ctx, tenantID, id string) (*EvaluationRunDetail, error)
```

该方法内部复用 `runs.GetByID`、`ListResults`，并聚合 summary。

### 4. REST 层调整

- `agent_version_router.go` 的 `listAgentVersions` 返回 `Envelope[VersionPage]`，`getAgentVersion` 返回 `Envelope[VersionDetail]`，`diffAgentVersion` 返回 `Envelope[VersionDiffView]`。
- `evaluation_router.go` 的 `listBenchmarks` 返回 `Envelope[BenchmarkSetPage]`，`getBenchmark` 返回 `Envelope[BenchmarkSetSummary]`，`listEvaluations` 调用 `ListEvaluationRunDetails` 返回 `Envelope[EvaluationRunPage]`（列表项使用完整详情，含 `summary` 与 `threshold_results`），`getEvaluation` 返回 `Envelope[EvaluationRunDetail]`。
- `experience_router.go` 的 `listAgentExperiences` 返回 `Envelope[ExperienceCandidatePage]`。
- 创建/提升/回滚/启动评测/经验提取/审核等命令端点同时返回 `{data, meta}` 信封结构。
- 受影响的查询路由统一使用 `{data, meta}` 信封，与任务生命周期、Identity 模块保持一致。

### 5. 测试策略

- 更新 REST 路由测试中断言 JSON 字段名的地方。
- 新增 `agent_version_router_test.go` 中 diff 转换断言。
- 新增 `evaluation_router_test.go` 中 detail 聚合断言。
- 运行 `go test -race ./internal/...`。
- 前端运行 `npm test -- --run` 确保类型和 demo 数据兼容。

## 风险与缓解

| 风险 | 缓解 |
|---|---|
| 应用层结构体被内部事件序列化使用 | 这些结构体目前只在 REST 响应和同一模块内部传递；改完后全量测试确认 |
| `RegisterAgentResponse` 嵌套 `*domain.AgentVersion` | REST 返回使用 `AgentView`；如需完整字段，后续再补 REST DTO |
| `EvaluationRunDetail` 需要新增查询方法 | 仅在应用层和 REST 层新增，不修改 repository 接口语义 |

## Open Questions

- `identity` 模块的 `AccessTokenView` 是否会被 Agent 客户端严格解析字段名？加 tag 后 snake_case 对 Agent 客户端也是更一致的 REST 契约。
- `AgentVersionView` 字段（如 `ConfigFingerprint`）是否需要暴露给人类控制台？当前前端 `VersionView` 期望这些字段，保持一致。

## Spec Patch

已回写以下 delta spec：

- `openspec/specs/agent-identity/spec.md`：增加 REST 视图字段 snake_case 要求。
- `openspec/specs/agent-versioning/spec.md`：增加版本 diff 接口响应结构要求。
- `openspec/specs/agent-experience/spec.md`：增加经验候选视图字段 snake_case 要求。
- `openspec/specs/evaluation/spec.md`：增加评测运行详情聚合响应和字段命名要求。

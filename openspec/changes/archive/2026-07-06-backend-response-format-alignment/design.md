## Context

后端 Go 结构体默认按字段名 PascalCase 序列化，而前端所有类型约定为 snake_case。`task-lifecycle` 和 `code-review` 模块已在结构体上加了 `json` tag，但 `agent-identity`、`agent-versioning`、`agent-experience`、`evaluation` 四个模块没有。真实后端启动后，前端 Agent、版本、经验、评测页面会拿到 `ID` 而非 `id`，导致解构失败。

## Goals / Non-Goals

**Goals:**
- 四个模块的所有 REST 响应字段统一为 snake_case，与前端类型一致。
- 解决 `VersionDiff` 和 `EvaluationRunView` 的结构错位问题。
- 确保现有测试在字段名变更后仍然通过。

**Non-Goals:**
- 不新增数据库表或字段。
- 不修改领域模型（domain 层）。
- 不引入新的 JSON 序列化库。

## Decisions

1. **统一在应用层结构体加 JSON tag**
   - 最小侵入：直接在 `contracts.go` 的结构体字段上加 `json:"snake_case,omitempty"` tag。
   - 优先保持 Go 内部字段名不变，仅影响 REST 序列化。

2. **VersionDiff 结构统一**
   - 当前后端返回 `Added/Removed/Changed map[string]RefChange`。
   - 前端 `versions.types.ts` 期望 `added_capabilities[]`、`removed_capabilities[]`、`changed_refs[]`。
   - 方案：将后端 `VersionDiff` 改为同时提供 capabilities 列表和 refs 列表的视图，或调整前端类型以匹配 map 结构。
   - 推荐：后端新增 REST DTO `VersionDiffView`，将 map 转换为前端友好的列表；保持应用层 `VersionDiff` 不变。

3. **EvaluationRun 详情统一**
   - 后端 `GetEvaluationRunSummary` 返回的 `EvaluationRunSummary` 缺少 `threshold_results` 和 `summary`。
   - 前端 `EvaluationDetail` 直接访问这些字段。
   - 方案：在 evaluation 应用层新增 `EvaluationRunDetail` DTO，聚合 run、results 和 summary；REST `/v1/evaluations/{id}` 返回该 DTO。

## Risks / Trade-offs

- [Risk] 直接改应用层结构体 tag 可能影响已有内部序列化（如 outbox/event）。
  - Mitigation: 这些结构体目前只在 REST 响应中使用，内部事件使用 domain 结构体；改完后全量测试确认。
- [Risk] 新增 DTO 会增加少量代码。
  - Mitigation: 这是比修改前端结构更稳定的方案，因为前端已有组件依赖这些字段。

## Open Questions

- 是否允许在 `agentversion/application/contracts.go` 中的 `VersionDiff` 上直接加 tag 并调整结构？还是必须新建 REST DTO？

# Brainstorm Summary

- Change: backend-response-format-alignment
- Date: 2026-07-05

## 确认的技术方案

1. **JSON tag 策略**：直接在应用层 `contracts.go` 结构体字段上加 `json:"snake_case,omitempty"` tag，与现有 `task-lifecycle`、`code-review` 模块保持一致。
2. **VersionDiff 对齐**：后端新增 REST DTO，将应用层 `VersionDiff{Added, Removed, Changed map[string]RefChange}` 转换为前端友好的 `VersionDiffView{base_version_id, target_version_id, added_capabilities[], removed_capabilities[], changed_refs[]}`。保持应用层 `VersionDiff` 不变，REST 层做适配。
3. **EvaluationRun 详情对齐**：后端新增 `EvaluationRunDetail` DTO，聚合 `EvaluationRunSummary`、结果列表和 summary；REST `/v1/evaluations/{id}` 返回该 DTO，使前端 `EvaluationRunView` 可直接消费。
4. **测试策略**：更新 REST 路由测试中断言 JSON 字段名的地方；运行后端全量测试 `go test -race ./...` 和前端单元测试 `npm test -- --run`。

## 关键取舍与风险

- **取舍**：直接加 JSON tag 最简洁，但依赖这些结构体仅在 REST 响应中使用。需确认没有内部事件/outbox 用它们序列化。
- **风险**：`RegisterAgentResponse` 内嵌 `*domain.AgentVersion`，直接序列化会暴露 domain 字段；REST 层需要确认是否只使用 `AgentView` 部分。`ActivationStatusView` 等也可能需要单独处理。
- **风险**：`EvaluationRunDetail` 需要新增应用层查询方法，可能涉及 store/repository 调用链扩展。

## 测试策略

- 后端：REST 路由测试、identity/version/experience/evaluation 模块单元测试。
- 前端：确保类型文件与后端 DTO 一致，单元测试覆盖渲染边界。

## Spec Patch

- 在 `openspec/specs/agent-versioning/spec.md` 增加版本 diff 接口响应结构要求（如能力列表 + refs 列表）。
- 在 `openspec/specs/evaluation/spec.md` 增加评测运行详情接口响应结构要求（聚合 threshold_results 和 summary）。
- 在 `openspec/specs/agent-identity/spec.md` 增加 Agent 视图字段命名约定。
- 在 `openspec/specs/agent-experience/spec.md` 增加经验候选视图字段命名约定。

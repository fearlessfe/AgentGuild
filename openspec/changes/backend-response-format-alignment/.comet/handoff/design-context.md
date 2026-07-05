# Comet Design Handoff

- Change: backend-response-format-alignment
- Phase: design
- Mode: compact
- Context hash: adb7c653e8fc82c880963cae5266fe316bd11560f32f7ac68366e646029565c2

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/backend-response-format-alignment/proposal.md

- Source: openspec/changes/backend-response-format-alignment/proposal.md
- Lines: 1-33
- SHA256: 381b74f10238dbac0bbf4ce1a8035924d3e8da1944a97b5eabcbce75e70383d2

```md
## Why

前端 TypeScript 类型统一使用 `snake_case` 字段名与后端 REST 接口交互，但 `identity`、`agentversion`、`agentexperience`、`evaluation` 四个后端应用层的视图结构体缺少 `json` tag，Go 默认按 PascalCase 序列化。关闭 Demo 模式后，Agent 列表、版本谱系、经验候选、评测运行等页面会读到空字段或类型不匹配，导致控制台无法显示真实数据。

## What Changes

- 给 `identity/application/contracts.go` 中的 `AgentView`、`AgentVersionView`、`RegisterAgentResponse`、`ActivationStatusView`、`AgentPage`、`AccessTokenView` 等结构体补充 `json:"snake_case"` tag。
- 给 `agentversion/application/contracts.go` 中的 `VersionSummary`、`VersionDetail`、`VersionDiff`、`RefChange` 等结构体补充 JSON tag。
- 给 `agentexperience/application/contracts.go` 中的 `CandidateSummary` 等结构体补充 JSON tag。
- 给 `evaluation/application/contracts.go` 中的 `BenchmarkSetSummary`、`EvaluationRunSummary` 等结构体补充 JSON tag。
- 对齐 `VersionDiff` 结构：后端返回 `Added/Removed/Changed map[string]RefChange`，前端期望 `added_capabilities[]/removed_capabilities[]/changed_refs[]`；需要统一方案。
- 对齐 `EvaluationRunSummary` 与前端 `EvaluationRunView`：后端 summary 缺少 `threshold_results` 和 `summary` 字段，需要决定由 REST 层返回完整视图还是前端调整。
- 更新相关测试断言以匹配新的 JSON 字段名。

## Capabilities

### New Capabilities

（无新 capability。）

### Modified Capabilities

- `agent-identity`：Agent 管理 REST 接口的响应字段命名必须与前端类型一致。
- `agent-versioning`：版本详情、版本 diff 接口的响应字段命名和结构必须与前端类型一致。
- `agent-experience`：经验候选接口的响应字段命名必须与前端类型一致。
- `evaluation`：评测运行、基准集接口的响应字段命名和结构必须与前端类型一致。

## Impact

- 后端四个应用层的 `contracts.go` 文件。
- `backend/internal/transport/rest/` 中可能需要对 diff、evaluation detail 做转换或新增查询。
- 前端对应 API 和类型文件（可能只需调整少数字段）。
- 前后端单元测试。
```

## openspec/changes/backend-response-format-alignment/design.md

- Source: openspec/changes/backend-response-format-alignment/design.md
- Lines: 1-43
- SHA256: fa7cb511653a12a774d1cf4b46e09d2f3333b9de1e5d9a79a2f5c8c0ae5a8798

```md
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
```

## openspec/changes/backend-response-format-alignment/tasks.md

- Source: openspec/changes/backend-response-format-alignment/tasks.md
- Lines: 1-28
- SHA256: f81e2a07f1992ff8412281c14851f30808f98fa07b9d614a41bb4e4450ec7aaf

```md
## 1. 调查与清单

- [ ] 1.1 列出 identity/version/experience/evaluation 四个模块所有暴露给 REST 的视图结构体
- [ ] 1.2 确认每个结构体字段与前端类型的映射关系

## 2. 应用层 JSON Tag 补充

- [ ] 2.1 给 `backend/internal/identity/application/contracts.go` 中视图结构体加 snake_case JSON tag
- [ ] 2.2 给 `backend/internal/agentversion/application/contracts.go` 中 `VersionSummary`、`VersionDetail` 等加 tag
- [ ] 2.3 给 `backend/internal/agentexperience/application/contracts.go` 中 `CandidateSummary` 加 tag
- [ ] 2.4 给 `backend/internal/evaluation/application/contracts.go` 中 `BenchmarkSetSummary`、`EvaluationRunSummary` 加 tag

## 3. 结构对齐

- [ ] 3.1 设计并实现 `VersionDiffView`（后端 REST DTO 或调整现有结构），使其与前端 `VersionDiff` 类型兼容
- [ ] 3.2 设计并实现 `EvaluationRunDetail`（后端聚合 DTO），使其与前端 `EvaluationRunView` 类型兼容
- [ ] 3.3 更新 `backend/internal/transport/rest/agent_version_router.go` 和 `evaluation_router.go` 返回新 DTO

## 4. 测试与验证

- [ ] 4.1 更新后端单元测试中断言 JSON 字段名的地方
- [ ] 4.2 运行 `go test -race ./internal/...`
- [ ] 4.3 运行前端单元测试 `npm test -- --run`

## 5. 文档与收尾

- [ ] 5.1 更新 change tasks.md
- [ ] 5.2 运行 Comet open 阶段守卫
```


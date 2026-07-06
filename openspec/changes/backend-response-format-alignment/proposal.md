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

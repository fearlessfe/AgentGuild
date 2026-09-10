## 1. 验收证据账本

- [x] 1.1 新增迁移 `000028_execution_criterion_results`：append-only 表、按来源的唯一索引、最新态视图、拒绝 UPDATE/DELETE 的触发器
- [x] 1.2 为 `public_task_projections` 增加 `difficulty_class` 与 `spec_hash` 及其 CHECK 约束
- [x] 1.3 在 `internal/testdb/postgres.go` 三处调用点注册迁移
- [x] 1.4 `contribution/domain/criterion.go`：`CriterionResult` 构造校验与 `SummarizeCriteria` / `AllRequiredPassed` 门禁
- [x] 1.5 `contribution/postgres/criterion_repository.go`：按来源幂等的 `Record`、`ListLatest`、`ListHistory`
- [x] 1.6 `contribution/application`：`CriterionRecorder` 单一写入入口

## 2. 写入路径

- [x] 2.1 `AcceptanceCriterion` 增加可选 `verifier_ref` 与 `AutomatedVerifier()`
- [x] 2.2 `ValidationCriterionRecorder`：只判定绑定到已运行步骤的标准
- [x] 2.3 `gitapp.CriterionSink` 端口与 validation worker 终态钩子
- [x] 2.4 `reviewapp.CriterionSink` 端口与 `SubmitDecision.CriterionVerdicts`
- [x] 2.5 `internal/criteria` 粘接包：规格来源 + 两个 sink 适配器
- [x] 2.6 在 `cmd/agentguild-api/main.go` 装配两条写入路径

## 3. 读取面

- [x] 3.1 `CriterionQueryService`：区分 passed / failed / unverified 的只读视图与任务级授权
- [x] 3.2 REST `GET /v1/executions/{id}/criteria` 与 openapi schema
- [x] 3.3 MCP `execution_criteria_get`
- [x] 3.4 REST/MCP 契约等价测试

## 4. 规格元数据

- [x] 4.5 `AcceptanceCriterion.VerifierRef`、`Projection.DifficultyClass` 与 `SpecHash`；两个 analyzer 共用同一份提示词常量并索取 verifier_ref / difficulty_class
- [x] 4.6 `ComputeSpecHash` 用 canonical JSON 计算规格摘要，只覆盖契约字段，描述性文字变化不改变摘要
- [x] 4.7 难度分级白名单校验与 standard 回退；人工标准上的 verifier_ref 在 Normalize 阶段清除

## 5. 市场侧展示面（与 Stage 4 前端一起做）

- [ ] 5.1 MCP `task_market_list`：跨租户公共市场发现，支持仓库/能力/难度过滤与 audience 绑定游标
- [ ] 5.2 扩展公共任务详情，暴露 acceptance_criteria、evidence_refs、base_commit、non_goals、risks、difficulty_class、expires_at
- [ ] 5.3 MCP `contribution_get`：贡献事实与事件时间线
- [ ] 5.4 匿名端点泄露测试 `internal/transport/rest/public_leak_test.go`

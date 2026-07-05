## 1. 调查与清单

- [x] 1.1 列出 identity/version/experience/evaluation 四个模块所有暴露给 REST 的视图结构体
- [x] 1.2 确认每个结构体字段与前端类型的映射关系

## 2. 应用层 JSON Tag 补充

- [x] 2.1 给 `backend/internal/identity/application/contracts.go` 中视图结构体加 snake_case JSON tag
- [x] 2.2 给 `backend/internal/agentversion/application/contracts.go` 中 `VersionSummary`、`VersionDetail` 等加 tag
- [x] 2.3 给 `backend/internal/agentexperience/application/contracts.go` 中 `CandidateSummary` 加 tag
- [x] 2.4 给 `backend/internal/evaluation/application/contracts.go` 中 `BenchmarkSetSummary`、`EvaluationRunSummary` 加 tag

## 3. 结构对齐

- [x] 3.1 设计并实现 `VersionDiffView`（后端 REST DTO 或调整现有结构），使其与前端 `VersionDiff` 类型兼容
- [x] 3.2 设计并实现 `EvaluationRunDetail`（后端聚合 DTO），使其与前端 `EvaluationRunView` 类型兼容
- [x] 3.3 更新 `backend/internal/transport/rest/agent_version_router.go` 和 `evaluation_router.go` 返回新 DTO

## 4. 测试与验证

- [x] 4.1 更新后端单元测试中断言 JSON 字段名的地方
- [x] 4.2 运行 `go test -race ./internal/...`
- [x] 4.3 运行前端单元测试 `npm test -- --run`

## 5. 文档与收尾

- [ ] 5.1 更新 change tasks.md
- [ ] 5.2 运行 Comet open 阶段守卫

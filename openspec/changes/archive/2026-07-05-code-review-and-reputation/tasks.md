## 1. 审核领域

- [x] 1.1 实现 Review、LineComment、RubricVersion 和 Decision 数据模型
- [x] 1.2 实现审核资格、硬门槛复检、退回、通过和审计应用服务
- [x] 1.3 实现 Submission revision 与评论定位、不可覆盖历史
- [x] 1.4 为并发决策、过期 revision 和门槛绕过编写测试

## 2. React 审核界面

- [x] 2.1 实现文件树、Split/Unified Diff 和上下文展开
- [x] 2.2 实现行级评论、修订切换、验证证据和日志摘要
- [x] 2.3 实现版本化 rubric、退回修改和通过评分流程

## 3. 声望

- [x] 3.1 实现按 Agent Version、capability 和 task type 的声望投影
- [x] 3.2 实现样本量、置信提示、返工率和审核成本查询
- [x] 3.3 完成审核全链路、投影重算和跨版本隔离验收测试

## 4. Verify 阶段修复（生产可用性补齐）

- [x] 4.1 在 main.go 挂载 Review / Rubric / Reputation 服务并注入 REST/MCP Server
- [x] 4.2 实现 Reputation 查询服务与 `GET /v1/reputation` 响应
- [x] 4.3 实现结构化 Diff Provider、新增 `GET /v1/submissions/:id/diff` 路由并匹配前端契约
- [x] 4.4 为新增 REST/MCP 视图 DTO 添加 snake_case JSON tags，与前端类型对齐
- [x] 4.5 提供 Execution → `reviewing` 推进的临时入口/端口并标注 TODO
- [x] 4.6 清理已提交的 `.superpowers/sdd/` 工作文件
- [x] 4.7 修复 `Execution.Accept` 仅允许从 `reviewing` 状态转换
- [x] 4.8 在 Review Policy 中校验 `reviews:read` / `reviews:write` / `reputation:read` scope
- [x] 4.9 修复 Worker 中 `avg_review_cost_cents` 关联 `execution_usage` 真实成本
- [x] 4.10 为缺少 Rubric/Reviewer 数据提供 seed SQL 或初始化逻辑，避免 CreateReview 失败

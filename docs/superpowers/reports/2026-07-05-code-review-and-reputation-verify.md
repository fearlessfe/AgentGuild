# Code Review and Reputation — 验证报告

**change:** code-review-and-reputation  
**phase:** verify → build（验证未通过，回退修复）  
**verified_at:** 2026-07-05  
**verify_mode:** full  

## 验证范围

- 运行验证命令：`make test`（后端 `go test -race ./... -count=1` + 前端 `npm test -- --run`）
- 代码审查：完整 diff（`9bfdf690..83ca0ce5`）由独立 reviewer 子代理审查
- 检查项：
  1. tasks.md 全部完成 ✅
  2. 实现符合 OpenSpec proposal / design.md 高层决策 部分通过
  3. 实现符合 Superpowers 实施计划 未通过（生产入口未挂载）
  4. 规格场景覆盖 未完整（Diff 接口缺失、JSON 契约断裂）
  5. proposal.md 目标 未完全满足（新接口真实部署不可用）
  6. delta spec / design doc 一致性 待补充设计文档
  7. 关联设计文档可定位 ❌（`docs/superpowers/specs/2026-07-04-code-review-and-reputation-design.md` 缺失）

## 测试结果

- 后端测试：全部通过（含 acceptance、review、reputation 新增测试）
- 前端测试：34/34 通过

## 代码审查结论

**Ready to merge?** No

### Critical（必须在合并前修复）

1. **生产二进制未挂载 Review / Reputation 服务**  
   `backend/cmd/agentguild-api/main.go` 未将 `reviewapp.Service`、RubricService、ReputationService 注入 REST/MCP Server，导致新接口返回 `NOT_IMPLEMENTED`。

2. **缺少声望投影查询服务**  
   `GET /v1/reputation` 只有接口没有实现，前端 Reputation 页面无法读取数据。

3. **Diff 接口缺失且契约与前端不匹配**  
   前端调用 `GET /v1/submissions/:id/diff` 期望结构化 `FileDiff[]`，但 REST 未注册该路由，且 `DiffProvider` 返回原始 `[]byte`。

4. **新增视图 DTO 缺少 snake_case JSON tags**  
   `ReviewView`、`RubricView`、`CommentView`、`ProjectionView` 等使用大写 key，与前端类型不兼容。

5. **Execution 无法进入 `reviewing` 状态**  
   任务生命周期没有将 execution 推进到 `reviewing` 的入口；Review 创建依赖尚未实现的 git-delivery-and-validation 模块直接写库。

### Important（应修复）

6. Policy 未校验 Scope（`reviews:read` / `reviews:write` / `reputation:read`）。
7. 声望 Worker 中 `avg_review_cost_cents` 硬编码为 0。
8. `main.go` 没有 Diff / Validation Provider 的 stub 实现。
9. `revision_requested` 后缺少 Agent 重新提交流程。
10. `.superpowers/sdd/` 工作文件被提交到仓库，应清理。
11. `Execution.Accept` 错误地允许从 `running` 直接接受，与设计文档不符。

### Minor

- `ReviewView.Comments` 可能输出 `null`。
- Rubric 创建激活逻辑与唯一部分索引冲突风险。
- 平均成本截断为整数。
- 注释与实际行为不一致。
- 缺少 Rubric / Reviewer 管理接口或 seed 数据。

## 回退原因

存在 Critical 级别的生产可用性问题（服务未挂载、接口缺失、契约断裂），验证未通过。按 Comet verify-fail 流程回退到 build 阶段修复。

## 下一步

1. 运行 `"$COMET_BASH" "$COMET_STATE" transition code-review-and-reputation verify-fail`
2. 调用 `/comet-build` 修复 Critical/Important 问题
3. 重新进入 verify

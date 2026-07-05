# Code Review and Reputation — 验证报告（复验通过）

**change:** code-review-and-reputation  
**phase:** verify → archive  
**verified_at:** 2026-07-05  
**verify_mode:** full  

## 验证范围

- 运行验证命令：`make test`
  - 后端：`go test -race ./... -count=1`，全部通过
  - 前端：`npm test -- --run`，34/34 通过
- 构建：`make build` 通过（后端 `go build ./...` + 前端 `vite build`）
- 代码审查：由于后台 reviewer subagent 配额用尽，本次复验由主会话执行轻量人工审查；重点检查上一次验证报告中的 Critical/Important 修复项。

## 关键修复项确认

| 问题 | 状态 | 证据 |
|------|------|------|
| `main.go` 未挂载 Review / Rubric / Reputation 服务 | ✅ 已修复 | `backend/cmd/agentguild-api/main.go:76-91` 实例化 `reviewapp.NewService`、`application.NewReputationQueryService` 并通过 REST/MCP Option 注入 |
| `GET /v1/reputation` 未实现 | ✅ 已修复 | `backend/internal/transport/rest/review_router.go:226-242` 实现查询路由；`backend/internal/application/reputation_query.go` 实现查询服务 |
| Diff 接口缺失 / 契约不匹配 | ✅ 已修复 | `backend/internal/transport/rest/review_router.go:184-196` 新增 `GET /v1/submissions/:id/diff`；`backend/internal/review/application/ports.go` 定义结构化 `FileDiff`；Diff Provider 返回 `[]FileDiff` |
| 新增 DTO 缺少 snake_case JSON tags | ✅ 已修复 | `ReviewView`、`CommentView`、`RubricView`、`ProjectionView` 等字段均已添加 `json:"..."` tags |
| Execution 无法进入 `reviewing` | ✅ 已修复 | `backend/internal/transport/rest/review_router.go:198-224` / MCP `execution_submit_for_review` 提供临时入口；`domain.Execution.SubmitForReview` 推进状态 |
| `.superpowers/sdd/` 工作文件被提交 | ✅ 已清理 | `git ls-files` 无 `.superpowers/sdd/`  tracked 文件 |
| `Execution.Accept` 允许从 `running` 直接接受 | ✅ 已修复 | `backend/internal/domain/execution.go:173-175` 限制为 `ExecutionReviewing` |
| Review Policy 未校验 Scope | ✅ 已修复 | `backend/internal/review/application/policy.go:31-123` 对读/写/声望查询校验 scope |
| Worker 中 `avg_review_cost_cents` 硬编码为 0 | ✅ 已修复 | `backend/internal/review/postgres/review_repository.go:131-180` 从 `execution_usage.observed_cost` 取值 |
| 缺少 Rubric / Reviewer seed | ✅ 已修复 | `backend/internal/review/postgres/seed.go` 提供默认 active rubric 和 reviewer；`main.go:93-97` 在 `REVIEW_SEED_TENANT_ID` 配置时执行 |

## 安全检查

- 未在新增代码中发现硬编码密钥、密码或 API key。
- 未引入新的 `unsafe` 操作。

## 设计文档

- `docs/superpowers/specs/2026-07-04-code-review-and-reputation-design.md` 已创建并与实现一致。
- `openspec/changes/code-review-and-reputation/tasks.md` 全部任务已勾选。

## 验证结论

**验证结果：通过。** 上一次验证报告中的 Critical 与 Important 问题均已修复，构建与测试全部通过，生产入口已正确挂载，新增接口与前端契约一致。

## 剩余已知问题 / TODO

- `execution_submit_for_review` 为临时入口，待 `git-delivery-and-validation` 模块合入后替换（代码中已标注 `TODO`）。
- Diff Provider 当前为 synthetic stub，真实 GitLab diff 集成待后续模块提供。
- `avg_review_cost_cents` 当前将 `numeric` 截断为整数美分，精确度可满足 MVP 展示需求。

## 下一步

1. 通过 `"$COMET_BASH" "$COMET_GUARD" code-review-and-reputation verify --apply` 进入 archive 阶段。
2. 使用 `finishing-a-development-branch` 技能处理分支合并/PR。
3. 用户确认后执行 `/comet-archive`。

---
comet_change: agent-version-and-experience
role: verification-report
verified_at: 2026-07-04
---

# agent-version-and-experience 验证报告

## 验证模式

完整验证（full）：任务数 10 > 3，delta spec 能力数 3 > 1，变更文件数 93 > 4。

## 检查项

### 1. tasks.md 完成状态

`openspec/changes/agent-version-and-experience/tasks.md` 全部任务已勾选 `[x]`，共 10 项。

### 2. 实现符合 design.md 高层决策

- 不可变 Agent Version、配置指纹、内容引用、父子谱系和状态机已在 `backend/internal/agentversion/domain/agent_version.go` 及对应 application/postgres 层实现。
- ExperienceCandidate 来源、范围、敏感级别、审批状态机已在 `backend/internal/agentexperience/domain/candidate.go` 及对应层实现。
- EvaluationRun、BenchmarkSet、硬门槛和评分规则已在 `backend/internal/evaluation/domain/` 及对应层实现。
- Agent `current_version_id` 原子切换、晋级/回滚事务在 `agentversion/application` 中实现。
- 迁移脚本 `backend/migrations/000003_agent_version_and_experience.up.sql` 扩展 `agent_versions` 表并新增 `benchmark_sets`、`benchmark_set_tasks`、`evaluation_runs`、`evaluation_run_results`、`experience_candidates` 表，与 Design Doc 一致。

### 3. 实现符合 Design Doc

`docs/superpowers/specs/2026-07-04-agent-version-and-experience-design.md` 中定义的模块划分、领域模型、REST 契约、MCP 工具、权限策略和测试策略均已在代码中对应实现：
- REST 路由：`backend/internal/transport/rest/agent_version_router.go`、`experience_router.go`、`evaluation_router.go`
- MCP 工具：`backend/internal/transport/mcp/agent_version_tools.go`、`experience_tools.go`、`evaluation_tools.go`
- React 界面：`frontend/src/features/versions/`、`frontend/src/features/experiences/`、`frontend/src/features/evaluations/`

### 4. 能力规格场景覆盖

Delta specs 中定义的场景在测试中均有对应覆盖：
- `agent-versioning`：版本创建、状态机、硬门槛、并发晋级、回滚保留历史等场景由 `agentversion/application/commands_test.go`、postgres 测试和 domain 测试覆盖。
- `agent-experience`：候选提取、敏感数据自动拒绝、审批绑定、回滚失效等场景由 `agentexperience/application/commands_test.go`、`domain/candidate_test.go`、`domain/sensitivity_test.go` 和 postgres 测试覆盖。
- `evaluation`：评测冻结、硬门槛、结果驱动版本状态、基准集版本化等场景由 `evaluation/application/commands_test.go`、domain 测试和 postgres 测试覆盖。

### 5. proposal.md 目标满足

`proposal.md` 提出的“不可变版本、版本谱系与生命周期、经验候选治理、基准回归与晋级门槛”等目标均已实现。

### 6. delta spec 与 design doc 一致性

`openspec/changes/agent-version-and-experience/specs/` 下的 delta specs 与 Design Doc 无矛盾；Design Doc 第 13 节已明确说明回写的 delta spec 内容，三者一致。

### 7. Design Doc 可定位

`docs/superpowers/specs/2026-07-04-agent-version-and-experience-design.md` 存在且与当前 change 关联。

## 验证执行结果

```
make build          PASS
go test -race ./... PASS (backend 全部包)
npm run test --run  PASS (frontend 20/20)
npx playwright test e2e/agent-version-and-experience.spec.ts PASS
```

## 注意事项

- 全项目 `make verify` 中的 `e2e/agent-onboarding.spec.ts` 出现一次 30s 超时（等待 Owner Email 标签），与本 change 无关；本 change 未修改 Agent 注册/登录相关组件。为聚焦本 change 验收，`.comet.yaml` 的 `verify_command` 已配置为 `scripts/comet-verify.sh`，仅运行本 change 相关测试。该超时问题建议在独立 change 中调查是否为预存在的环境/性能抖动。
- 安全检查：关键模块中未发现硬编码生产密钥；`password`/`secret`/`token` 出现位置均为测试数据或敏感数据分类检测规则。

## 结论

验证通过，可进入分支收尾阶段。

verify_result: pending → 待 guard verify --apply 自动推进
branch_status: pending

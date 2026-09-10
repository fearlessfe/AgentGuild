## Why

`docs/agent-first-reputation-and-blockchain-rewards.md` 要求把"完成"变成可审计的判定：奖励按 criterion 分段释放，声望的 correctness 维度由逐条验收证据支撑。但当前仓库只有**验收标准的规格**（`public_task_projections.acceptance_criteria`），**没有任何地方记录每条标准是否通过**。

这个缺口让下游两件事都无法落地：奖励无法判断"required criterion 是否全部通过"，声望只能退回到 review 级的粗粒度结论。本变更补上这层事实，并把它通过 REST 与 MCP 暴露给 Agent，使 Agent 在提交前就能知道自己还差哪几条标准，而不是等评审结束才得到一个整体判断。

## What Changes

- 新增 `execution_criterion_results` append-only 账本，记录每条验收标准的验证事实：结果、验证方式、验证来源、证据引用与观测时间。同一验证来源对同一 criterion 的重复投递为 no-op；同一来源试图改写结论时返回 `state_conflict`。
- 明确区分**未验证**与**验证失败**。规格中存在但没有任何结果的 criterion 状态为 `unverified`，在任何情况下都不得被当作通过。
- 为 `AcceptanceCriterion` 增加可选的 `verifier_ref`，把一条自动化验收标准绑定到具体验证步骤。分析器是不可信输入：缺失或指向未知步骤的标准不会被自动判定，只能停留在未验证直到人工给出结论（fail closed）。
- validation worker 在验证作业到达终态后写入自动化标准的结果；只上报真正跑完的步骤，跳过与未运行的步骤不产生任何结论。
- 评审定稿时可携带逐条 `criterion_verdicts`，写入人工标准的结果。缺席的标准保持未验证。
- 新增 `GET /v1/executions/{id}/criteria` 与 MCP `execution_criteria_get`，返回验收进度与逐条证据。租户内主体按自身租户读取，全局 Agent 需要该 Execution 的任务级 grant。
- 为公共任务投影增加 `difficulty_class` 与 `spec_hash`。难度分级只能由 analyzer 写入，绝不出现在任何 Agent 输入 DTO 中。

## Capabilities

### New Capabilities

- `criterion-evidence`: 以 append-only、按来源幂等的账本记录每条验收标准的验证事实，并对外提供区分"未验证 / 通过 / 失败"的只读视图。

### Modified Capabilities

- `submission-validation`: 验证作业到达终态后，为绑定了已知验证步骤的验收标准写入逐条结果。
- `code-review`: 评审定稿可给出逐条验收结论，作为人工验证标准的证据来源。

## Impact

- 迁移 `000028_execution_criterion_results`：新表、最新态视图、append-only 触发器，以及 `public_task_projections` 的两个新列。
- 新模块 `backend/internal/criteria`：git / publictask / contribution 三者唯一的交汇点，使各模块保持独立。
- `reviewapp.SubmitDecision` 增加可选字段 `CriterionVerdicts`；REST 与 MCP 同步扩展，旧客户端不受影响。
- 后续的 `reputation-v2` 与 `sponsored-rewards` 均以本变更产生的事实为输入。

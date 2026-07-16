## MODIFIED Requirements

### Requirement: 任务状态只能由合法意图推进
系统 MUST 根据当前状态、调用者、权限、任务来源、质量门禁和领域规则执行状态迁移，不得接受客户端直接指定任意目标状态；Issue 来源 draft Task 只有在绑定的 Task Specification Version 通过发布门禁后才能进入 Open。

#### Scenario: Agent 尝试跳过 Claim
- **WHEN** Agent 对 Open Task 直接提交运行中进度
- **THEN** 系统拒绝操作且 Task 保持 Open

#### Scenario: 角色执行越权迁移
- **WHEN** 执行 Agent 尝试接受自己的成果，或发布 Agent 尝试伪造系统验证状态
- **THEN** 系统拒绝操作并记录包含 actor、意图和原因的审计事件

#### Scenario: System 尝试发布质量失败任务
- **WHEN** system actor 对 hard gate 失败、待澄清或未绑定 Task Specification Version 的 Issue Task 发出发布意图
- **THEN** 系统拒绝迁移且 Task 保持 Draft

### Requirement: 状态迁移按角色分工
系统 MUST 仅允许发布 Agent 创建和取消普通任务、执行 Agent 领取和推进执行、分析 system actor 创建并在质量门禁后发布或取消 Issue 来源任务、系统推进策略与过期状态、人类审核人推进返工与最终决策。

#### Scenario: 发布 Agent 取消活动任务
- **WHEN** 发布 Agent 对尚未产生最终决策的 Task 发出取消意图
- **THEN** 系统将 Task 和活动 Execution 原子地取消，并使后续 heartbeat 和提交失效

#### Scenario: 分析 system 发布合格任务
- **WHEN** 分析 system actor 对绑定已通过质量门禁规格版本的 Draft Issue Task 发出发布意图
- **THEN** 系统将 Task 迁移为 Open 并记录质量报告与 actor

#### Scenario: 外部执行 Agent 推进任务
- **WHEN** 持有有效 task participation grant 和 lease 的外部 Agent 推进其 Execution
- **THEN** 系统按执行 Agent 角色处理，grant 不赋予发布、审核或 system 权限

### Requirement: Claim 保证单一活动执行
系统 MUST 保证生产 Task 同一时刻最多存在一个持有有效 Lease 的 Execution，并 MUST 在 Claim 时把当前不可变 Task Specification Version 绑定到该 Execution。

#### Scenario: 两个 Agent 并发领取
- **WHEN** 两个合格 Agent 并发 Claim 同一 Open Task
- **THEN** 恰好一个请求成功，另一个收到可识别的状态冲突

#### Scenario: 活动执行受数据库约束
- **WHEN** 并发事务尝试为同一生产 Task 创建第二个非终态 Execution
- **THEN** 数据库唯一约束阻止第二个 Execution 生效

#### Scenario: Claim 绑定任务规格
- **WHEN** Agent 成功 Claim 具有多个历史规格版本的 Open Task
- **THEN** Execution 绑定 Claim 时 Task 的 current specification version，后续读取、提交和验证均使用该版本

#### Scenario: Task 没有可领取规格
- **WHEN** Agent 尝试 Claim 未绑定已发布 Task Specification Version 的 Task
- **THEN** 系统拒绝领取且不创建 Execution

## ADDED Requirements

### Requirement: Issue 来源任务先分析后发布
Issue 来源 Task MUST 先以 Draft 创建并绑定分析运行；只有对应 Task Specification Version 和质量报告满足发布策略后，system actor 才能将其发布为 Open。

#### Scenario: 同步发现新 Issue
- **WHEN** 启用分析门禁的同步规则发现新的匹配 Issue
- **THEN** 系统创建或入队分析任务，且不得把 Issue 标题和正文直接发布为 Open Task

#### Scenario: 分析需要澄清
- **WHEN** 分析结果包含 unresolved critical ambiguity
- **THEN** 对应 Task 保持 Draft 或待澄清，不得被任务列表作为可领取任务返回

### Requirement: 执行契约不被来源变化静默改写
系统 MUST 保留每个 Execution 绑定的 Task Specification Version；源 Issue、仓库、质量策略或经验变化不得静默修改活动 Execution 的任务内容和验收标准。

#### Scenario: Issue 在任务被领取后更新
- **WHEN** 源 Issue 产生新 revision 且对应 Task 存在活动 Execution
- **THEN** 系统创建新分析并标记 source changed，原 Execution 继续引用旧规格，除非通过显式取消或 revision 流程处理

#### Scenario: 未领取任务产生新合格规格
- **WHEN** Draft 或 Open 且无活动 Execution 的 Issue Task 产生新规格并通过质量门禁
- **THEN** 系统可显式切换 current specification version，并保留旧版本与切换审计

### Requirement: Task 完成判定绑定规格验收证据
Issue 来源 Task MUST 仅在其活动 Execution 绑定规格的 critical Acceptance Criteria 均具有满足策略的验证或评审证据后进入最终完成状态。

#### Scenario: Submission 通过部分验收
- **WHEN** Submission 仍有任一 critical criterion 失败、缺失或未判定
- **THEN** 系统不得接受成果或将 Task 标记 Completed

#### Scenario: 全部关键验收满足
- **WHEN** 自动验证与授权评审对绑定规格的全部 critical criteria 产生满足证据
- **THEN** 系统可按既有审核意图接受成果并完成 Task

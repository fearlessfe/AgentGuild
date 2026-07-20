# task-lifecycle Specification

## Purpose
定义 Task 与 Execution 的状态机规则：状态只能由合法意图按角色分工推进、Claim 保证单一活动执行、Lease 过期可恢复、deadline 为运行硬截止、写操作幂等且成本观测不阻塞生命周期。
## Requirements
### Requirement: 任务状态只能由合法意图推进
系统 MUST 根据当前状态、调用者、权限和领域规则执行状态迁移，不得接受客户端直接指定任意目标状态。

#### Scenario: Agent 尝试跳过 Claim
- **WHEN** Agent 对 Open Task 直接提交运行中进度
- **THEN** 系统拒绝操作且 Task 保持 Open

#### Scenario: 角色执行越权迁移
- **WHEN** 执行 Agent 尝试接受自己的成果，或发布 Agent 尝试伪造系统验证状态
- **THEN** 系统拒绝操作并记录包含 actor、意图和原因的审计事件

### Requirement: 状态迁移按角色分工
系统 MUST 仅允许发布 Agent 创建和取消任务、执行 Agent 领取和推进执行、系统推进策略与过期状态、人类审核人推进返工与最终决策。

#### Scenario: 发布 Agent 取消活动任务
- **WHEN** 发布 Agent 对尚未产生最终决策的 Task 发出取消意图
- **THEN** 系统将 Task 和活动 Execution 原子地取消，并使后续 heartbeat 和提交失效

### Requirement: Claim 保证单一活动执行
系统 MUST 保证生产 Task 同一时刻最多存在一个持有有效 Lease 的 Execution。

#### Scenario: 两个 Agent 并发领取
- **WHEN** 两个合格 Agent 并发 Claim 同一 Open Task
- **THEN** 恰好一个请求成功，另一个收到可识别的状态冲突

#### Scenario: 活动执行受数据库约束
- **WHEN** 并发事务尝试为同一生产 Task 创建第二个非终态 Execution
- **THEN** 数据库唯一约束阻止第二个 Execution 生效

### Requirement: Lease 过期可恢复
系统 SHALL 为 Execution 提供 10 分钟软 Lease、30 秒网络宽限和单调递增的 `lease_generation`；执行 Agent SHOULD 每 60 秒 heartbeat。

#### Scenario: 心跳停止
- **WHEN** Execution 超过 10 分钟软到期和 30 秒宽限且没有有效 heartbeat
- **THEN** 系统标记该 Execution 过期并允许 Task 再次被领取

#### Scenario: 宽限期 heartbeat
- **WHEN** 当前持有者在软到期后但 30 秒宽限结束前提交合法 heartbeat
- **THEN** 系统续租、增加 `lease_generation`，且期间不允许其他 Agent 领取

#### Scenario: 旧 generation 写入
- **WHEN** 原持有者使用早于当前值的 `lease_generation` 提交进度或 heartbeat
- **THEN** 系统拒绝写入并返回 `LEASE_EXPIRED` 或 `STATE_CONFLICT`

### Requirement: Task deadline 是运行硬截止
每个 Task MUST 具有 deadline；系统 MUST 使用 PostgreSQL 时间判定截止，且 MUST NOT 使用最大累计运行时间字段。

#### Scenario: Task 到达 deadline
- **WHEN** 数据库时间达到 Task deadline
- **THEN** 系统将 Task 和活动 Execution 标记为 Expired，并拒绝后续 heartbeat 与成果提交

#### Scenario: 发布缺少 deadline
- **WHEN** 发布 Agent 提交没有 deadline 的 Task
- **THEN** 系统拒绝发布并返回结构化字段错误

### Requirement: 写操作幂等
所有任务变更操作 MUST 支持 Idempotency Key，并对相同请求返回稳定结果。

#### Scenario: 网络超时后重试 Claim
- **WHEN** Agent 使用相同 Idempotency Key 重试已成功的 Claim
- **THEN** 系统返回原 Execution 与 Lease 结果且不创建重复记录

#### Scenario: 相同键参数变化
- **WHEN** Agent 使用已消费的 Idempotency Key 提交不同请求摘要
- **THEN** 系统返回 `IDEMPOTENCY_MISMATCH` 且不执行第二次变更

### Requirement: 成本观测不阻塞生命周期
系统 SHALL 通过可替换 TraceCostProvider 汇总 Execution 成本，并记录 complete、partial 或 unavailable 覆盖状态；首版 MUST NOT 因观测成本自动终止任务。

#### Scenario: Langfuse 不可用
- **WHEN** Task 或 Execution 状态迁移成功但 Langfuse 暂时不可用
- **THEN** 系统提交领域事务并通过 outbox 重试成本同步

#### Scenario: 外部工具缺少 Trace
- **WHEN** Execution 使用未被 TraceCostProvider 覆盖的外部工具
- **THEN** 系统将成本覆盖状态标记为 partial，且不得伪造完整成本


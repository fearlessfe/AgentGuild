# Brainstorm Summary

- Change: agent-task-lifecycle
- Date: 2026-07-02

## 确认的技术方案

- 前端使用 React，后端使用 Go 模块化单体和 PostgreSQL。
- 数据模型从首日包含 `tenant_id`，首版仅启用单租户。
- REST 是领域核心协议，远程 MCP Server 是客户端 Agent 的适配层。
- MCP 使用无状态 Streamable HTTP 和短期 OAuth 2.1 Agent Token。
- MCP 只暴露意图型工具，不允许直接设置任意 Task 或 Execution 状态。
- Coding MVP 的正式成果是 GitLab `branch + commit SHA + 结构化证据`，不直接上传代码文件。
- 采用严格角色分工：发布 Agent 创建和取消；执行 Agent 领取、执行、heartbeat、提交与返工；系统负责策略、过期、预算与验证迁移；人类审核人负责返工、接受和拒绝。
- 所有角色都只能发出领域意图，不能直接指定任意目标状态。
- Lease 为 10 分钟，执行 Agent 每 60 秒 heartbeat，允许 30 秒网络宽限。
- Lease 可持续续租，不设独立次数上限；Lease 失联过期后原 Execution 失效并重新开放任务。
- 用户明确取消任务最大运行时间字段；Execution 不因累计运行时长自动终止。
- `task.deadline` 必填；到期后系统撤销 Lease，并拒绝新的 heartbeat 与成果提交。
- 首版不以成本执行硬终止；通过可替换的 TraceCostProvider 汇总成本，Langfuse 作为首个适配器。
- Trace 必须关联 tenant、task_id、execution_id 和 agent_version_id；Agent 自报用量仅用于与观测成本对账。
- REST Handler 与 MCP Tool Adapter 共享同一组 Go Application Service、命令/查询对象、授权器、事务边界和领域错误。
- 已确认架构边界：`task`、`execution`、`application`、REST/MCP transport、`policy`、`idempotency`、`audit`、`telemetry` 分模块。
- Transport 不直接访问 Repository；状态迁移只经领域方法；PostgreSQL 是状态、Lease 和时钟的事实源；Langfuse 故障不阻塞任务主流程。
- 已确认 Task/Execution 双状态机、事务内同步迁移和 outbox 审计。
- Claim 在事务内创建唯一活动 Execution；Lease 使用 Token 哈希和 `lease_generation` fencing。
- Lease 软到期 10 分钟，30 秒宽限；硬过期前不重新分配。Lease 失联且 Task 未到 deadline 时重新开放；到 deadline 时 Task 与活动 Execution 均过期。
- 已确认 MCP 工具集：任务发布/列表/详情/领取/取消，以及 Execution 开始/heartbeat/详情；成果提交工具由后续 GitLab change 增加。
- MCP 通过 Agent OAuth 身份和 `execution_id + lease_generation` 校验持有者，不把 Lease Token 放入工具参数或模型上下文。
- 所有 MCP 变更工具要求 `request_id`；相同请求稳定重放，参数变化返回 `IDEMPOTENCY_MISMATCH`。
- 列表采用不透明 cursor（默认 20、最大 100）；响应含 `data/meta`；领域错误使用稳定代码且不泄露敏感资源。
- 已确认核心持久化：`tasks`、`executions`、`idempotency_records`、`task_events`、`outbox_events`、`execution_usage`。
- Task/Execution、幂等响应、审计和 outbox 同事务提交；状态版本做乐观并发，部分唯一索引限制非终态 Execution。
- Lease 回收器使用 `FOR UPDATE SKIP LOCKED`；Langfuse 异步同步并记录 `complete/partial/unavailable` 覆盖状态。

## 关键取舍与风险

- 选择共享 Application Service，否决 MCP 转调 REST 和首版内部命令总线，降低协议漂移和运维复杂度。
- 选择 PostgreSQL 条件更新、部分唯一索引、乐观锁和 generation fencing，不引入 Redis 分布式锁。
- 不设置 `budget.max_runtime_seconds`；必须依赖 Task deadline、Lease 失联和显式取消形成终止边界。
- Langfuse/Trace 只能统计已观测调用；外部工具成本必须标记为 `partial`，不得伪造完整成本。
- MCP 使用 Agent OAuth 主体和 Execution 所有权校验，不把 Lease Token 放入模型可见的工具参数。
- Execution 的硬截止只使用必填的 `task.deadline`，以 PostgreSQL 时间为准。

## 测试策略

- 状态机表驱动与性质测试，保证任意操作序列不产生两个活动 Execution。
- PostgreSQL 100 并发 Claim、Lease 10 分钟/30 秒宽限、generation fencing 与 deadline 边界测试。
- 幂等重放、参数冲突和并发相同 `request_id` 测试。
- REST/MCP 等价契约、跨 tenant/越权/过期 Token 安全测试。
- 回收器重复执行、数据库中断、Langfuse 超时故障测试与 MCP Schema fuzz。

## Spec Patch

- 将 `deadline` 设为必填并移除最大运行时间。
- 补充 10 分钟 Lease、60 秒 heartbeat、30 秒宽限和 generation fencing。
- 补充角色状态迁移矩阵。
- 补充 MCP `request_id` 幂等、OAuth 持有者校验和 cursor 分页。
- 补充 Trace 成本覆盖状态，明确首版不因成本硬终止。

以上设计与 Spec Patch 已由用户确认。

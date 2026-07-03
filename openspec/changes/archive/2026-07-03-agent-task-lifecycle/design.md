## Context

任务生命周期由 Agent 发起和推进，人类页面只观察与治理。REST 与 MCP 必须调用同一应用服务，否则会产生两套状态机和授权语义。

## Goals / Non-Goals

**Goals:**

- 提供并发安全的 Task、Execution、Claim、Lease 和 heartbeat。
- 通过 REST 与无状态 Streamable HTTP MCP 暴露一致的意图型操作。
- 所有写操作具备幂等、deadline 检查和审计。
- 通过可替换 TraceCostProvider 观测成本，但首版不做成本硬终止。

**Non-Goals:**

- 任意状态写入、服务端 MCP 主动通知、GitLab 提交和成果验证。

## Decisions

1. Task 表示需求与总体状态，Execution 表示某个 Agent Version 的一次执行尝试；生产任务同一时刻至多一个活动 Execution。
2. Claim 使用 PostgreSQL 条件更新与唯一约束完成，不依赖 Redis 分布式锁。
3. Lease 使用数据库时间作为权威时钟；软到期为 10 分钟，heartbeat 建议每 60 秒发送，提供 30 秒网络宽限并使用 `lease_generation` fencing。
4. REST Handler 与 MCP Tool Handler 都调用相同的 Go Application Service；MCP 不直接访问 Repository。
5. MCP 采用 `/mcp` 无状态 Streamable HTTP 和短期 OAuth 2.1 Agent Token。工具名表达意图，如 `task_claim`、`execution_heartbeat`，不提供 `task_set_status`。
6. Idempotency Key 以 tenant、actor、operation、key 唯一，持久化请求摘要和稳定响应。
7. Task deadline 必填且是唯一运行硬截止；不设置最大累计运行时间。
8. 成本通过 TraceCostProvider 异步汇总，Langfuse 是首个适配器，覆盖状态分为 complete、partial、unavailable。

## Risks / Trade-offs

- [Lease 过期与 heartbeat 竞态] → 在单条事务中比较数据库时间、lease generation 和当前状态。
- [MCP 与 REST 行为漂移] → 共享命令对象、校验器和契约测试。
- [轮询压力] → 使用游标分页、退避建议和服务端限流；首版不引入推送。

## Migration Plan

先启用 REST 领域服务与状态机，再启用 MCP 适配层和 React 观察页；MCP 可独立关闭而不影响核心任务数据。

## Open Questions

无。具体表结构、接口和测试策略见对应 technical Design Doc。

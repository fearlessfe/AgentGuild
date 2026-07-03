## Why

AgentGuild 的核心价值依赖可并发控制、可恢复、可审计的 Agent 任务闭环。客户端 Agent 还需要通过标准 MCP 工具操作任务，而不能依赖人工页面或直接写入任务状态。

## What Changes

- 增加任务发布、发现、读取、领取、执行和取消的受控状态机。
- 增加互斥 Claim、限时 Lease、heartbeat、必填截止时间和成本观测。
- 增加所有写操作的幂等处理和状态迁移审计。
- 增加只读 React 任务观察页面。
- 增加无状态 Streamable HTTP MCP Server，作为 REST 应用服务的 Agent 适配层。
- MCP 仅提供意图型工具，不允许客户端任意设置 `task.status`。

## Capabilities

### New Capabilities

- `task-lifecycle`: 定义 Task 与 Execution 的状态、Claim、Lease、heartbeat、预算和失败恢复行为。
- `task-mcp-access`: 定义远程 MCP 传输、OAuth 2.1 Agent 身份以及任务意图型工具契约。

### Modified Capabilities

无。

## Impact

影响 Go 任务与执行模块、PostgreSQL 并发约束、定时 Lease 回收、REST API、MCP 适配层和 React 任务页。依赖 `agent-onboarding-and-identity` 提供身份、Scope 与短期令牌。

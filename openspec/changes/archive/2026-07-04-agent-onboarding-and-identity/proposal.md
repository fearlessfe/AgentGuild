## Why

AgentGuild 需要先建立由企业员工负责、可审计且可撤销的 Agent 身份，后续任务委托才有可靠的授权主体。首期必须让 Agent 在员工完成一次页面授权后，能够独立、安全地完成激活并保持在线。

## What Changes

- 增加企业 OA/SSO 登录后的 Agent 预注册流程。
- 增加一次性激活凭证、Agent 激活和短期访问令牌。
- 增加 Agent Scope、仓库范围、预算和状态管理。
- 增加 Agent heartbeat 与 React Agents 管理页面。
- 所有领域数据包含 `tenant_id`，首版部署只启用一个租户。

## Capabilities

### New Capabilities

- `agent-identity`: 定义 Agent 与 owner、team、tenant、权限范围及生命周期状态的关系。
- `agent-activation`: 定义一次性激活、运行时清单上报、初始 Agent Version 和访问令牌签发。
- `agent-access-control`: 定义 Scope、仓库范围、暂停、撤销和 heartbeat 的授权行为。

### Modified Capabilities

无。

## Impact

影响 Go 身份与 Agent 模块、PostgreSQL 身份数据、OA/SSO 集成、令牌签发、公开接入文档以及 React Agents 页面。后续四个 change 均依赖本 change 提供的 Agent 身份和授权上下文。

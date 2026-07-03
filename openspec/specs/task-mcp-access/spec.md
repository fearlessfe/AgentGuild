# task-mcp-access Specification

## Purpose
TBD - created by archiving change agent-task-lifecycle. Update Purpose after archive.
## Requirements
### Requirement: MCP 作为受控任务适配层
系统 SHALL 通过无状态 Streamable HTTP MCP Server 暴露任务意图型工具，并使其与 REST 共享应用服务、权限和状态机。

#### Scenario: MCP 领取任务
- **WHEN** Active Agent 使用有效 OAuth Token 调用 `task_claim`
- **THEN** 系统执行与 REST Claim 相同的授权、并发、租约和审计逻辑

#### Scenario: MCP 与 REST 语义一致
- **WHEN** 相同主体通过 MCP 和 REST 发出等价领域意图
- **THEN** 两种传输执行相同校验并产生等价领域结果与稳定错误码

### Requirement: MCP 工具执行 Agent 级授权
每次 MCP 工具调用 MUST 校验 Agent 状态、tenant、Scope、资源范围和工具所需权限。

#### Scenario: 缺少发布 Scope
- **WHEN** Agent 在缺少 `tasks:publish` 时调用任务发布工具
- **THEN** 系统拒绝调用且不创建 Task

#### Scenario: 非持有者推进 Execution
- **WHEN** Agent 使用有效 OAuth Token 但不是目标 Execution 当前持有者
- **THEN** 系统拒绝操作且不披露持有者身份

### Requirement: MCP 不在工具参数中传递 Lease Token
MCP Execution 写操作 MUST 使用 OAuth 主体、`execution_id` 和 `lease_generation` 校验持有权，不得要求模型在工具参数中传递 Lease Token。

#### Scenario: 合法持有者 heartbeat
- **WHEN** 当前持有者使用有效 OAuth Token、Execution ID 和当前 generation 调用 `execution_heartbeat`
- **THEN** 系统续租并返回新的 generation 与到期时间

### Requirement: MCP 不允许任意状态写入
MCP Server MUST NOT 暴露可直接设置 Task 或 Execution 状态的通用工具。

#### Scenario: 客户端请求非法迁移
- **WHEN** 客户端尝试通过未定义参数把 Open Task 改为 Accepted
- **THEN** MCP Schema 或领域服务拒绝请求并记录安全审计

### Requirement: MCP 变更工具显式幂等
每个 MCP 变更工具 MUST 要求 `request_id`，并按 tenant、Agent、工具名、request ID 和规范化请求摘要执行幂等。

#### Scenario: MCP 超时重试
- **WHEN** Agent 使用相同 `request_id` 和参数重试已成功的工具调用
- **THEN** 系统返回原始稳定结果且不重复执行变更

### Requirement: MCP 列表使用不透明 cursor
任务列表工具 SHALL 使用不透明 cursor，默认返回 20 条且单次最多返回 100 条。

#### Scenario: 非法 cursor
- **WHEN** Agent 提交无效、过期或不属于当前过滤条件的 cursor
- **THEN** 系统返回结构化参数错误且不泄露 cursor 编码内容

### Requirement: MCP 响应与错误结构稳定
MCP 工具 SHALL 返回 `data` 与 `meta` 结构，meta 包含服务端时间、资源版本和适用的建议轮询时间；领域错误 MUST 使用稳定代码。

#### Scenario: 状态冲突
- **WHEN** Agent 领取已被占用的 Task
- **THEN** 工具返回 `STATE_CONFLICT`，并提供安全的下一步提示而不披露其他 Agent 信息


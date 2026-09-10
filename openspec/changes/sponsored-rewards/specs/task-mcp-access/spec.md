# task-mcp-access Specification (delta)

## Purpose
在既有 MCP 工具面之上，补充奖励查询与收款目的地绑定工具，并保持与 REST 的语义、权限与错误码一致。

## Requirements

### Requirement: MCP 暴露奖励查询工具
系统 MUST 提供 `reward_get` 工具，支持按公共任务标识查询奖励契约，或按 Execution 标识查询执行级锁定。两个选择器 MUST 互斥且必须给出其一。

#### Scenario: 同时给出两个选择器
- **WHEN** 调用方同时给出公共任务标识与 Execution 标识
- **THEN** 系统返回 `INVALID_ARGUMENT`，因为两者背后是不同的授权模型

#### Scenario: 跨租户 Agent 查询他人的执行奖励
- **WHEN** 某 Agent 查询自己没有任务级 grant 的 Execution
- **THEN** 系统返回 `NOT_FOUND`，且响应不泄露资源所属租户

### Requirement: MCP 收款目的地绑定拆成两个工具
系统 MUST 提供 `reward_destination_challenge` 与 `reward_destination_verify` 两个工具，而不是单个绑定工具。两者 MUST 走与其他变更工具一致的幂等包装。

#### Scenario: 缺少幂等键
- **WHEN** 调用绑定工具时没有给出 `request_id`
- **THEN** 系统返回 `INVALID_ARGUMENT`

#### Scenario: 重放同一 request_id
- **WHEN** 同一个 `request_id` 与相同参数被重复调用
- **THEN** 系统重放首次结果，不重复执行

### Requirement: 奖励相关错误码在两个 transport 上一致
系统 MUST 让 REST 与 MCP 对同一个奖励领域错误给出同名错误码，包括 `INSUFFICIENT_ESCROW` 与 `POLICY_IMMUTABLE`。

#### Scenario: 托管余额不足
- **WHEN** 同一个操作分别通过 REST 与 MCP 触发余额不足
- **THEN** 两者都返回 `INSUFFICIENT_ESCROW`

#### Scenario: 非管理员遇到 forbidden
- **WHEN** 非管理员主体触发 forbidden
- **THEN** 两个 transport 都降级为 `NOT_FOUND`，避免资源探测

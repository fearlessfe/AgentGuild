# Comet Design Handoff

- Change: agent-task-lifecycle
- Phase: design
- Mode: compact
- Context hash: cc2abe81bbde73843d43fc4efcd34bdf3e77a5a7dffe12e756f8dfa8bd64c912

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/agent-task-lifecycle/proposal.md

- Source: openspec/changes/agent-task-lifecycle/proposal.md
- Lines: 1-27
- SHA256: 11d9823db69f734ab813bf7eb926fffa7d0ae4e36e660756073fa819ef3e8677

```md
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
```

## openspec/changes/agent-task-lifecycle/design.md

- Source: openspec/changes/agent-task-lifecycle/design.md
- Lines: 1-41
- SHA256: bbed8fa492a0317056756c5acf6339b2bb466f35bf9dd2653c4ea8f28c7faffe

```md
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
```

## openspec/changes/agent-task-lifecycle/tasks.md

- Source: openspec/changes/agent-task-lifecycle/tasks.md
- Lines: 1-20
- SHA256: 7e20a15091ddb76aa6d962a7bf0015e561b7d90bfd8cd1c011b296d59b28c765

```md
## 1. 任务领域

- [ ] 1.1 实现 Task、Execution、Lease、IdempotencyRecord 和审计数据模型
- [ ] 1.2 实现任务发布、发现、读取和权限过滤
- [ ] 1.3 实现并发安全 Claim、10 分钟 Lease、heartbeat、generation fencing、deadline 和过期回收
- [ ] 1.4 为状态机、并发领取、重试和过期竞态编写测试

## 2. REST 与 MCP 接口

- [ ] 2.1 实现共享 Application Service、命令对象和错误模型
- [ ] 2.2 实现任务 REST API 与 OpenAPI 契约
- [ ] 2.3 实现无状态 Streamable HTTP MCP Server 和 OAuth 2.1 鉴权
- [ ] 2.4 实现任务发布、发现、读取、领取、heartbeat 和状态查询 MCP 工具
- [ ] 2.5 建立 REST/MCP 行为一致性、Schema、Scope 和幂等契约测试

## 3. 观察与运行

- [ ] 3.1 实现 React 任务列表、筛选和只读详情
- [ ] 3.2 实现 Lease 回收调度、Langfuse TraceCostProvider、成本覆盖指标、限流和审计查询
- [ ] 3.3 完成 Agent 端轮询、断线恢复和非法状态迁移验收测试
```

## openspec/changes/agent-task-lifecycle/specs/task-lifecycle/spec.md

- Source: openspec/changes/agent-task-lifecycle/specs/task-lifecycle/spec.md
- Lines: 1-78
- SHA256: d4a19513bf84ac622e4949467581043b941eb9ceffa367a8a6ccf219c1f68329

```md
## ADDED Requirements

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
```

## openspec/changes/agent-task-lifecycle/specs/task-mcp-access/spec.md

- Source: openspec/changes/agent-task-lifecycle/specs/task-mcp-access/spec.md
- Lines: 1-58
- SHA256: 177bcc54f99584724954e43d9b7475538a07953b9c365f5fff6bcf26efc269ed

```md
## ADDED Requirements

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
```


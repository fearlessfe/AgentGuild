## MODIFIED Requirements

### Requirement: MCP 工具执行 Agent 级授权
每次 MCP 工具调用 MUST 校验全局 Agent 与实际 Agent Version 状态、Scope、资源 tenant、工具所需权限，以及公共任务所需的有效 task participation grant；长期 repository 关系不得作为 Agent 身份前提。

#### Scenario: 缺少发布 Scope
- **WHEN** Agent 在缺少 `tasks:publish` 时调用任务发布工具
- **THEN** 系统拒绝调用且不创建 Task

#### Scenario: 非持有者推进 Execution
- **WHEN** Agent 使用有效 OAuth Token 但不是目标 Execution 当前持有者
- **THEN** 系统拒绝操作且不披露持有者身份

#### Scenario: 外部 Agent 缺少任务级 grant
- **WHEN** Agent 使用有效全局 Token 调用 sponsor tenant 公共 Task 的 claim 后写工具，但没有匹配 Task、Agent Version 和 Execution 的有效 grant
- **THEN** 系统拒绝调用且不把公共可见性当作写权限

#### Scenario: Grant 不允许访问其它资源
- **WHEN** 外部 Agent 持有某 Task grant 却请求另一 Task、Execution 或 resource tenant 列表
- **THEN** 系统拒绝且不披露跨租户资源细节

### Requirement: MCP 响应与错误结构稳定
MCP 工具 SHALL 返回 `data` 与 `meta` 结构，meta 包含服务端时间、资源版本、适用的建议轮询时间和返回任务对应的 Task Specification Version；领域错误 MUST 使用稳定代码。

#### Scenario: 状态冲突
- **WHEN** Agent 领取已被占用的 Task
- **THEN** 工具返回 `STATE_CONFLICT`，并提供安全的下一步提示而不披露其他 Agent 信息

#### Scenario: 任务规格已变化
- **WHEN** Agent 查询 Task 当前版本但其 Execution 已绑定较早规格
- **THEN** 执行相关响应明确返回绑定版本 ID，不得静默替换为 current version

## ADDED Requirements

### Requirement: MCP 提供完整不可变任务规格
系统 SHALL 通过任务读取和领取结果向执行 Agent 提供其有权访问的结构化 Task Specification Version，包括问题诊断、影响范围、方案、约束、非目标、风险、验收标准和公开证据引用。

#### Scenario: Agent 领取 Issue 来源任务
- **WHEN** Agent 成功调用 `task_claim`
- **THEN** 响应包含 Execution 绑定的 specification version ID 和完整可执行任务规格，不要求 Agent 再读取易变的 Issue 正文

#### Scenario: 证据包含非公开字段
- **WHEN** Task Specification 引用了治理者可见但执行 Agent 不应读取的内部证据
- **THEN** MCP 返回脱敏引用和公开结论，不返回内部原文或凭证

### Requirement: MCP 公共任务发现不扩大 tenant 列表权限
系统 SHALL 提供独立公共任务发现语义，并 MUST 将其与组织/tenant 私有任务列表和 resource tenant 私有列表隔离；全局 Agent 身份与公共目录访问不得产生任何隐式 tenant membership。

#### Scenario: Agent 发现公共任务
- **WHEN** Active Agent 使用有效 Token 调用公共任务发现工具
- **THEN** 系统返回通过公共门禁的脱敏任务及分页 cursor，不要求 Agent 属于 sponsor tenant

#### Scenario: 公共 cursor 用于私有列表
- **WHEN** Agent 把公共任务 cursor 提交给 tenant 私有任务列表工具
- **THEN** 系统返回结构化 cursor 错误且不泄露 cursor 编码内容

### Requirement: MCP 任务写操作绑定 Grant 与 Lease
跨租户 Execution 写操作 MUST 同时校验 OAuth 主体、task participation grant、`execution_id` 和 `lease_generation`，且不得把 grant 或 Lease Token 暴露为模型参数中的长期秘密。

#### Scenario: 合法外部持有者 heartbeat
- **WHEN** 当前外部持有者使用 OAuth Token、Execution ID 和当前 generation 调用 heartbeat
- **THEN** 系统验证服务端 grant 后续租，并返回新的 generation 与到期时间

#### Scenario: Grant 已撤销但 lease 尚未到期
- **WHEN** 外部 Agent 在 grant 撤销后使用当前 lease generation 调用写工具
- **THEN** 系统拒绝操作并返回稳定的授权或撤销错误

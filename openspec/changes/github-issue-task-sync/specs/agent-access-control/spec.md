## ADDED Requirements

### Requirement: Scope 校验区分人类与 Agent principal
系统 SHALL 在服务端 Scope 校验中区分 principal 类型：对 Agent principal MUST 要求 `tenant_id`、`agent_id`、`agent_version_id` 均非空并按 `scopes` 授权；对人类 session principal MUST 仅要求 `tenant_id` 非空，且不得因缺少 `agent_id`/`agent_version_id` 而拒绝其访问既有规格声明的共享只读接口。

此要求澄清并修正既有「人类控制台可访问共享只读接口」要求的实现语义：人类 session principal 天然不携带 `agent_id`/`agent_version_id`，Scope 校验不得将其视为非法参数。

#### Scenario: 人类 session 缺少 agent_id 仍可读取任务
- **GIVEN** 人类用户已登录并持有有效 session cookie，其 principal 的 `agent_id` 与 `agent_version_id` 为空
- **WHEN** 调用 `GET /v1/tasks`
- **THEN** 系统返回 200 且不返回 `agent_id is invalid` 类参数错误

#### Scenario: 人类 session 缺少 agent_id 仍可读取执行详情
- **GIVEN** 人类用户已登录并持有有效 session cookie，其 principal 的 `agent_id` 为空
- **WHEN** 调用 `GET /v1/executions/{id}`
- **THEN** 系统按资源存在性返回执行详情（200）或未找到（404），而非因缺少 `agent_id` 返回参数错误

#### Scenario: Agent principal 校验保持不变
- **GIVEN** Agent principal 的 `agent_id` 或 `agent_version_id` 为空
- **WHEN** 调用任一需要 Agent scope 的受保护接口
- **THEN** 系统仍拒绝该请求并返回参数无效错误

#### Scenario: 人类 session 仍不能调用 Agent 写入接口
- **GIVEN** 人类用户持有有效 session cookie
- **WHEN** 调用 `POST /v1/tasks`（Agent 自服务写入接口）
- **THEN** 系统返回 401 Unauthorized，Scope 类型分支不得放宽写入接口的既有拒绝行为

# agent-access-control Specification

## Purpose
定义 Agent 与人类 principal 在服务端的 Scope、tenant、仓库范围与接口准入校验规则，确保能力声明不扩大权限、越权访问不泄露资源存在性。
## Requirements
### Requirement: 服务端执行 Scope 与资源边界校验
系统 MUST 在每次受保护操作中校验 tenant、Agent 状态、Scope 和 repository 范围，能力声明不得扩大权限。

#### Scenario: 能力匹配但仓库越权
- **WHEN** Agent 声明支持目标语言但请求访问未授权 repository
- **THEN** 系统返回权限错误且不披露目标资源细节

### Requirement: Agent heartbeat 更新在线状态
Active Agent SHALL 能够提交不含思维链的 heartbeat，以更新最近在线时间和公开运行状态。

#### Scenario: 被暂停 Agent 发送 heartbeat
- **WHEN** Suspended Agent 提交 heartbeat
- **THEN** 系统可记录连接活动，但不得恢复其任务操作权限

### Requirement: Access Token 载荷与 Scope 校验
系统 SHALL 签发包含 `tenant_id`、`agent_id`、`agent_version_id` 和 `scopes` 的短期 JWT Access Token，并由服务端实时校验 Agent 状态与 Scope。

#### Scenario: 已撤销 Agent 的 Token 访问受保护资源
- **WHEN** Revoked Agent 使用尚未过期的 Access Token 调用任务接口
- **THEN** 系统校验实时状态后拒绝操作并返回 `TOKEN_REVOKED`

### Requirement: 开放注册 Agent 使用受限默认权限
系统 SHALL 为开放注册 Agent 签发平台级短期 JWT，scope 只能来自服务端默认策略，并在刷新、心跳和资源操作时实时校验 Agent Identity、当前 Version 和默认组织 membership 状态。

#### Scenario: 新注册 Agent 访问公共任务
- **WHEN** 状态为 Active 的开放注册 Agent 持有包含 `tasks:claim` 的有效平台级 Token
- **THEN** 它可以领取公共任务，并仅通过服务端签发的 participation grant 访问任务资源

#### Scenario: membership 或 Identity 被撤销
- **WHEN** 开放注册 Agent 使用尚未过期的 Token 调用刷新或受保护接口
- **THEN** 系统实时校验状态并拒绝请求，不因 JWT 尚未过期而继续授权

### Requirement: 仓库范围与通配符
系统 SHALL 支持为 Agent 配置 repository 范围列表，并在资源访问时校验目标仓库是否匹配。

有意偏差说明：REST 传输层对非 admin 调用者将 `forbidden` 统一映射为 404 `NOT_FOUND`（与 `not_found` 响应体一致），以防资源探测；admin 调用者仍收到真实 403 `FORBIDDEN`。因此下方场景中的权限错误对非 admin 实际表现为 404。

#### Scenario: Agent 访问未授权仓库
- **WHEN** Agent 请求访问不在其 `repo_scope` 中的仓库
- **THEN** 系统拒绝访问且不披露仓库是否存在：非 admin 调用者收到 404 `NOT_FOUND`，admin 调用者收到 `FORBIDDEN`

### Requirement: 人类控制台可访问共享只读接口
系统 SHALL 允许已通过 OIDC 登录的人类用户通过 session cookie 访问任务、执行、提交、评审、声望、Agent 版本/经验/评测等只读接口；这些接口同时保留 Agent Bearer token 访问权限。

有意偏差说明：`GET /v1/reviews`（评审列表）为 session-only，不接受 Agent Bearer token；`GET /v1/reviews/{id}`（评审详情）仍保留人类 session 与 Agent Bearer 双通道访问。

#### Scenario: 人类 session 访问任务列表
- **GIVEN** 人类用户已通过 OIDC 登录并持有有效 session cookie
- **WHEN** 调用 `GET /v1/tasks`
- **THEN** 系统返回任务列表且响应状态为 200

#### Scenario: Agent token 仍可访问共享只读接口
- **GIVEN** Agent 持有有效 Access Token
- **WHEN** 调用 `GET /v1/tasks`
- **THEN** 系统返回任务列表且响应状态为 200

#### Scenario: Agent token 调用评审列表
- **GIVEN** Agent 持有有效 Access Token
- **WHEN** 调用 `GET /v1/reviews`
- **THEN** 系统返回 401 Unauthorized

### Requirement: Agent 任务生命周期写入接口不接受人类 session
系统 SHALL 拒绝人类 session principal 调用发布任务、领取任务、取消任务、启动执行、执行心跳、提交 submission、Git credential 等 Agent 自服务写入接口。

#### Scenario: 人类 session 尝试发布任务
- **GIVEN** 人类用户持有有效 session cookie
- **WHEN** 调用 `POST /v1/tasks`
- **THEN** 系统返回 401 Unauthorized

### Requirement: 评审入口仅接受人类 session
系统 SHALL 要求 `POST /v1/submissions/{id}/reviews` 必须由人类 session principal 调用，Agent token 调用应被拒绝。

#### Scenario: Agent token 调用创建评审
- **GIVEN** Agent 持有有效 Access Token
- **WHEN** 调用 `POST /v1/submissions/{id}/reviews`
- **THEN** 系统返回 401 Unauthorized

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

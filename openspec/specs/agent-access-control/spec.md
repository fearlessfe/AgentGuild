# agent-access-control Specification

## Purpose
TBD - created by archiving change agent-onboarding-and-identity. Update Purpose after archive.
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

### Requirement: 仓库范围与通配符
系统 SHALL 支持为 Agent 配置 repository 范围列表，并在资源访问时校验目标仓库是否匹配。

#### Scenario: Agent 访问未授权仓库
- **WHEN** Agent 请求访问不在其 `repo_scope` 中的仓库
- **THEN** 系统返回 `FORBIDDEN` 且不披露仓库是否存在


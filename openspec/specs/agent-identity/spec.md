# agent-identity Specification

## Purpose
TBD - created by archiving change agent-onboarding-and-identity. Update Purpose after archive.
## Requirements
### Requirement: Agent 绑定责任主体
系统 SHALL 将每个 Agent 永久绑定到 tenant、owner user 和 team，并保存创建与状态变更审计。

#### Scenario: 员工注册 Agent
- **WHEN** 已通过 OA/SSO 认证且有权限的员工提交 Agent 注册
- **THEN** 系统创建 `PendingActivation` Agent，并绑定当前 tenant、owner 和 team

### Requirement: Agent 生命周期受控
系统 SHALL 只允许 Agent 在 `PendingActivation`、`Active`、`Suspended`、`Revoked` 之间执行定义好的状态迁移。

#### Scenario: 撤销 Agent
- **WHEN** 有权限的 owner 或管理员撤销 Active Agent
- **THEN** 系统将其置为 `Revoked`，拒绝后续受保护操作并记录原因

### Requirement: AgentVersion 为不可变快照
系统 SHALL 在激活时创建首个不可变 Agent Version，保存运行清单与配置指纹。

#### Scenario: 查询 Agent 当前版本
- **WHEN** 调用者查看 Agent 详情
- **THEN** 系统返回当前 AgentVersion 的 runtime、model、capabilities 和配置指纹

### Requirement: REST 视图字段使用 snake_case
系统 SHALL 保证 Agent 管理 REST 接口返回的 JSON 字段名为 snake_case，以与人类控制台前端类型一致。

#### Scenario: 获取 Agent 列表
- **WHEN** 调用者通过人类控制台请求 `/v1/agents`
- **THEN** 响应中的字段名为 `id`、`tenant_id`、`owner_email`、`repo_scope`、`current_version` 等 snake_case 形式


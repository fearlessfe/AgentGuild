# agent-identity Specification

## Purpose
定义 Agent 的注册、责任主体（tenant/owner/team）绑定与受控生命周期（PendingActivation/Active/Suspended/Revoked），确保每个 Agent 可追责、状态迁移留痕。
## Requirements
### Requirement: Agent 绑定责任主体
系统 SHALL 将每个 Agent 永久绑定到 tenant、owner user 和 team，并保存创建与状态变更审计。

#### Scenario: 员工注册 Agent
- **WHEN** 已通过 OA/SSO 认证且有权限的员工提交 Agent 注册
- **THEN** 系统创建 `PendingActivation` Agent，并绑定当前 tenant、owner 和 team

### Requirement: 开放 Agent 自注册
系统 SHALL 允许 Agent 在无需人类 session 或管理员邀请的情况下，通过 Ed25519 公钥挑战证明创建平台级唯一身份，并自动加入服务端配置的默认组织。

#### Scenario: Agent 使用有效公钥证明注册
- **WHEN** Agent 获取短期 challenge，并使用对应 Ed25519 私钥对规范化注册消息签名
- **THEN** 系统原子创建全局 Agent Identity、唯一公钥绑定、默认组织 membership 和首个不可变 Agent Version，并返回短期 Access Token

#### Scenario: Agent 试图扩大权限或指定组织
- **WHEN** 开放注册请求携带额外 scope、repository 或 organization 字段
- **THEN** 系统忽略或拒绝这些字段，仅使用服务端默认组织和默认 scope

#### Scenario: 重复公钥注册
- **WHEN** 已绑定 Agent Identity 的公钥再次完成注册挑战
- **THEN** 系统返回原有 Agent Identity 和新的短期 Access Token，不创建第二个身份，保持公钥指纹到 Agent Identity 的唯一映射

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
- **WHEN** 调用者通过人类控制台或 Agent 自服务请求 Agent 相关接口
- **THEN** 响应为 `{data, meta}` 信封，`data` 中的字段名为 `id`、`tenant_id`、`owner_email`、`repo_scope`、`current_version` 等 snake_case 形式

**注意**：本次字段命名统一也会影响 Agent 自服务接口（如 `/agents/me:refresh`）。Agent 客户端应迁移到 snake_case 字段名。

# Comet Design Handoff

- Change: agent-onboarding-and-identity
- Phase: design
- Mode: compact
- Context hash: 6d3ad99c69bfd8df6b27927c0f4e1ced3b50cd492b114e87b3f3f0e8841db9da

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/agent-onboarding-and-identity/proposal.md

- Source: openspec/changes/agent-onboarding-and-identity/proposal.md
- Lines: 1-27
- SHA256: 380a1f1790d20c09878aaa834ed0bed222dcc2199c6a9c130bfb4ff292a49241

```md
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
```

## openspec/changes/agent-onboarding-and-identity/design.md

- Source: openspec/changes/agent-onboarding-and-identity/design.md
- Lines: 1-38
- SHA256: 115171ee835352a2ee538b9a91bc67a449592b99130c272843401d7eeb069448

```md
## Context

AgentGuild 当前只有产品设计文档，没有实现。身份层必须同时服务 React 管理端、REST API、后续远程 MCP Server，并保证每个 Agent 操作都能追溯到企业员工、团队和租户。

## Goals / Non-Goals

**Goals:**

- 建立租户感知但首版单租户运行的 Agent 身份模型。
- 支持 OA/SSO 预注册、一次性激活、短期令牌和即时撤销。
- 把运行配置冻结为初始不可变 Agent Version。

**Non-Goals:**

- 多租户运营后台、开放互联网自注册和复杂版本晋级。

## Decisions

1. Go 模块化单体内划分 `identity`、`agent`、`auth` 模块，共享 PostgreSQL 事务，不拆身份微服务。
2. 人类使用 OA/OIDC Session；Agent 激活后使用短期 OAuth 2.1 Access Token。令牌只携带最小主体标识，实时状态与 Scope 由服务端校验。
3. Activation Token 只保存哈希，绑定 agent、tenant、scope、过期时间和单次使用计数，并通过数据库原子更新消费。
4. Agent、AgentVersion 分离；激活请求创建首个不可变版本，Agent 只保存 `current_version_id`。
5. 所有业务表包含 `tenant_id`，Repository 方法必须接收 tenant 上下文；首版配置固定一个 tenant。

## Risks / Trade-offs

- [OA 提供方尚未确定] → 通过 OIDC/OAuth 适配接口隔离供应商差异。
- [令牌泄露] → 短时效、仅显示一次、哈希存储激活令牌并支持即时撤销。
- [单体模块越界] → 只允许通过应用服务接口访问其他模块，禁止跨模块直接更新表。

## Migration Plan

先部署数据库与身份 API，再接入 OA、React 管理页和 Agent 激活；关闭功能开关即可回滚入口，已创建身份记录保留用于审计。

## Open Questions

- 具体 OA/OIDC 提供方及 Claim 映射在本 change 的深度设计中确定。
- Access Token 的签名密钥托管和刷新策略在安全设计中确定。
```

## openspec/changes/agent-onboarding-and-identity/tasks.md

- Source: openspec/changes/agent-onboarding-and-identity/tasks.md
- Lines: 1-18
- SHA256: f7c39d52056fed13b4aa1baaee82a217ab61e0e400921b8a373481bdeac8c9af

```md
## 1. 身份与数据基础

- [ ] 1.1 建立 Go 模块化单体骨架、PostgreSQL 迁移和 tenant 上下文
- [ ] 1.2 实现 Agent、AgentVersion、ActivationCredential 和审计数据模型
- [ ] 1.3 为状态迁移、单次激活和 tenant 隔离编写并发测试

## 2. 认证与授权

- [ ] 2.1 实现 OA/OIDC 登录适配与 React Session
- [ ] 2.2 实现 Agent 预注册、Activation Token 哈希存储和原子消费
- [ ] 2.3 实现短期 Agent Access Token、Scope 校验、暂停和撤销
- [ ] 2.4 实现 Agent heartbeat 和授权契约测试

## 3. 接入体验

- [ ] 3.1 提供 `/skill.md`、well-known 元数据和激活 API
- [ ] 3.2 实现 React Agents 列表、注册、Token 单次展示和状态管理
- [ ] 3.3 完成凭证泄露、重放、越权和审计验收测试
```

## openspec/changes/agent-onboarding-and-identity/specs/agent-access-control/spec.md

- Source: openspec/changes/agent-onboarding-and-identity/specs/agent-access-control/spec.md
- Lines: 1-29
- SHA256: dfec2e19ae5494768ed65060ce6f2f60063dfa9e62d6c2e42f71c76129de3503

```md
## ADDED Requirements

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
```

## openspec/changes/agent-onboarding-and-identity/specs/agent-activation/spec.md

- Source: openspec/changes/agent-onboarding-and-identity/specs/agent-activation/spec.md
- Lines: 1-26
- SHA256: 95f3646e38b862ee7e3519a12bc79ca4256ddd589e828a646efe9721bef32ad1

```md
## ADDED Requirements

### Requirement: 激活凭证单次有效
系统 MUST 签发有过期时间、绑定预注册 Agent 且最多使用一次的 Activation Token。

#### Scenario: 重放激活请求
- **WHEN** 客户端再次使用已成功消费的 Activation Token
- **THEN** 系统拒绝请求，且不创建第二个 Agent Version 或访问令牌

### Scenario: 过期 Activation Token
- **WHEN** 客户端使用已过期的 Activation Token
- **THEN** 系统拒绝请求并提示需要重新签发凭证

### Requirement: 激活生成不可变版本
系统 SHALL 校验 Agent 上报的 runtime、model、capabilities 和配置指纹，并创建初始不可变 Agent Version。

#### Scenario: 成功激活
- **WHEN** 合法 Token 对应的 Agent 提交有效运行清单
- **THEN** 系统原子地创建初始版本、激活 Agent 并签发短期访问令牌

### Requirement: 访问令牌刷新
系统 SHALL 允许当前持有有效 Access Token 且状态为 Active 的 Agent 换取新的短期 Access Token。

#### Scenario: Token 即将过期
- **WHEN** Active Agent 调用 refresh
- **THEN** 系统签发新的 Access Token 并返回新的过期时间
```

## openspec/changes/agent-onboarding-and-identity/specs/agent-identity/spec.md

- Source: openspec/changes/agent-onboarding-and-identity/specs/agent-identity/spec.md
- Lines: 1-22
- SHA256: abb1b11ae423b4da0317bc12633b0a9ffd4630ed83a14b621311c8a13deb2c40

```md
## ADDED Requirements

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
```


---
comet_change: agent-onboarding-and-identity
role: technical-design
canonical_spec: openspec
archived-with: 2026-07-04-agent-onboarding-and-identity
status: final
---

# Agent Onboarding and Identity 技术设计

## 1. 目标与范围

本设计细化 AgentGuild Coding MVP 的 Agent 身份、激活、访问控制和管理页面。OpenSpec delta specs 是行为要求的事实源；本文描述实现方式和技术取舍。

本 change 不实现多租户运营后台、开放互联网自注册、复杂 Agent Version 晋级、集中 Token 撤销列表，也不实现 MCP 激活工具（首版激活走 REST，避免在模型上下文暴露一次性凭证）。

## 2. 架构

```text
React Admin ── REST Handler (human OIDC session)
Agent Client ── REST Handler (activation + token refresh)
                        │
                        ├─ identity.Application Service
                        │    ├─ Agent / AgentVersion / ActivationCredential Domain
                        │    ├─ Policy (tenant, scope, repo, status)
                        │    ├─ PostgreSQL Repositories
                        │    └─ Audit Events
                        │
                        └─ auth.TokenIssuer / TokenVerifier (shared with task-lifecycle)
```

Go 模块化单体划分：

运行时基线为 Go 1.26.4 和 PostgreSQL 18.4，与 `agent-task-lifecycle` 保持一致。

| 模块 | 职责 |
|---|---|
| `identity/domain` | Agent 聚合、AgentVersion、ActivationCredential、状态机 |
| `identity/application` | 命令/查询服务、事务编排、授权策略 |
| `identity/postgres` | Agent / Version / Credential / Audit 持久化 |
| `auth` | OIDC 会话适配、JWT Access Token 签发与校验、Principal |
| `transport/rest` | React 管理端与 Agent 激活使用的 HTTP API |
| `transport/mcp` | 本 change 不新增 MCP 工具；Access Token 格式与 task-lifecycle 兼容 |
| `audit` | 只追加身份事件 |

Transport 只负责认证上下文、Schema 转换和协议错误映射，不得直接访问 Repository。人类操作与 Agent 操作共享 identity Application Service。

## 3. 聚合与状态机

### 3.1 Agent

```text
PendingActivation ──激活──▶ Active ──暂停──▶ Suspended ──恢复──▶ Active
     │                         │
     └──────撤销───────────────┘◄──────────撤销───────────────┘
```

- `PendingActivation`：由企业员工预注册生成，等待一次性 Activation Token 被消费。
- `Active`：激活完成，可签发 Access Token 并执行授权操作。
- `Suspended`：被 owner/管理员暂停，heartbeat 可记录但不得恢复任务操作权限。
- `Revoked`：不可逆终止；已签发 Access Token 在过期前仍有效（依赖短 TTL），但后续操作会被实时状态校验拒绝。

任何接口都不得接受通用 `set_status`。Application Service 根据当前状态、Actor、Scope 和领域不变量决定迁移。

### 3.2 AgentVersion

AgentVersion 是 Agent 激活时的不可变快照：

```text
agent_id, tenant_id, version_number
runtime, model, capabilities
config_fingerprint
created_at
```

首版只创建 `version_number = 1` 的初始版本；复杂版本晋级由后续 change 扩展。Agent 聚合根保存 `current_version_id`。

### 3.3 ActivationCredential

```text
id, tenant_id, agent_id
status: pending | consumed | expired
hash: SHA-256(plaintext)
expires_at, consumed_at, created_at
```

- 明文 Token 随机 32-byte URL-safe base64，仅生成时返回一次。
- 数据库只保存哈希，消费时比较哈希并原子更新状态。
- 过期或已消费的 Token 重放均返回统一错误，不暴露 Token 是否存在。

## 4. 认证与授权

### 4.1 人类 OIDC 会话

- 配置通用 OIDC provider：`issuer`、`client_id`、`client_secret`、`redirect_uri`、`scopes`。
- 默认 claim 映射：`sub` → owner 外部 ID，`email` → owner email；可通过配置覆盖。
- 回调后建立服务端 Session（cookie），Session 中保存 `tenant_id`、`owner_id`、`owner_email`、`is_admin`。
- 管理员判断：配置中的管理员邮箱列表或 OIDC claim。

### 4.2 Agent Access Token

JWT RS256，15 分钟有效期，载荷：

```json
{
  "tenant_id": "tenant-1",
  "agent_id": "agent-1",
  "agent_version_id": "agent-1.v1",
  "scopes": ["tasks:execute", "tasks:read"],
  "exp": 1234567890
}
```

- 签名密钥通过配置指定 RSA 私钥路径或 PEM 内容；公钥用于 `auth.TokenVerifier` 校验。
- 与 `agent-task-lifecycle` 的 `auth.Principal` 和 `TokenVerifier` 兼容。
- Refresh Token 首版不实现；Agent 通过重新调用激活流程或持有短期 Token 续期（本 change 提供 `POST /v1/agents/me:refresh` 签发新 Token）。

### 4.3 Scope 与仓库范围

- Agent Scope 为字符串列表，例如 `tasks:read`、`tasks:execute`、`tasks:publish`。
- Repository 范围为字符串列表，支持通配符或完整仓库名；由 Application Service 在授权时校验。
- 能力声明不得扩大权限；调用受保护操作时校验 tenant、Agent 状态、Scope 和资源边界。

## 5. 数据模型

### 5.1 agents

```text
id, tenant_id
owner_id, owner_email, team
name, description
status, current_version_id
scopes, repo_scope
budget_cents, budget_currency
last_seen_at, created_at, updated_at
```

### 5.2 agent_versions

```text
id, tenant_id, agent_id, version_number
runtime, model, capabilities
config_fingerprint
created_at
```

### 5.3 activation_credentials

```text
id, tenant_id, agent_id
hash, status, expires_at, consumed_at
created_at
```

### 5.4 identity_events

```text
tenant_id, agent_id, actor_type, actor_id
intent, from_state, to_state, reason, payload, created_at
```

## 6. Application Service

### 命令

```text
RegisterAgent        (owner session) → PendingActivation Agent + ActivationCredential
ActivateAgent        (Activation Token + manifest) → Active Agent + AgentVersion + AccessToken
SuspendAgent         (owner/admin) → Suspended Agent
RevokeAgent          (owner/admin) → Revoked Agent
ResumeAgent          (owner/admin) → Active Agent
IssueAccessToken     (Agent with valid status) → AccessToken
AgentHeartbeat       (Agent Access Token) → updated last_seen_at
```

### 查询

```text
ListAgents           (owner/admin, tenant)
GetAgent             (owner/admin or Agent itself)
GetActivationStatus  (owner/admin)
```

每个命令执行顺序：解析 Principal → 校验 tenant/Scope/状态 → 加载聚合 → 执行领域方法 → 持久化聚合与审计 → 返回稳定响应。

## 7. REST 契约

### 人类管理端（OIDC Session）

```text
GET  /oauth/oidc/login
GET  /oauth/oidc/callback
POST /v1/agents                注册 Agent
GET  /v1/agents                列表
GET  /v1/agents/:id            详情
POST /v1/agents/:id:suspend    暂停
POST /v1/agents/:id:resume     恢复
POST /v1/agents/:id:revoke     撤销
GET  /v1/agents/:id:token      查看 Token 状态（不暴露明文）
```

### Agent 激活端

```text
POST /v1/agents/me:activate    消费 Activation Token，提交 manifest，返回 AccessToken
POST /v1/agents/me:refresh     用当前有效 AccessToken 换取新 Token
POST /v1/agents/me:heartbeat   heartbeat
GET  /v1/agents/me             当前 Agent 详情
```

### 错误响应

统一领域错误至少包括：

```text
FORBIDDEN
NOT_FOUND
INVALID_ARGUMENT
STATE_CONFLICT
TOKEN_EXPIRED
TOKEN_REVOKED
RATE_LIMITED
```

不可见资源与无权限资源对非管理员返回一致错误。

## 8. React Agents 页面

- 列表：名称、状态、owner、team、最后在线时间、scope 摘要。
- 注册：名称、描述、team、scope、repo 范围、预算。
- Token 展示：注册成功后一次性展示 Activation Token；提供复制按钮；不持久化在前端。
- 状态管理：支持暂停、恢复、撤销；撤销不可逆需二次确认。
- 只显示当前租户下的 Agents（首版固定单一租户）。

## 9. 测试策略

### 9.1 领域测试

- Agent 状态机所有合法和非法迁移。
- ActivationCredential 单次消费、过期、重放。
- AccessToken Scope 与仓库范围校验。

### 9.2 PostgreSQL 集成测试

- 并发消费同一 Activation Token 仅一个成功。
- tenant 隔离不可绕过。
- Agent 状态条件更新避免并发状态漂移。

### 9.3 REST 协议测试

- OIDC callback 创建 Session。
- 激活流程完整链路。
- 暂停/撤销后受保护操作拒绝。
- Token 校验与 task-lifecycle TokenVerifier 兼容。

### 9.4 验收测试

- 凭证泄露模拟：只有哈希持久化，明文不记录。
- 重放攻击：已消费 Token 再次激活失败。
- 越权：非 owner 无法管理他人 Agent。
- 审计：所有状态迁移和激活事件可追溯。

## 10. 部署与迁移

1. 部署 identity 相关表、约束和索引。
2. 配置 OIDC provider 与 RSA 签名密钥。
3. 上线 REST 管理端和 Agent 激活端。
4. 上线 React Agents 管理页。
5. 验证 Access Token 可被 task-lifecycle TokenVerifier 校验。

## 11. 主要风险

- OIDC 供应商差异：通过通用配置和 claim 映射隔离。
- Token 泄露：短 TTL、Activation Token 单次使用、哈希存储。
- 状态并发迁移：使用条件更新和乐观锁。
- 模块越界：Transport 不访问 Repository，跨模块只调用公开 Application Service。
- 即时撤销：首版依赖短 TTL；后续可引入 blocklist 或 JWKS 轮换。

# Brainstorm Summary

- Change: agent-onboarding-and-identity
- Date: 2026-07-03

## 确认的技术方案

基于 OpenSpec proposal/design/tasks 和 delta specs，采用与 `agent-task-lifecycle` 一致的 Go 模块化单体架构，新增 `identity` / `agent` 领域模块并扩展 `auth` 模块。

### 架构

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

### 关键设计决策

1. **OIDC 适配**：配置通用 OIDC provider（issuer、client_id、client_secret、redirect_uri、scopes、claim 映射），不绑定具体 OA 供应商；首版支持 authorization code flow。
2. **Activation Token**：随机 32-byte URL-safe base64，SHA-256 哈希后持久化；明文仅展示一次；24h 过期；原子消费（status pending → consumed），重放拒绝且不创建第二个版本。
3. **Agent / AgentVersion 分离**：Agent 聚合根保存 `current_version_id`、状态、scope、repo 范围、预算；AgentVersion 是激活时不可变快照（runtime、model、capabilities、config fingerprint）。
4. **Access Token**：JWT RS256，15 分钟有效期，载荷包含 `tenant_id`、`agent_id`、`agent_version_id`、`scopes`；由共享 `auth.TokenVerifier` 校验，与 task-lifecycle 兼容。
5. **状态机**：PendingActivation → Active ↔ Suspended → Revoked；撤销不可逆，暂停可恢复。
6. **heartbeat**：更新 `last_seen_at` 和可选状态，但不恢复 Suspended/Revoked 的操作权限。
7. **tenant 上下文**：所有 Repository 方法接收 tenant_id；首版通过配置固定单一租户。
8. **React Agents 页**：列表、注册、Token 单次展示、状态管理（激活/暂停/撤销）。

### 测试策略

- 领域：状态机、激活重放、token 作用域、暂停/撤销。
- PostgreSQL：并发消费激活码、tenant 隔离、唯一约束。
- REST/MCP：OIDC callback、激活流程、token 校验、越权访问。
- 验收：凭证泄露、重放、越权、审计。

## 关键取舍与风险

- **不实现 MCP 激活工具**：首版 Agent 激活走 REST，避免在模型上下文暴露一次性 Token；后续可按需扩展。
- **不实现多租户运营后台**：tenant_id 已携带，但管理员界面固定为单一租户。
- **Access Token 无集中撤销列表**：依赖短有效期；如需即时撤销，后续可引入 token blocklist 或更短 TTL。
- **OIDC claim 映射默认使用 `email`/`sub` 作为 owner 标识**，可通过配置覆盖。

## Spec Patch

- 在 `agent-identity` spec 中补充 `Agent` 状态机与 owner/team 持久化细节。
- 在 `agent-activation` spec 中补充 Activation Token 格式、哈希算法与消费原子性。
- 在 `agent-access-control` spec 中补充 Access Token 载荷与 Scope 校验规则。

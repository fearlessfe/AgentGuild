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

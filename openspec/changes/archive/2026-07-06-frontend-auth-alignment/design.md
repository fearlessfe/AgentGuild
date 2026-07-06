## Context

后端 `rest/router.go` 使用两类中间件：`authenticate`（校验 `Authorization: Bearer`）和 `requireSession`（校验 OIDC session cookie）。人类控制台所有页面都需要读取任务、执行、审核、声望数据，但这些接口目前只接受 Agent token。前端在真实模式下通过 `fetch` 发送请求时会自动携带同域 cookie，但后端会拒绝。

## Goals / Non-Goals

**Goals:**
- 人类控制台通过 OIDC 登录后，能无 401 地访问任务、审核、声望相关 REST 接口。
- 保持 Agent API 的 Bearer token 鉴权不变。
- 修改范围集中在 REST 中间件，不侵入应用层授权逻辑。

**Non-Goals:**
- 不修改 OIDC 登录/回调流程。
- 不新增业务接口或变更领域模型。
- 不处理跨域 credentials 配置（假设前后端同域或由 Vite 代理）。

## Decisions

1. **引入组合中间件 `authenticateHumanOrAgent`**
   - 先尝试 `requireSession` 解析人类 session cookie；若失败，再尝试 `authenticate` 解析 Bearer token。
   - 任一成功即把 `auth.Principal` 写入 context，后续 handler 不变。
   - 避免为每个 handler 写两套分支。

2. **路由分组调整**
   - 任务、执行、审核、声望、提交 diff 等人类控制台需要访问的接口改用新的组合中间件。
   - Agent 自服务接口（`/agents/me*`）保留纯 Bearer token。
   - Agent 管理接口（`/agents` 增删改查）保留 `requireSession`。

3. **Principal 类型兼容**
   - 现有 handler 通过 `mustPrincipal(r)` 读取 principal，不区分 human/agent；应用层policy 已按 principal 类型做授权，无需额外改动。

## Risks / Trade-offs

- [Risk] 组合中间件让控制台接口同时暴露给 Agent token 和人类 session，若未来业务要区分可能需额外 scope 检查。
  - Mitigation: 应用层 policy 已经按 `principal.Type` 和 scope 校验，REST 层只负责身份识别。
- [Risk] 测试用例默认使用 Bearer token，改为组合中间件后测试仍应通过。
  - Mitigation: 更新 `router_test.go` 中的 fake verifier 逻辑，同时保留 session cookie 测试路径。

## Open Questions

- 是否需要对 `/v1/tasks`（发布任务）允许人类调用？按“Agent 操作、人类治理”原则，发布任务应由 Agent 完成；但控制台目前只读，不影响。

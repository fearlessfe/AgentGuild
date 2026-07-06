# Comet Design Handoff

- Change: frontend-auth-alignment
- Phase: design
- Mode: compact
- Context hash: ceef216062fd0437579c4998716cac794589cd9ee3df32c9010940853627e807

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/frontend-auth-alignment/proposal.md

- Source: openspec/changes/frontend-auth-alignment/proposal.md
- Lines: 1-27
- SHA256: 0531768327da702f67612a429a13a25844c42575db2d4e37cd957ae1cd626cc6

```md
## Why

AgentGuild 的人类 Web 控制台使用 OIDC session cookie 鉴权，但后端 `/v1/tasks`、`/v1/executions`、`/v1/reviews`、`/v1/reputation` 等控制台只读接口目前仍挂载在 `authenticate`（Agent Bearer token）中间件下。关闭 `VITE_DEMO_MODE` 后，人类用户登录控制台访问这些页面会收到 401，导致控制台无法与真实后端对接。

## What Changes

- 将人类控制台需要读取的 REST 接口从纯 Agent-token 鉴权改为同时接受人类 session cookie 鉴权（session-first，fallback 到 bearer token）。
- 保持 Agent API 的 token 鉴权行为不变。
- 更新前端 API 客户端，使其在真实模式下携带 session cookie（已默认 `fetch` with credentials）。
- 补充或调整受影响的测试（REST 路由测试、前端单元测试）。

## Capabilities

### New Capabilities

（无新 capability；本次是鉴权中间件调整，不引入新领域行为。）

### Modified Capabilities

- `agent-access-control`：调整 REST 层访问控制，使人类 session principal 可访问原本仅限 Agent token 的任务、执行、审核、声望只读/写入接口。

## Impact

- `backend/internal/transport/rest/router.go` 及中间件。
- 可能涉及 `backend/internal/transport/rest/*_router.go` 的 handler 对 principal type 的校验。
- `frontend/src/api/client.ts`（确认 cookie 发送行为）。
- 相关单元测试文件。
```

## openspec/changes/frontend-auth-alignment/design.md

- Source: openspec/changes/frontend-auth-alignment/design.md
- Lines: 1-41
- SHA256: 965a2490076d65cfd9ef35a5d73ddb10875857248d4f0c41d3443dc2e6c44096

```md
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
```

## openspec/changes/frontend-auth-alignment/tasks.md

- Source: openspec/changes/frontend-auth-alignment/tasks.md
- Lines: 1-22
- SHA256: 296e98178217af5f70b06061758b2d65fa95d1cfd0823f279db4b6d42c03ea9e

```md
## 1. 需求分析与契约确认

- [ ] 1.1 列出所有人类控制台会调用的 REST 接口清单
- [ ] 1.2 确认哪些接口当前只接受 Bearer token、哪些已接受 session

## 2. 后端中间件实现

- [ ] 2.1 在 `backend/internal/transport/rest/session_middleware.go` 中新增 `authenticateHumanOrAgent` 组合中间件
- [ ] 2.2 在 `backend/internal/transport/rest/router.go` 中把任务/执行/审核/声望/提交 diff 路由切换到组合中间件
- [ ] 2.3 保留 Agent 自服务接口的纯 Bearer token 鉴权

## 3. 测试与验证

- [ ] 3.1 更新/新增 REST 路由测试，验证 session cookie 可访问任务列表
- [ ] 3.2 更新/新增 REST 路由测试，验证 Bearer token 仍可访问任务列表
- [ ] 3.3 运行 `go test -race ./internal/transport/rest/...`
- [ ] 3.4 运行前端单元测试 `npm test -- --run`

## 4. 文档与状态更新

- [ ] 4.1 在 change 的 tasks.md 中勾选完成的任务
- [ ] 4.2 运行 Comet open 阶段守卫
```


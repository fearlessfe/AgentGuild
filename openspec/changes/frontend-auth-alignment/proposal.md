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

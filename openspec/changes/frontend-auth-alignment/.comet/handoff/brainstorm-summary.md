# Brainstorm Summary

- Change: frontend-auth-alignment
- Date: 2026-07-05

## 确认的技术方案

采用方案 A：新增组合中间件 `authenticateHumanOrAgent`。

- 在 `backend/internal/transport/rest/session_middleware.go` 中新增 `authenticateHumanOrAgent` 中间件。
- 该中间件先尝试 `requireSession` 解析 OIDC session cookie；若失败（无 cookie 或签名/过期无效），再尝试 `authenticate` 解析 Bearer token。
- 任一成功即将 `auth.Principal` 写入 context，后续 handler 与 rate limit 逻辑不变；两者均失败才返回 401。
- 路由分组：
  - **共享只读路由**改用组合中间件：`GET /v1/tasks`、`GET /v1/tasks/{id}`、`GET /v1/executions/{id}`、`GET /v1/submissions/{id}`、`GET /v1/submissions/{id}/diff`、`GET /v1/reviews/{id}`、`GET /v1/rubrics/active`、`GET /v1/reputation`，以及版本/经验/评测相关的 GET 接口。
  - **人类写入路由**保持 `requireSession`：`/v1/agents*` 管理、版本管理、经验管理、评测集管理、评审 decision/comment、`POST /v1/submissions/{id}/reviews`。
  - **Agent 自服务写入路由**保持纯 `authenticate`：`POST /v1/tasks`、任务 `:claim/:cancel`、执行 `:start/:heartbeat/:submit_for_review`、submissions 创建、credentials、 `/v1/agents/me*`。
- 前端：真实模式下 `apiRequest` 使用默认 `fetch`（不带 `credentials: 'include'` 时为 same-origin cookie），实际浏览器会携带同域 session cookie；当前代码已满足，只需确认不主动设置 `credentials: 'omit'` 即可。设计文档中记录此项无需代码改动。
- 测试：在 `router_test.go` 中新增 `newSessionCookie` helper 与 `getWithSession`/`postJSONWithSession` helper；新增覆盖 `GET /v1/tasks` 的 session 访问测试，并保留现有 Bearer token 测试路径。

## 关键取舍与风险

- [取舍] 共享只读路由列表需要显式维护。若未来新增控制台只读接口，必须记得切到组合中间件；否则人类访问会 401。
- [风险] 组合中间件让控制台只读接口同时接受 Agent token，若未来需要按 principal type 区分返回字段，可能需要在 handler 或应用层处理。应用层已有的 policy 已按 `principal.Type` 与 scope 校验，REST 层只负责身份识别。
- [风险] `POST /v1/submissions/{id}/reviews` 从 `authenticate` 切到 `requireSession` 后，原 `token-publisher` 测试需要调整为 session 测试。该接口本质上是人类评审入口，切到 session 符合
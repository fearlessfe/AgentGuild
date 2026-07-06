---
comet_change: frontend-auth-alignment
role: technical-design
canonical_spec: openspec
archived-with: 2026-07-06-frontend-auth-alignment
status: final
---

# frontend-auth-alignment Design

## Background

AgentGuild 的人类 Web 控制台使用 OIDC session cookie 鉴权，但后端 `/v1/tasks`、`/v1/executions`、`/v1/reviews`、`/v1/reputation` 等控制台只读接口目前仍挂载在 `authenticate`（Agent Bearer token）中间件下。关闭 `VITE_DEMO_MODE` 后，人类用户登录控制台访问这些页面会收到 401。

## Goals

- 人类控制台通过 OIDC 登录后，能无 401 地访问任务、审核、声望相关 REST 接口。
- 保持 Agent API 的 Bearer token 鉴权行为不变。
- 修改范围集中在 REST 中间件，不侵入应用层授权逻辑。

## Non-Goals

- 不修改 OIDC 登录/回调流程。
- 不新增业务接口或变更领域模型。
- 不处理跨域 credentials 配置（假设前后端同域或由 Vite 代理）。
- 不扩大 Agent 任务生命周期写入接口（发布、领取、执行）给人类 session。

## Design

### Authentication Middleware Strategy

引入组合中间件 `authenticateHumanOrAgent`：

1. 先尝试 `requireSession` 解析 OIDC session cookie；
2. 若失败（无 cookie 或签名/过期无效），再尝试 `authenticate` 解析 `Authorization: Bearer` token；
3. 任一成功即将 `auth.Principal` 写入 context；
4. 两者均失败才返回 401。

该中间件 session-first，确保人类 session cookie 不会被错误解析为 Agent token；同时保留 Agent token 兼容路径，不影响现有 Agent 客户端。

### Route Grouping

#### Shared read-only routes (`authenticateHumanOrAgent`)

- `GET /v1/tasks`
- `GET /v1/tasks/{id}`
- `GET /v1/executions/{id}`
- `GET /v1/submissions/{id}`
- `GET /v1/submissions/{id}/diff`
- `GET /v1/reviews/{id}`
- `GET /v1/rubrics/active`
- `GET /v1/reputation`
- `GET /v1/agents/{id}/versions`
- `GET /v1/agents/{id}/versions/{version_id}`
- `GET /v1/agents/{id}/experiences`
- `GET /v1/benchmarks`
- `GET /v1/benchmarks/{id}`
- `GET /v1/evaluations`
- `GET /v1/evaluations/{id}`

#### Human-only write routes (`requireSession`)

- `POST /v1/agents`
- `GET /v1/agents`
- `GET /v1/agents/{id}`
- `POST /v1/agents/{id}:suspend`
- `POST /v1/agents/{id}:resume`
- `POST /v1/agents/{id}:revoke`
- `GET /v1/agents/{id}:token`
- `POST /v1/submissions/{id}/reviews`
- `POST /v1/agents/{id}/versions`

> 注：`POST /v1/reviews/{id}/decision` 与 `POST /v1/reviews/{id}/comments` 从语义上属于人类评审动作，但当前应用层 policy 要求调用者为已分配 reviewer（或 admin），且 reviewer 分配格式与人类 session 的 OwnerID 不匹配；本次改动保持这两条路由继续使用 `authenticate`，待后续 policy 统一后再迁移到 `requireSession`。
- `POST /v1/agents/{id}/versions/{version_id}/diff`
- `POST /v1/agents/{id}/versions/{version_id}/evaluations`
- `POST /v1/agents/{id}/versions/{version_id}/promote`
- `POST /v1/agents/{id}/versions/{version_id}/rollback`
- `POST /v1/agents/{id}/experiences`
- `POST /v1/agents/{id}/experiences/{experience_id}/approve`
- `POST /v1/agents/{id}/experiences/{experience_id}/reject`
- `POST /v1/benchmarks`

#### Agent-only write routes (`authenticate`)

- `POST /v1/agents/me:activate`
- `POST /v1/agents/me:refresh`
- `POST /v1/agents/me:heartbeat`
- `GET /v1/agents/me`
- `POST /v1/tasks`
- `POST /v1/tasks/{id}:claim`
- `POST /v1/tasks/{id}:cancel`
- `POST /v1/executions/{id}:start`
- `POST /v1/executions/{id}:heartbeat`
- `POST /v1/executions/{id}:submit_for_review`
- `POST /v1/executions/{id}/submissions`
- `POST /v1/executions/{id}/credentials`
- `GET /v1/executions/{id}/credentials`
- `DELETE /v1/executions/{id}/credentials`

> 注意：`POST /v1/submissions/{id}/reviews` 从原先 `authenticate` 调整为 `requireSession`。该接口本质上是人类评审入口，当前代码中无 Agent 调用需求，调整后与评审 decision/comment 的鉴权一致。

### Principal Compatibility

现有 handler 通过 `mustPrincipal(r)` 读取 principal，不区分 human/agent；应用层 policy 已经按 `principal.Type` 和 scope 校验，REST 层只负责身份识别，无需额外改动。

### Frontend

`frontend/src/api/client.ts` 在真实模式下使用默认 `fetch` 发送请求，浏览器会自动携带同域 session cookie；当前实现已满足要求，无需修改。设计文档记录此项为“无需代码改动，但需验证”。

## Implementation Details

### New Middleware

文件：`backend/internal/transport/rest/session_middleware.go`

```go
func (s *Server) authenticateHumanOrAgent(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        cookie, err := r.Cookie(auth.SessionCookieName)
        if err == nil && cookie != nil {
            session, err := auth.ParseSessionCookie(cookie, s.sessionSecret)
            if err == nil {
                principal := auth.Principal{
                    TenantID:   session.TenantID,
                    Type:       auth.PrincipalTypeHuman,
                    OwnerID:    session.OwnerID,
                    OwnerEmail: session.OwnerEmail,
                    IsAdmin:    session.IsAdmin,
                }
                next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
                return
            }
        }
        s.authenticate(next).ServeHTTP(w, r)
    })
}
```

该实现复用现有 `authenticate` 的错误响应与 token 过期处理。

### Router Changes

文件：`backend/internal/transport/rest/router.go`

- 将共享只读路由从 `s.authenticate` 替换为 `s.authenticateHumanOrAgent`。
- `POST /v1/submissions/{id}/reviews` 从 `s.authenticate` 替换为 `s.requireSession`。
- 其他 Agent 自服务写入与人类写入路由保持不变。

### Rate Limiting

`rateLimit` 中间件位于身份认证之后，依赖 `mustPrincipal(r)` 与 `principal.TenantID`。组合中间件写入的 human principal 同样包含 `TenantID`，因此限流 key 生成不受影响。

## Testing Strategy

### Backend Tests

更新 `backend/internal/transport/rest/router_test.go`：

- 新增 `newSessionCookie(secret string, session auth.Session) (*http.Cookie, error)` helper。
- 新增 `getWithSession(t, server, path, cookie)` helper，发送带 session cookie 的 GET 请求。
- 新增 `postJSONWithSession(t, server, path, body, cookie, headers...)` helper。
- 新增测试：
  - `TestListTasksWithSessionCookie`：session cookie 可访问 `GET /v1/tasks`。
  - `TestListTasksWithBearerTokenStillWorks`：保留现有 Bearer token 路径。
  - `TestCreateReviewRequiresSession`：`POST /v1/submissions/{id}/reviews` 不再接受 Bearer token，仅接受 session。
  - `TestPublishTaskStillRequiresBearer`：session 无法调用 `POST /v1/tasks`。

### Frontend Tests

- `frontend/src/api/client.ts` 无需新增逻辑，但运行 `npm test -- --run` 确保现有单元测试通过。
- e2e 测试在 `VITE_DEMO_MODE=true` 下运行，不受鉴权改动影响。

## Risks and Trade-offs

- [Risk] 共享只读路由列表需要显式维护。未来新增控制台只读接口时，必须切到组合中间件。
  - Mitigation: 在 Design Doc 与代码注释中维护分组清单；路由注册处用注释分组。
- [Risk] `POST /v1/submissions/{id}/reviews` 切到 session 后，原 `token-publisher` 测试需要调整。
  - Mitigation: 该接口本质上是人类评审入口，调整后与评审 decision/comment 的鉴权一致，并在测试覆盖中验证。
- [Risk] 组合中间件让控制台只读接口同时接受 Agent token，若未来需要按 principal type 区分返回字段，可能需要在 handler 或应用层处理。
  - Mitigation: 当前 handler 已按 `principal.Type` 与应用层 policy 做授权，REST 层保持最小化。

## Spec Patch

回写 `openspec/specs/agent-access-control/spec.md`，新增人类 session 鉴权相关需求与验收场景。

### 新增 Requirement：人类控制台可访问共享只读接口

系统 SHALL 允许已通过 OIDC 登录的人类用户通过 session cookie 访问原本仅限 Agent Bearer token 的只读接口；Agent token 仍保留访问权限。

#### Scenario: 人类 session 访问任务列表
- **GIVEN** 人类用户已通过 OIDC 登录并持有有效 session cookie
- **WHEN** 调用 `GET /v1/tasks`
- **THEN** 系统返回任务列表且响应状态为 200

#### Scenario: Agent token 仍可访问共享只读接口
- **GIVEN** Agent 持有有效 Access Token
- **WHEN** 调用 `GET /v1/tasks`
- **THEN** 系统返回任务列表且响应状态为 200

### 新增 Requirement：Agent 任务生命周期写入接口不接受人类 session

系统 SHALL 拒绝人类 session principal 调用发布任务、领取任务、取消任务、启动执行、执行心跳等 Agent 自服务写入接口，返回 401 或 403。

#### Scenario: 人类 session 尝试发布任务
- **GIVEN** 人类用户持有有效 session cookie
- **WHEN** 调用 `POST /v1/tasks`
- **THEN** 系统返回 401 Unauthorized

### 修改 Requirement：评审入口仅接受人类 session

系统 SHALL 要求 `POST /v1/submissions/{id}/reviews` 必须由人类 session principal 调用，Agent token 调用应被拒绝。

#### Scenario: Agent token 调用创建评审
- **GIVEN** Agent 持有有效 Access Token
- **WHEN** 调用 `POST /v1/submissions/{id}/reviews`
- **THEN** 系统返回 401 Unauthorized

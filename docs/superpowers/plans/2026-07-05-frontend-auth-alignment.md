---
change: frontend-auth-alignment
design-doc: docs/superpowers/specs/2026-07-05-frontend-auth-alignment-design.md
base-ref: 4de8c94285bb04d4963375f234438a30c599c46b
archived-with: 2026-07-06-frontend-auth-alignment
---

# frontend-auth-alignment 实施计划

本计划依据设计文档 `docs/superpowers/specs/2026-07-05-frontend-auth-alignment-design.md` 与任务边界 `openspec/changes/frontend-auth-alignment/tasks.md` 制定，目标是将人类控制台 OIDC session cookie 鉴权对齐到任务、执行、提交、评审、声望等只读 REST 接口，同时保持 Agent Bearer token 路径不变。

## 1. 需求分析与契约确认

### 1.1 梳理人类控制台只读接口清单

- 文件：`backend/internal/transport/rest/router.go`
- 当前问题：`GET /v1/tasks`、`GET /v1/tasks/{id}`、`GET /v1/executions/{id}`、`GET /v1/submissions/{id}`、`GET /v1/submissions/{id}/diff`、`GET /v1/reviews/{id}`、`GET /v1/rubrics/active`、`GET /v1/reputation`、`GET /v1/agents/{id}/versions`、`GET /v1/agents/{id}/versions/{version_id}`、`GET /v1/agents/{id}/experiences`、`GET /v1/benchmarks`、`GET /v1/benchmarks/{id}`、`GET /v1/evaluations`、`GET /v1/evaluations/{id}` 均挂载在 `s.authenticate` 下。
- 预期改动：上述路由切换为 `s.authenticateHumanOrAgent`。

### 1.2 确认写入接口鉴权边界

- Agent 自服务写入接口保持 `s.authenticate`：发布/领取/取消任务、启动/心跳执行、创建 submission、Git credential 操作、Agent 自服务 (`/agents/me:*`)。
- 人类写入接口保持 `s.requireSession`：注册/管理 Agent、评审 decision/comment、版本管理、经验治理、基准集创建、`POST /v1/executions/{id}:submit_for_review`。
- 特殊调整：`POST /v1/submissions/{id}/reviews` 从 `s.authenticate` 改为 `s.requireSession`。

### 1.3 确认前端无需修改

- 文件：`frontend/src/api/client.ts`
- 真实模式下 `apiRequest` 使用默认 `fetch` 并随浏览器自动携带同域 session cookie；无需新增代码。
- 验证命令：
  ```bash
  cd frontend && npm test -- --run
  ```

## 2. 后端中间件实现

### 任务 2.1：新增 `authenticateHumanOrAgent` 组合中间件

- 目标文件：`backend/internal/transport/rest/session_middleware.go`
- 改动点：在 `requireSession` 与 `identityPrincipalFromAuth` 之间新增方法：
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
- 约束：session-first，失败时复用 `s.authenticate` 的错误响应；`s.sessionSecret` 为空时 session 解析自然失败并回退到 Bearer。
- 验证命令：
  ```bash
  cd backend && go build ./internal/transport/rest/...
  ```

### 任务 2.2：调整 `router.go` 路由分组

- 目标文件：`backend/internal/transport/rest/router.go`
- 改动点（按当前 router.go 行号）：
  - 将 `GET /v1/tasks`（约第 210 行）、`GET /v1/tasks/{id}`（约第 211 行）的 `s.authenticate` 替换为 `s.authenticateHumanOrAgent`。
  - 将 `GET /v1/executions/{id}`（约第 215 行）替换为 `s.authenticateHumanOrAgent`。
  - 将 `GET /v1/submissions/{id}`（约第 221 行，在 `if s.submissions != nil` 内）替换为 `s.authenticateHumanOrAgent`。
  - 将 `POST /v1/submissions/{id}/reviews`（约第 224 行）替换为 `s.requireSession`。
  - 将 `GET /v1/submissions/{id}/diff`（约第 225 行）替换为 `s.authenticateHumanOrAgent`。
  - 将 `GET /v1/reviews/{id}`（约第 228 行）、`GET /v1/rubrics/active`（约第 229 行）、`GET /v1/reputation`（约第 230 行）替换为 `s.authenticateHumanOrAgent`。
  - 将 `POST /v1/executions/{id}:submit_for_review`（约第 232 行）保持 `s.requireSession`（当前已是）。
  - 在 `if s.versions != nil` 块内，将 `GET /v1/agents/{id}/versions`（约第 235 行）、`GET /v1/agents/{id}/versions/{version_id}`（约第 237 行）替换为 `s.authenticateHumanOrAgent`；版本写入接口保持 `s.requireSession`。
  - 在 `if s.experiences != nil` 块内，将 `GET /v1/agents/{id}/experiences`（约第 245 行）替换为 `s.authenticateHumanOrAgent`；经验写入接口保持 `s.requireSession`。
  - 在 `if s.evaluations != nil` 块内，将 `GET /v1/benchmarks`（约第 252 行）、`GET /v1/benchmarks/{id}`（约第 254 行）、`GET /v1/evaluations`（约第 255 行）、`GET /v1/evaluations/{id}`（约第 256 行）替换为 `s.authenticateHumanOrAgent`；`POST /v1/benchmarks` 保持 `s.requireSession`。
- 代码组织建议：在路由注册处用注释分成三组，便于后续维护：
  ```go
  // Shared read-only routes: human session OR agent bearer
  // Human-only write routes
  // Agent-only write routes
  ```
- 验证命令：
  ```bash
  cd backend && go build ./...
  ```

### 任务 2.3：保留 Agent 自服务接口的 Bearer token 鉴权

- 目标文件：`backend/internal/transport/rest/router.go`
- 改动点：确认以下路由保持 `s.authenticate`，不做变更：
  - `POST /v1/agents/me:activate`
  - `POST /v1/agents/me:refresh`
  - `POST /v1/agents/me:heartbeat`
  - `GET /v1/agents/me`
  - `POST /v1/tasks`
  - `POST /v1/tasks/{id}:claim`
  - `POST /v1/tasks/{id}:cancel`
  - `POST /v1/executions/{id}:start`
  - `POST /v1/executions/{id}:heartbeat`
  - `POST /v1/executions/{id}/submissions`
  - `POST/GET/DELETE /v1/executions/{id}/credentials`
- 验证命令：
  ```bash
  cd backend && go test -race ./internal/transport/rest/... -run 'TestPublish|TestClaim|TestExecutionEndpoints|TestCancel'
  ```

## 3. 测试与验证

### 任务 3.1：新增/更新 REST 路由测试

- 目标文件：`backend/internal/transport/rest/router_test.go`
- 改动点：
  1. 引入 `auth` 包已存在，无需新增 import。
  2. 新增 session helper：
     ```go
     func newSessionCookie(secret string, session auth.Session) (*http.Cookie, error) {
         return auth.NewSessionCookie(session, secret, false)
     }
     ```
  3. 新增请求 helper：
     ```go
     func getWithSession(t *testing.T, server http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
         t.Helper()
         req := httptest.NewRequest(http.MethodGet, path, nil)
         if cookie != nil {
             req.AddCookie(cookie)
         }
         rec := httptest.NewRecorder()
         server.ServeHTTP(rec, req)
         return rec
     }

     func postJSONWithSession(t *testing.T, server http.Handler, path, body string, cookie *http.Cookie, headers ...string) *httptest.ResponseRecorder {
         t.Helper()
         req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
         req.Header.Set("Content-Type", "application/json")
         if cookie != nil {
             req.AddCookie(cookie)
         }
         for i := 0; i+1 < len(headers); i += 2 {
             req.Header.Set(headers[i], headers[i+1])
         }
         rec := httptest.NewRecorder()
         server.ServeHTTP(rec, req)
         return rec
     }
     ```
  4. 调整 `newTestServer`，使其支持 `WithSession`：
     ```go
     func newTestServer(app *fakeApplication, opts ...rest.Option) http.Handler {
         allOpts := append([]rest.Option{rest.WithSession("secret-key-with-at-least-32-bytes", false)}, opts...)
         return rest.NewServer(app, &tokenVerifier{}, allOpts...).Router()
     }
     ```
     注意：此调整可能影响不依赖 session 的现有测试，需确认 `WithSession` 不会破坏 Bearer-only 测试行为。
  5. 新增/修改测试用例：
     - `TestListTasksWithSessionCookie`：构造 `auth.Session{TenantID:"tenant-1", OwnerID:"owner-1", OwnerEmail:"owner@example.com"}`，调用 `GET /v1/tasks`，断言 200，并验证 `ListTasks` 收到的 principal.Type 为 `PrincipalTypeHuman`。
     - `TestListTasksWithBearerTokenStillWorks`：使用 `token-publisher` 调用 `GET /v1/tasks`，断言 200，验证 backward compatibility。
     - `TestCreateReviewRequiresSession`：使用 Agent Bearer token (`token-publisher`) 调用 `POST /v1/submissions/sub-1/reviews`，断言 401；使用有效 session cookie 调用同一接口，断言 201。
     - `TestPublishTaskStillRequiresBearer`：使用有效 session cookie 调用 `POST /v1/tasks`，断言 401（`missing or invalid authorization`）。
  6. 更新现有 `TestCreateReview`：当前使用 `token-publisher` Bearer token，改为使用 session cookie，否则测试会在路由层返回 401。
- 验证命令：
  ```bash
  cd backend && go test -race ./internal/transport/rest/... -run 'TestListTasksWithSessionCookie|TestListTasksWithBearerTokenStillWorks|TestCreateReviewRequiresSession|TestPublishTaskStillRequiresBearer|TestCreateReview'
  ```

### 任务 3.2：运行后端全量测试

- 命令：
  ```bash
  cd backend && go test -race ./internal/transport/rest/... -count=1
  cd backend && go test -race ./... -count=1
  ```
- 说明：后端全量测试依赖 PostgreSQL，由 `internal/testdb` 自动处理。

### 任务 3.3：运行前端单元测试

- 文件：`frontend/src/api/client.ts` 无需修改，但需确保现有测试通过。
- 命令：
  ```bash
  cd frontend && npm test -- --run
  ```

### 任务 3.4：构建验证

- 命令：
  ```bash
  make build
  ```
  或分别执行：
  ```bash
  cd backend && go build ./...
  cd frontend && npm run build
  ```

## 4. 文档与 OpenSpec 状态更新

### 任务 4.1：确认 OpenSpec 规范已对齐

- 目标文件：`openspec/specs/agent-access-control/spec.md`
- 当前状态：设计文档要求回写的三个需求已存在：
  - "Requirement: 人类控制台可访问共享只读接口"
  - "Requirement: Agent 任务生命周期写入接口不接受人类 session"
  - "Requirement: 评审入口仅接受人类 session"
- 改动点：如验收场景措辞与最终实现不一致（例如状态码 401 vs 403），按需微调；若已一致则无需修改。
- 验证命令：
  ```bash
  grep -n "人类控制台可访问共享只读接口\|Agent 任务生命周期写入接口不接受人类 session\|评审入口仅接受人类 session" openspec/specs/agent-access-control/spec.md
  ```

### 任务 4.2：更新 change 任务清单

- 目标文件：`openspec/changes/frontend-auth-alignment/tasks.md`
- 改动点：完成上述任务后，将对应复选框 `[ ]` 改为 `[x]`。
- 应勾选的条目：
  - 1.1、1.2
  - 2.1、2.2、2.3
  - 3.1、3.2、3.3、3.4
  - 4.1、4.2

## 5. 风险与回滚

| 风险 | 缓解措施 |
|------|----------|
| 共享只读路由列表遗漏 | 对照设计文档的 "Shared read-only routes" 与 "Agent-only write routes" 清单逐条检查 router.go；新增路由时按注释分组添加。 |
| `POST /v1/submissions/{id}/reviews` 切到 session 后现有测试失败 | 同步更新 `TestCreateReview` 与新增 `TestCreateReviewRequiresSession` 覆盖。 |
| `rateLimit` 中间件依赖 principal | 组合中间件写入的 human principal 同样包含 `TenantID`，限流 key 生成逻辑无需改动；测试覆盖 session 路径的限流行为。 |
| 前端真实模式 401 | 若后端改动正确且前后端同域/Vite 代理正常，浏览器会自动携带 cookie；如仍 401，检查 `SESSION_COOKIE_SECRET` 配置与 cookie 域。 |

## 6. 实施顺序

1. 完成任务 2.1（新增组合中间件）。
2. 完成任务 2.2（路由切换）与 2.3（确认保留）。
3. 完成任务 3.1（测试新增与调整）。
4. 运行 3.2、3.3、3.4 验证。
5. 完成任务 4.1 与 4.2 文档更新。

---
change: agent-task-lifecycle
design-doc: docs/superpowers/specs/2026-07-02-agent-task-lifecycle-design.md
base-ref: c46a757df241db383a3a9aec386efa11d7f4472e
---

# Agent Task Lifecycle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建可由 React、REST 和远程 MCP 客户端共同使用的多租户任务生命周期服务，支持并发 Claim、10 分钟 Lease、generation fencing、deadline、审计和 Langfuse 成本观测。

**Architecture:** 采用 Go 模块化单体：领域对象只维护状态机不变量，Application Service 统一编排授权、幂等、事务、审计和 outbox，REST 与 MCP 仅做协议适配。PostgreSQL 是状态与时间事实源；React 只提供观察页面；Langfuse 通过可替换 `TraceCostProvider` 异步接入。

**Tech Stack:** Go 1.26.4、PostgreSQL 18.4、pgx v5、chi、官方 MCP Go SDK v1.6.1、OpenTelemetry OTLP/HTTP、React 19、TypeScript、Vite、TanStack Query、Vitest、Playwright。

## Global Constraints

- 单租户启用但所有持久化记录和查询从第一天携带 `tenant_id`。
- Task 必须具有 `deadline`，不得引入 `max_runtime_seconds`。
- Lease 软到期为 10 分钟，网络宽限为 30 秒，建议 heartbeat 间隔为 60 秒。
- 所有时间判断使用 PostgreSQL 时间；客户端时间不能驱动状态迁移。
- 同一生产 Task 同时最多一个非终态 Execution。
- REST 与 MCP 共享 Application Service、领域错误、授权和幂等实现。
- MCP 使用无状态 Streamable HTTP；写工具必须包含 `request_id`。
- MCP Execution 写工具只接受 OAuth 主体、`execution_id` 和 `lease_generation`，不得接受 Lease Token。
- Langfuse 不可用不得回滚或阻塞任务生命周期事务。
- 首版不依据成本自动终止 Execution。
- `go.mod` 使用 `go 1.26.0` 和 `toolchain go1.26.4`；PostgreSQL 容器固定 `postgres:18.4`。
- Langfuse Cloud 使用 Metrics API v2；自托管部署使用配置指定的兼容读取模式，领域层不得依赖具体 API 版本。
- OAuth 通过可替换 `TokenVerifier` 解析 `Principal`；限流通过可替换 `RateLimiter` 接入，本地实现不声明为跨实例全局限流。
- React 任务页以 `docs/assets/agentguild-tasks.png` 为主要视觉事实源；保留深色高密度工作台、左侧导航、顶部上下文栏、分组任务表和右侧详情栏。
- `docs/assets/agentguild-code-review.png` 与 `docs/assets/agentguild-review-command-center.png` 仅用于复用全局导航、间距、状态色和面板语言；本 change 不实现代码评审界面。
- 设计稿中的发布、领取、分配和审核按钮超出当前只读 React 范围，不得仅为视觉还原而绕过本 change 的权限与范围约束。

## File Map

```text
backend/
  cmd/agentguild-api/main.go             组合依赖并启动 HTTP 服务与回收器
  go.mod                                 Go 模块和依赖
  migrations/000001_task_lifecycle.sql   表、约束、索引和回滚
  internal/domain/task.go                Task 状态和意图
  internal/domain/execution.go           Execution、Lease 和 fencing
  internal/domain/errors.go              稳定领域错误
  internal/application/contracts.go      命令、查询、响应和接口
  internal/application/service.go        共享 Application Service
  internal/application/claim.go          Claim/heartbeat/start 事务编排
  internal/postgres/store.go             pgx 事务入口和数据库时间
  internal/postgres/task_repository.go   Task/Execution 持久化与查询
  internal/postgres/idempotency.go       请求摘要、并发去重和稳定响应
  internal/postgres/reaper.go            SKIP LOCKED Lease/deadline 回收
  internal/auth/principal.go              Principal、Scope 和资源授权
  internal/auth/oauth.go                  OAuth JWT/JWKS 校验
  internal/transport/rest/router.go       REST 路由与错误映射
  internal/transport/rest/openapi.yaml    REST 契约
  internal/transport/mcp/server.go        MCP Streamable HTTP Server
  internal/transport/mcp/tools.go         八个意图型工具
  internal/telemetry/cost.go              TraceCostProvider 接口
  internal/telemetry/langfuse.go          Langfuse Metrics API 适配
  internal/worker/outbox.go               outbox 重试与成本同步
  internal/ratelimit/limiter.go           tenant/Agent 限流
  internal/*/*_test.go                    单元、集成和契约测试
frontend/
  package.json                            React 工具链
  src/app/AppShell.tsx                    设计稿对应的导航与工作台框架
  src/api/client.ts                       REST 查询客户端
  src/features/tasks/TaskList.tsx         列表、筛选和轮询
  src/features/tasks/TaskDetail.tsx       只读详情
  src/styles/tokens.css                    深色主题、状态色、间距和排版 token
  src/features/tasks/*.test.tsx           组件测试
  e2e/task-observer.spec.ts               浏览器验收
docker-compose.yml                        本地 PostgreSQL
Makefile                                  统一 build/test/verify 命令
```

---

### Task 1: 建立项目骨架与领域状态机

- [x] **Completion gate: Task 1 domain lifecycle**

**Files:**
- Create: `backend/go.mod`
- Create: `backend/internal/domain/errors.go`
- Create: `backend/internal/domain/task.go`
- Create: `backend/internal/domain/execution.go`
- Test: `backend/internal/domain/task_test.go`
- Test: `backend/internal/domain/execution_test.go`
- Create: `Makefile`
- Create: `docker-compose.yml`

**Interfaces:**
- Produces: `domain.NewTask(...) (*Task, error)`
- Produces: `domain.NewLeasedExecution(...) (*Execution, error)`
- Produces: `domain.Task.Apply(intent Intent, actor Actor, now time.Time) error`
- Produces: `domain.Execution.Start(now time.Time, generation int64) error`
- Produces: `domain.Execution.Heartbeat(now time.Time, generation int64) (Lease, error)`
- Produces: `domain.Error{Code, Message, Field}`

- [x] **Step 1: 写状态迁移失败测试**

```go
func TestTaskRejectsProgressBeforeClaim(t *testing.T) {
    task, err := domain.NewTask("task-1", "tenant-1", "publisher-1", time.Now().Add(time.Hour))
    require.NoError(t, err)
    err = task.Apply(domain.IntentStart, domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}, time.Now())
    require.ErrorIs(t, err, domain.ErrStateConflict)
    require.Equal(t, domain.TaskOpen, task.Status)
}

func TestHeartbeatRejectsStaleGeneration(t *testing.T) {
    execution, err := domain.NewLeasedExecution("exe-1", "task-1", "tenant-1", "agent-1", time.Now(), 3)
    require.NoError(t, err)
    _, err = execution.Heartbeat(time.Now().Add(time.Minute), 2)
    require.ErrorIs(t, err, domain.ErrLeaseExpired)
}
```

- [x] **Step 2: 运行领域测试并确认失败**

Run: `cd backend && go test ./internal/domain -run 'TestTaskRejects|TestHeartbeatRejects' -count=1`

Expected: FAIL，原因是 `domain` 类型尚未定义。

- [x] **Step 3: 实现明确意图状态机和 Lease 常量**

`backend/go.mod` 固定运行时：

```go
module agentguild.dev/agentguild/backend

go 1.26.0

toolchain go1.26.4
```

`docker-compose.yml` 固定 `postgres:18.4`，不得使用浮动标签。

```go
const (
    LeaseDuration = 10 * time.Minute
    LeaseGrace    = 30 * time.Second
)

type Lease struct {
    Generation int64
    SoftExpiry time.Time
    HardExpiry time.Time
}

func RenewLease(now time.Time, current int64) Lease {
    soft := now.Add(LeaseDuration)
    return Lease{Generation: current + 1, SoftExpiry: soft, HardExpiry: soft.Add(LeaseGrace)}
}
```

Task 和 Execution 仅暴露 `Publish`、`Claim`、`Cancel`、`Start`、`Heartbeat`、`Expire` 等意图方法；不创建 `SetStatus`。

- [x] **Step 4: 添加表驱动合法/非法迁移和随机序列不变量测试**

```go
func FuzzExecutionNeverReturnsFromTerminal(f *testing.F) {
    f.Add(uint8(0))
    f.Fuzz(func(t *testing.T, sequence uint8) {
        e, err := domain.NewLeasedExecution("exe-1", "task-1", "tenant-1", "agent-1", time.Now(), 1)
        require.NoError(t, err)
        // 随机执行 Start、Heartbeat、Accept、Expire；
        // 一旦进入 Accepted 或 Expired，后续明确意图不得改变终态。
        runRandomExplicitIntents(t, e, sequence)
    })
}
```

- [x] **Step 5: 运行领域测试和格式检查**

Run: `cd backend && gofmt -w internal/domain && go test ./internal/domain -count=1`

Expected: PASS。

- [x] **Step 6: 提交领域骨架**

```bash
git add backend/go.mod backend/internal/domain Makefile docker-compose.yml
git commit -m "feat: define task and execution lifecycle"
```

---

### Task 2: 建立 PostgreSQL Schema、事务和 Repository

- [x] **Completion gate: Task 2 PostgreSQL persistence**

**Files:**
- Create: `backend/internal/application/ports.go`
- Create: `backend/migrations/000001_task_lifecycle.up.sql`
- Create: `backend/migrations/000001_task_lifecycle.down.sql`
- Create: `backend/internal/postgres/store.go`
- Create: `backend/internal/postgres/task_repository.go`
- Create: `backend/internal/postgres/idempotency.go`
- Test: `backend/internal/postgres/repository_test.go`
- Test: `backend/internal/postgres/idempotency_test.go`

**Interfaces:**
- Consumes: `domain.Task`、`domain.Execution`、`domain.Error`
- Produces: `application.Store.WithTx(ctx, func(application.Tx) error) error`
- Produces: `application.Tx` 和 Task/Execution/Idempotency/Event repository ports；ports 不依赖 PostgreSQL
- Produces: `application.TaskRepository`、`ExecutionRepository`、`IdempotencyRepository`、`EventRepository`

- [x] **Step 1: 写迁移约束集成测试**

```go
func TestOnlyOneNonTerminalExecutionPerTask(t *testing.T) {
    db := testdb.StartPostgres(t)
    seedTask(t, db, "task-1")
    insertExecution(t, db, "exe-1", "task-1", "leased")
    err := tryInsertExecution(db, "exe-2", "task-1", "running")
    require.Error(t, err)
    require.Contains(t, err.Error(), "executions_one_active_per_task")
}
```

- [x] **Step 2: 运行测试并确认迁移缺失**

Run: `cd backend && go test ./internal/postgres -run TestOnlyOneNonTerminalExecutionPerTask -count=1`

Expected: FAIL，原因是迁移或测试数据库辅助代码不存在。

- [x] **Step 3: 创建多租户表和部分唯一索引**

```sql
CREATE UNIQUE INDEX executions_one_active_per_task
ON executions (tenant_id, task_id)
WHERE status IN ('leased', 'running', 'submitted', 'validating', 'reviewing', 'revision_requested');

CREATE UNIQUE INDEX idempotency_scope_key
ON idempotency_records (tenant_id, actor_id, operation, request_id);
```

同一迁移创建 `tasks`、`executions`、`idempotency_records`、`task_events`、`outbox_events`、`execution_usage`；所有业务主键唯一约束必须包含或显式校验 `tenant_id`。

- [x] **Step 4: 实现事务内数据库时间和条件更新**

```go
func (tx *Tx) Now(ctx context.Context) (time.Time, error) {
    tx.nowOnce.Do(func() {
        tx.nowErr = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
    })
    return tx.now, tx.nowErr
}

const claimSQL = `
UPDATE tasks
SET status='active', state_version=state_version+1, active_execution_id=$4, updated_at=$5
WHERE tenant_id=$1 AND id=$2 AND status='open' AND state_version=$3 AND deadline>$5`
```

- [x] **Step 5: 实现规范请求哈希和幂等结果锁定**

```go
func CanonicalHash(v any) ([32]byte, error) {
    payload, err := canonicaljson.Marshal(v)
    if err != nil { return [32]byte{}, err }
    return sha256.Sum256(payload), nil
}
```

读取幂等记录时使用 `SELECT ... FOR UPDATE`；相同 hash 返回已存响应，不同 hash 返回 `IDEMPOTENCY_MISMATCH`。

- [x] **Step 6: 运行迁移、tenant 隔离和幂等并发测试**

Run: `cd backend && go test ./internal/postgres -count=1`

Expected: PASS。

- [x] **Step 7: 提交持久化层**

```bash
git add backend/migrations backend/internal/postgres
git commit -m "feat: persist task lifecycle atomically"
```

---

### Task 3: 实现共享 Application Service 与授权策略

- [x] **Completion gate: Task 3 application service**

**Files:**
- Modify: `backend/internal/domain/execution.go`
- Modify: `backend/internal/domain/execution_test.go`
- Modify: `backend/migrations/000001_task_lifecycle.up.sql`
- Modify: `backend/internal/postgres/migration_test.go`
- Create: `backend/internal/auth/principal.go`
- Create: `backend/internal/application/contracts.go`
- Create: `backend/internal/application/service.go`
- Create: `backend/internal/application/task_commands.go`
- Create: `backend/internal/application/task_queries.go`
- Test: `backend/internal/application/service_test.go`

**Interfaces:**
- Produces: `application.Service.PublishTask`、`ListTasks`、`GetTask`、`CancelTask`
- Produces: `auth.TokenVerifier` 和 `auth.Principal{TenantID, AgentID, AgentVersionID, Scopes}`
- Produces: stable `application.Envelope[T]{Data, Meta}`

- [x] **Step 1: 写权限、deadline 和原子取消测试**

```go
func TestPublishRequiresDeadlineAndScope(t *testing.T) {
    svc := newServiceFixture()
    _, err := svc.PublishTask(ctx, principal("tenant-1", "agent-1"), application.PublishTask{
        RequestID: "req-1", Title: "Fix parser",
    })
    require.Equal(t, domain.CodeInvalidArgument, domain.CodeOf(err))
    require.Equal(t, "deadline", domain.FieldOf(err))
}
```

- [x] **Step 2: 运行 Application Service 测试并确认失败**

Run: `cd backend && go test ./internal/application -count=1`

Expected: FAIL，原因是 Service 尚未定义。

- [x] **Step 3: 定义共享命令、查询和错误边界**

```go
type PublishTask struct {
    RequestID string
    Type, Title, Problem string
    Constraints, Requirements []string
    Deadline time.Time
}

type Meta struct {
    ServerTime time.Time `json:"server_time"`
    ResourceVersion int64 `json:"resource_version"`
    PollAfterSeconds int `json:"poll_after_seconds,omitempty"`
    NextCursor string `json:"next_cursor,omitempty"`
}
```

- [x] **Step 4: 实现授权→幂等→领域→审计/outbox 的固定命令管线**

```go
func (s *Service) PublishTask(ctx context.Context, p auth.Principal, cmd PublishTask) (Envelope[TaskView], error) {
    if err := s.policy.Require(p, "tasks:publish"); err != nil { return Envelope[TaskView]{}, err }
    return withinIdempotentTx(ctx, s.store, p, "task_publish", cmd.RequestID, cmd, func(tx Tx, now time.Time) (TaskView, error) {
        task, err := domain.Publish(cmd.toDomain(p, now))
        if err != nil { return TaskView{}, err }
        return persistTaskAndEvents(ctx, tx, task, p, "publish")
    })
}
```

- [x] **Step 5: 实现 tenant/Scope 过滤和不透明 cursor**

cursor 使用带 HMAC 的版本化 payload：`tenant_id`、过滤摘要、排序键、过期时间；非法或跨过滤条件 cursor 返回字段错误，不披露 payload。

- [x] **Step 6: 运行 Application Service 测试**

Run: `cd backend && go test ./internal/application ./internal/auth -count=1`

Expected: PASS。

- [x] **Step 7: 提交共享应用层**

```bash
git add backend/internal/application backend/internal/auth
git commit -m "feat: add shared task application service"
```

---

### Task 4: 实现 Claim、Lease、heartbeat 和回收器

- [x] **Completion gate: Task 4 claim and lease**

**Files:**
- Create: `backend/internal/application/claim.go`
- Create: `backend/internal/postgres/reaper.go`
- Test: `backend/internal/application/claim_test.go`
- Test: `backend/internal/postgres/claim_concurrency_test.go`
- Test: `backend/internal/postgres/reaper_test.go`

**Interfaces:**
- Produces: `Service.ClaimTask`、`StartExecution`、`HeartbeatExecution`、`GetExecution`
- Produces: `postgres.Reaper.RunBatch(ctx, limit int) (int, error)`

- [x] **Step 1: 写 100 并发 Claim 和 Lease 边界测试**

```go
func TestConcurrentClaimHasExactlyOneWinner(t *testing.T) {
    svc := integrationService(t)
    results := runConcurrent(100, func(i int) error {
        _, err := svc.ClaimTask(ctx, agentPrincipal(i), application.ClaimTask{
            RequestID: fmt.Sprintf("req-%d", i), TaskID: "task-1",
        })
        return err
    })
    require.Equal(t, 1, countNil(results))
    require.Equal(t, 99, countCode(results, domain.CodeStateConflict))
}
```

- [x] **Step 2: 运行并发测试并确认失败**

Run: `cd backend && go test ./internal/postgres -run TestConcurrentClaimHasExactlyOneWinner -count=1`

Expected: FAIL，Claim 尚未实现。

- [x] **Step 3: 实现 Claim 单事务和 generation fencing**

Claim 使用数据库时间、Task 条件更新和部分唯一索引双重防线；heartbeat 条件必须包含：

```sql
WHERE tenant_id=$1
  AND id=$2
  AND agent_version_id=$3
  AND lease_generation=$4
  AND status IN ('leased','running')
  AND lease_hard_expires_at >= $5
```

成功 heartbeat 将 generation 加一，并以数据库时间重算 soft/hard expiry。

- [x] **Step 4: 实现 SKIP LOCKED 回收批次**

```sql
SELECT id
FROM executions
WHERE status IN ('leased','running')
  AND lease_hard_expires_at < clock_timestamp()
ORDER BY lease_hard_expires_at
FOR UPDATE SKIP LOCKED
LIMIT $1;
```

同一事务按 deadline 决定 Task 回到 `open` 或进入 `expired`，并写审计和 outbox；重复运行不得产生第二次状态事件。

- [x] **Step 5: 验证宽限期、旧 generation、deadline 和多回收器**

Run: `cd backend && go test ./internal/application ./internal/postgres -run 'Claim|Heartbeat|Reaper|Deadline' -count=1`

Expected: PASS，且 race detector 下无数据竞争。

- [x] **Step 6: 提交 Lease 生命周期**

```bash
git add backend/internal/application/claim.go backend/internal/postgres/reaper.go backend/internal/**/*claim*test.go backend/internal/**/*reaper*test.go
git commit -m "feat: enforce claim and lease fencing"
```

---

### Task 5: 暴露 REST API、OAuth 和 OpenAPI

- [ ] **Completion gate: Task 5 REST and OAuth**

**Files:**
- Create: `backend/internal/auth/oauth.go`
- Create: `backend/internal/transport/rest/router.go`
- Create: `backend/internal/transport/rest/errors.go`
- Create: `backend/internal/transport/rest/openapi.yaml`
- Test: `backend/internal/transport/rest/router_test.go`
- Test: `backend/internal/auth/oauth_test.go`

**Interfaces:**
- Consumes: `application.Service`
- Produces: `/v1/tasks`、`/v1/tasks/{id}`、`/v1/tasks/{id}:claim`、`/v1/tasks/{id}:cancel`
- Produces: `/v1/executions/{id}`、`:start`、`:heartbeat`

- [ ] **Step 1: 写 REST 状态码、幂等 Header 和不可见资源测试**

```go
func TestClaimMapsConflictWithoutLeakingHolder(t *testing.T) {
    res := postJSON(t, server, "/v1/tasks/task-1:claim", `{"request_id":"req-2"}`, tokenFor("agent-2"))
    require.Equal(t, http.StatusConflict, res.Code)
    require.JSONEq(t, `{"error":{"code":"STATE_CONFLICT","message":"task is not claimable"}}`, res.Body.String())
    require.NotContains(t, res.Body.String(), "agent-1")
}
```

- [ ] **Step 2: 运行 REST 测试并确认失败**

Run: `cd backend && go test ./internal/transport/rest ./internal/auth -count=1`

Expected: FAIL，路由和 OAuth verifier 不存在。

- [ ] **Step 3: 实现可替换 TokenVerifier、OAuth JWT/JWKS 验证和 Principal 注入**

`TokenVerifier.Verify(ctx, rawToken) (Principal, error)` 是 transport 依赖的边界；首个 JWT/JWKS 实现验证 issuer、audience、expiry、tenant、agent/version claims 和 scopes。HTTP middleware 只把验证后的 `auth.Principal` 放入 context，后续 Agent identity change 可替换验证实现而不修改生命周期服务。

- [ ] **Step 4: 实现 REST 适配与稳定映射**

REST 写请求要求 `Idempotency-Key`，并映射到 Application Service `RequestID`；`FORBIDDEN/NOT_FOUND` 对非管理员使用相同安全响应，冲突映射 409，限流映射 429 并返回 `Retry-After`。

- [ ] **Step 5: 编写并校验 OpenAPI 契约**

Run: `cd backend && go run github.com/getkin/kin-openapi/cmd/validate@latest internal/transport/rest/openapi.yaml`

Expected: `openapi document is valid`。

- [ ] **Step 6: 运行 REST 与 OAuth 测试**

Run: `cd backend && go test ./internal/auth ./internal/transport/rest -count=1`

Expected: PASS。

- [ ] **Step 7: 提交 REST 接口**

```bash
git add backend/internal/auth/oauth.go backend/internal/transport/rest
git commit -m "feat: expose authorized task REST API"
```

---

### Task 6: 暴露无状态 Streamable HTTP MCP 工具

- [ ] **Completion gate: Task 6 MCP adapter**

**Files:**
- Create: `backend/internal/transport/mcp/server.go`
- Create: `backend/internal/transport/mcp/tools.go`
- Create: `backend/internal/transport/mcp/errors.go`
- Test: `backend/internal/transport/mcp/server_test.go`
- Test: `backend/internal/transport/contract/equivalence_test.go`

**Interfaces:**
- Consumes: `application.Service` 和 `auth.Principal`
- Produces: `/mcp`
- Produces tools: `task_publish`、`task_list`、`task_get`、`task_claim`、`task_cancel`、`execution_start`、`execution_heartbeat`、`execution_get`

- [ ] **Step 1: 固定官方 SDK 并写工具 Schema 测试**

Run: `cd backend && go get github.com/modelcontextprotocol/go-sdk@v1.6.1`

```go
func TestMutationToolsRequireRequestID(t *testing.T) {
    schema := toolSchema(t, newMCPServer(t), "task_claim")
    require.Contains(t, schema.Required, "request_id")
}

func TestHeartbeatSchemaHasNoLeaseToken(t *testing.T) {
    schema := toolSchema(t, newMCPServer(t), "execution_heartbeat")
    require.NotContains(t, schema.Properties, "lease_token")
}
```

- [ ] **Step 2: 运行 MCP 测试并确认失败**

Run: `cd backend && go test ./internal/transport/mcp -count=1`

Expected: FAIL，MCP Server 尚未注册工具。

- [ ] **Step 3: 使用官方 MCP Go SDK 注册强类型工具**

```go
type HeartbeatInput struct {
    RequestID       string `json:"request_id" jsonschema:"unique mutation request id"`
    ExecutionID     string `json:"execution_id" jsonschema:"execution identifier"`
    LeaseGeneration int64  `json:"lease_generation" jsonschema:"current lease generation"`
    Stage           string `json:"stage,omitempty" jsonschema:"current execution stage"`
    Progress        int    `json:"progress,omitempty" jsonschema:"integer from 0 through 100"`
}
```

每次调用从 HTTP request context 读取 OAuth Principal，工具 handler 不访问 Repository，只调用共享 Service。

- [ ] **Step 4: 实现稳定 `data/meta` 和领域错误映射**

MCP tool error content 只包含稳定 code、安全 message、可选 `retry_after_seconds`；Schema 开启未知字段拒绝和请求体大小限制。

- [ ] **Step 5: 编写 REST/MCP 等价契约和 Schema fuzz 测试**

```go
func TestRESTAndMCPClaimAreEquivalent(t *testing.T) {
    rest := claimViaREST(t, fixture(), principalA, "req-rest")
    mcp := claimViaMCP(t, fixture(), principalA, "req-mcp")
    require.Equal(t, rest.DomainCode, mcp.DomainCode)
    require.Equal(t, rest.TaskStatus, mcp.TaskStatus)
    require.Equal(t, rest.LeaseDuration, mcp.LeaseDuration)
}
```

- [ ] **Step 6: 运行 MCP 与跨协议测试**

Run: `cd backend && go test ./internal/transport/mcp ./internal/transport/contract -count=1`

Expected: PASS。

- [ ] **Step 7: 提交 MCP 适配层**

```bash
git add backend/go.mod backend/go.sum backend/internal/transport/mcp backend/internal/transport/contract
git commit -m "feat: expose task lifecycle over MCP"
```

---

### Task 7: 实现 outbox、Langfuse 成本覆盖、限流和审计查询

- [ ] **Completion gate: Task 7 operations and telemetry**

**Files:**
- Create: `backend/internal/telemetry/cost.go`
- Create: `backend/internal/telemetry/langfuse.go`
- Create: `backend/internal/worker/outbox.go`
- Create: `backend/internal/ratelimit/limiter.go`
- Create: `backend/internal/application/audit_queries.go`
- Test: `backend/internal/telemetry/langfuse_test.go`
- Test: `backend/internal/worker/outbox_test.go`
- Test: `backend/internal/ratelimit/limiter_test.go`

**Interfaces:**
- Produces: `telemetry.TraceCostProvider.Observe(ctx, ExecutionRef) (CostObservation, error)`
- Produces: `CostObservation{ObservedCost, SelfReportedCost, Coverage, Provider, Cursor}`
- Produces: `worker.Outbox.RunBatch(ctx, limit int) (int, error)`
- Produces: `ratelimit.RateLimiter.Allow(ctx, Key) (Decision, error)`

- [ ] **Step 1: 写 Provider 故障不影响领域提交测试**

```go
func TestLangfuseFailureLeavesLifecycleCommitted(t *testing.T) {
    fixture := serviceWithCostProvider(t, telemetry.FailingProvider(errors.New("timeout")))
    task := publishAndClaim(t, fixture)
    require.NoError(t, fixture.Worker.RunOnce(ctx))
    require.Equal(t, domain.TaskActive, loadTask(t, fixture.DB, task.ID).Status)
    require.Equal(t, "unavailable", loadUsage(t, fixture.DB, task.ExecutionID).Coverage)
}
```

- [ ] **Step 2: 运行 telemetry/worker 测试并确认失败**

Run: `cd backend && go test ./internal/telemetry ./internal/worker -count=1`

Expected: FAIL，Provider 和 worker 尚未定义。

- [ ] **Step 3: 实现 TraceCostProvider 与可配置 Langfuse 读取模式**

```go
type TraceCostProvider interface {
    Observe(context.Context, ExecutionRef) (CostObservation, error)
}

type CostObservation struct {
    ObservedCost decimal.Decimal
    SelfReportedCost decimal.Decimal
    Coverage Coverage
    Provider string
    Cursor string
}
```

Langfuse Cloud 模式使用 Basic Auth 调用 `/api/public/v2/metrics`，按 execution、tenant、task、agent version 标签聚合 `totalCost`。自托管模式从配置读取兼容 API 路径和能力，若实例不支持成本读取则返回 `unavailable`；没有外部工具 trace 时标记 `partial`，HTTP/解析故障标记 `unavailable`。

- [ ] **Step 4: 实现 outbox 锁定、指数退避和幂等消费**

worker 使用 `FOR UPDATE SKIP LOCKED` 领取事件，通过 `attempts`、`claimed_until`、`published_at` 防止并发重复；同一 source cursor 更新 `execution_usage` 必须幂等。

- [ ] **Step 5: 实现可替换 RateLimiter、tenant+Agent 本地令牌桶与审计查询**

Application Service 依赖 `RateLimiter` 接口；MVP 提供 tenant+Agent 进程内令牌桶，并明确其不提供跨实例全局配额。生产部署可由 Redis 或 API Gateway 实现同一接口。限流错误统一返回 `RATE_LIMITED` 和 `retry_after_seconds`；审计查询只返回调用者可见 Task 的脱敏事件摘要。

- [ ] **Step 6: 运行故障、重放和限流测试**

Run: `cd backend && go test ./internal/telemetry ./internal/worker ./internal/ratelimit ./internal/application -run 'Langfuse|Outbox|Rate|Audit' -count=1`

Expected: PASS。

- [ ] **Step 7: 提交可观测性与运行组件**

```bash
git add backend/internal/telemetry backend/internal/worker backend/internal/ratelimit backend/internal/application/audit_queries.go
git commit -m "feat: observe execution cost asynchronously"
```

---

### Task 8: 组装服务和 React 只读观察页面

- [ ] **Completion gate: Task 8 runnable observer app**

**Files:**
- Create: `backend/cmd/agentguild-api/main.go`
- Create: `backend/internal/config/config.go`
- Create: `frontend/package.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/src/main.tsx`
- Create: `frontend/src/app/AppShell.tsx`
- Create: `frontend/src/api/client.ts`
- Create: `frontend/src/features/tasks/TaskList.tsx`
- Create: `frontend/src/features/tasks/TaskDetail.tsx`
- Create: `frontend/src/styles/tokens.css`
- Test: `frontend/src/features/tasks/TaskList.test.tsx`
- Test: `frontend/src/features/tasks/TaskDetail.test.tsx`
- Test: `frontend/e2e/task-observer.visual.spec.ts`

**Interfaces:**
- Consumes: REST `Envelope<TaskView>`、`Envelope<TaskPage>`、`Envelope<ExecutionView>`
- Produces: `/tasks` 列表和 `/tasks/:id` 详情

- [ ] **Step 1: 确认设计稿实现 brief**

执行 Product Design `get-context`，明确以 `docs/assets/agentguild-tasks.png` 为主参考：桌面端深色高密度布局，左侧 60px 图标导航，双层顶部上下文区，中间分组表格，右侧约 320px 详情面板；随后使用 `image-to-code` 指引实现。移动端将详情面板改为抽屉，表格改为横向可滚动列表，不新增设计稿中超出本 change 范围的 mutation 控件。

- [ ] **Step 2: 写轮询、cursor、deadline 和只读行为测试**

```tsx
it("renders lease and cost coverage without mutation controls", async () => {
  render(<TaskDetail taskId="task-1" />, { wrapper: testQueryClient() });
  expect(await screen.findByText("Lease: active")).toBeVisible();
  expect(screen.getByText("Cost coverage: partial")).toBeVisible();
  expect(screen.queryByRole("button", { name: /claim|accept|set status/i })).toBeNull();
});
```

- [ ] **Step 3: 初始化 React 工具链并确认组件测试失败**

Run: `cd frontend && npm install && npm test -- --run`

Expected: FAIL，任务组件尚未实现。

- [ ] **Step 4: 实现类型化 REST 客户端和安全轮询**

```ts
export type Envelope<T> = {
  data: T;
  meta: {
    server_time: string;
    resource_version: number;
    poll_after_seconds?: number;
    next_cursor?: string;
  };
};
```

TanStack Query 使用 `poll_after_seconds` 设置下一次轮询，不根据浏览器时间改变状态；cursor 原样传回服务端，不在前端解码。

- [ ] **Step 5: 实现设计稿对应的 Shell、主题和任务工作台**

在 `tokens.css` 定义设计 token：

```css
:root {
  color-scheme: dark;
  --ag-bg: #0d1115;
  --ag-panel: #12171c;
  --ag-panel-raised: #171d22;
  --ag-border: #293139;
  --ag-text: #f2f5f7;
  --ag-muted: #9ca7b0;
  --ag-accent: #25bce0;
  --ag-success: #48cf75;
  --ag-warning: #f2b43c;
  --ag-danger: #f05b5b;
  --ag-radius: 6px;
}
```

列表支持 status、type 和 cursor；详情展示 Task/Execution 状态、stage、progress、deadline、lease soft/hard expiry、成本 coverage 和审计摘要，不提供通用状态按钮。复用设计稿的状态点、进度条、分组标题和详情键值排版。

- [ ] **Step 6: 组装 Go 服务和功能开关**

`main.go` 组合 PostgreSQL Store、Application Service、REST、MCP、reaper、outbox 和 Langfuse adapter；`MCP_ENABLED`、`WEB_ENABLED`、`LANGFUSE_ENABLED` 分别控制适配层，关闭时不影响领域服务。

- [ ] **Step 7: 执行视觉截图比对**

以 1512×1064 viewport 截取 `/tasks`，与 `docs/assets/agentguild-tasks.png` 对照，验证信息层级、主列宽、右侧详情栏、状态色、密度和对齐；允许因当前只读范围缺少 mutation 按钮，但不得改变整体布局结构。移动端另以 390×844 验证详情抽屉、横向滚动和键盘焦点。

- [ ] **Step 8: 运行前后端构建和组件测试**

Run: `cd backend && go test ./... -count=1 && go build ./cmd/agentguild-api`

Run: `cd frontend && npm test -- --run && npm run build`

Expected: 所有命令 PASS。

- [ ] **Step 9: 提交可运行应用**

```bash
git add backend/cmd backend/internal/config frontend
git commit -m "feat: add task lifecycle observer application"
```

---

### Task 9: 完成端到端、恢复和验收测试

- [ ] **Completion gate: Task 9 end-to-end acceptance**

**Files:**
- Create: `backend/internal/acceptance/lifecycle_test.go`
- Create: `backend/internal/acceptance/mcp_recovery_test.go`
- Create: `frontend/e2e/task-observer.spec.ts`
- Modify: `Makefile`
- Modify: `openspec/changes/agent-task-lifecycle/tasks.md`

**Interfaces:**
- Verifies: Agent 轮询、断线重试、幂等恢复、非法迁移、deadline 和 REST/MCP 等价性

- [ ] **Step 1: 写完整 Agent 断线恢复验收**

```go
func TestAgentRecoversAfterHeartbeatResponseLoss(t *testing.T) {
    env := acceptance.Start(t)
    claimed := env.MCP.TaskClaim("task-1", "req-claim")
    first := env.MCP.HeartbeatDropResponse(claimed.ExecutionID, claimed.Generation, "req-heartbeat")
    replay := env.MCP.Heartbeat(claimed.ExecutionID, claimed.Generation, "req-heartbeat")
    require.Equal(t, first.StoredGeneration, replay.Generation)
    require.Equal(t, 1, env.CountAuditIntent("heartbeat"))
}
```

- [ ] **Step 2: 添加 deadline、非法状态和 100 并发客户端场景**

测试必须断言：deadline 后 heartbeat 返回 `DEADLINE_EXCEEDED`；非持有者只收到安全的 `FORBIDDEN/NOT_FOUND`；100 个 Claim 仅一个成功；宽限期内不能重领；旧 generation 被拒绝。

- [ ] **Step 3: 添加 Playwright 只读观察页验收**

```ts
test("observer follows server polling metadata", async ({ page }) => {
  await page.goto("/tasks/task-1");
  await expect(page.getByText("Running")).toBeVisible();
  await expect(page.getByText("10m lease")).toBeVisible();
  await expect(page.getByRole("button", { name: "Set status" })).toHaveCount(0);
});
```

- [ ] **Step 4: 配置统一验证命令**

```make
build:
	cd backend && go build ./...
	cd frontend && npm run build

test:
	cd backend && go test -race ./... -count=1
	cd frontend && npm test -- --run

verify: build test
	cd frontend && npm run test:e2e
```

- [ ] **Step 5: 运行完整验证**

Run: `docker compose up -d postgres && make verify`

Expected: Go race tests、PostgreSQL 集成测试、React/Vitest、Playwright 和两种 transport 契约全部 PASS。

- [ ] **Step 6: 对照 OpenSpec 勾选 12 项任务**

逐项核对 `tasks.md`：每个勾选项必须有对应测试或构建证据；不得仅因代码文件存在而勾选。

- [ ] **Step 7: 提交验收闭环**

```bash
git add backend/internal/acceptance frontend/e2e Makefile openspec/changes/agent-task-lifecycle/tasks.md
git commit -m "test: verify agent task lifecycle end to end"
```

## Self-Review Result

- Spec coverage：Task/Execution 状态、角色权限、单活动 Execution、10m+30s Lease、generation fencing、deadline、幂等、成本覆盖、MCP OAuth、八个工具、cursor、稳定错误、REST/MCP 等价、React 观察页和故障恢复均有对应任务。
- Placeholder scan：已通过；每个实现步骤都给出了明确文件、接口、命令和预期结果。
- Type consistency：`auth.Principal`、`application.Service`、`Envelope/Meta`、`TraceCostProvider` 和 `LeaseGeneration` 在生产者与消费者任务中命名一致。
- Scope boundary：成果上传、GitLab commit 验证、人工审核和声望仍保留给后续 change，本计划不实现 `submission_create` 或 `submission_get`。

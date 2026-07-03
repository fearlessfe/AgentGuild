---
change: agent-onboarding-and-identity
design-doc: docs/superpowers/specs/2026-07-03-agent-onboarding-and-identity-design.md
base-ref: bc6e3dd0ef0eeb81405479f161a1bbc1e2db2286
---

# Agent Onboarding and Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 AgentGuild Coding MVP 构建 Agent 身份、激活、访问控制和管理页面，支持企业人员预注册 Agent、一次性 Activation Token 激活、短期 JWT Access Token 授权、状态生命周期管理以及 React 管理端。

**Architecture:** 采用 Go 模块化单体，新增 `identity` 领域模块（Agent / AgentVersion / ActivationCredential / 审计事件），由 `identity/application` 统一编排授权、事务、审计和状态迁移；`auth` 模块复用并扩展 `agent-task-lifecycle` 的 `Principal` 和 `TokenVerifier`，新增 OIDC 会话适配和 Agent Access Token 签发；REST transport 分别服务人类管理端（OIDC cookie session）和 Agent 激活端（JWT bearer）；React 提供 Agents 管理页面。

**Tech Stack:** Go 1.26.4、PostgreSQL 18.4、pgx v5、chi、jwt/v5、React 19、TypeScript、Vite、TanStack Query、Vitest、Playwright。

## Global Constraints

- 所有持久化记录和查询必须携带 `tenant_id`；首版固定单一租户，但代码层面不允许绕过 tenant 隔离。
- Agent 状态机不得暴露通用 `set_status` 接口；只允许 `Register → PendingActivation → Active ↔ Suspended → Revoked` 的明确迁移。
- Activation Token 明文仅生成时返回一次；数据库只保存 SHA-256 哈希；重放、过期、不存在返回统一错误。
- Agent Access Token 使用 RS256，15 分钟有效期；首版不实现 Refresh Token，Agent 通过 `POST /v1/agents/me:refresh` 用当前有效 Token 换取新 Token。
- 人类 OIDC 登录建立服务端 Session（cookie）；管理员通过配置邮箱列表或 OIDC claim 判定。
- Transport 只负责认证上下文、Schema 转换和协议错误映射，不得直接访问 Repository。
- 不可见资源与无权限资源对非管理员返回一致错误；错误码至少包含 `FORBIDDEN`、`NOT_FOUND`、`INVALID_ARGUMENT`、`STATE_CONFLICT`、`TOKEN_EXPIRED`、`TOKEN_REVOKED`、`RATE_LIMITED`。
- 本 change 不实现 MCP 激活工具；首版激活走 REST，避免在模型上下文暴露一次性凭证。
- 本 change 不实现多租户运营后台、开放互联网自注册、复杂 Agent Version 晋级、集中 Token 撤销列表。
- `go.mod` 使用 `go 1.26.0` 和 `toolchain go1.26.4`；PostgreSQL 容器固定 `postgres:18.4`。

## File Map

```text
backend/
  internal/identity/domain/agent.go                  Agent 聚合、状态机和领域错误
  internal/identity/domain/agent_version.go          AgentVersion 不可变快照
  internal/identity/domain/activation_credential.go  ActivationCredential 单次消费凭证
  internal/identity/domain/audit.go                  identity_events 领域事件
  internal/identity/application/commands.go          命令服务：Register/Activate/Suspend/Revoke/Resume
  internal/identity/application/queries.go           查询服务：List/Get/GetActivationStatus
  internal/identity/application/policy.go            tenant/scope/repo/status 授权策略
  internal/identity/application/contracts.go         命令、查询、响应 DTO
  internal/identity/postgres/agent_repository.go     Agent / Version / Credential 持久化
  internal/identity/postgres/audit_repository.go     身份审计事件持久化
  internal/auth/oidc.go                              OIDC provider 适配与 Session
  internal/auth/token_issuer.go                      Agent Access Token 签发
  internal/auth/token_verifier.go                    JWT RS256 校验（兼容 task-lifecycle）
  internal/auth/principal.go                         Principal、Scope 和资源授权
  internal/transport/rest/identity_router.go         人类管理端路由
  internal/transport/rest/agent_self_router.go       Agent 激活端路由
  internal/transport/rest/session_middleware.go      OIDC Session cookie middleware
  internal/transport/rest/openapi.yaml               REST 契约
  migrations/000002_agent_identity.up.sql            identity 相关表、约束、索引
  migrations/000002_agent_identity.down.sql          回滚
frontend/
  src/features/agents/AgentList.tsx                  Agents 列表页
  src/features/agents/AgentRegister.tsx              注册 Agent 表单
  src/features/agents/AgentTokenReveal.tsx           一次性 Activation Token 展示
  src/features/agents/AgentDetail.tsx                Agent 详情与状态管理
  src/features/agents/agents.api.ts                  Agents REST 客户端
  src/features/agents/agents.test.tsx                组件测试
  e2e/agent-onboarding.spec.ts                       Playwright 验收测试
openspec/changes/agent-onboarding-and-identity/tasks.md  任务边界勾选更新
```

---

## Task 1: 建立 identity 领域状态机与数据模型

- [x] **Completion gate: Task 1 identity domain**

**Files:**
- Create: `backend/internal/identity/domain/agent.go`
- Create: `backend/internal/identity/domain/agent_version.go`
- Create: `backend/internal/identity/domain/activation_credential.go`
- Create: `backend/internal/identity/domain/audit.go`
- Create: `backend/internal/identity/domain/errors.go`
- Test: `backend/internal/identity/domain/agent_test.go`
- Test: `backend/internal/identity/domain/activation_credential_test.go`

**Interfaces:**
- Produces: `domain.NewAgent(...) (*Agent, error)`
- Produces: `agent.Register()`, `agent.Activate(...)`, `agent.Suspend()`, `agent.Resume()`, `agent.Revoke()`
- Produces: `domain.NewAgentVersion(...) (*AgentVersion, error)`
- Produces: `domain.NewActivationCredential(...) (*ActivationCredential, string, error)` — 返回领域对象与一次性明文 Token
- Produces: `credential.Consume(plaintext string, now time.Time) error`
- Produces: `domain.IdentityEvent{TenantID, AgentID, ActorType, ActorID, Intent, FromState, ToState, Reason, Payload}`
- Produces: `domain.Error{Code, Message, Field}`

- [ ] **Step 1: 写状态迁移失败测试**

```go
func TestAgentCannotActivateTwice(t *testing.T) {
    agent, _ := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", []string{"tasks:read"}, nil)
    cred, token, _ := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
    require.NoError(t, agent.Activate(cred, manifest(), token, time.Now()))
    _, _, err := agent.Activate(cred, manifest(), token, time.Now())
    require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestRevokedAgentCannotResume(t *testing.T) {
    agent, _ := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
    agent.Revoke("admin-1", "compromised", time.Now())
    err := agent.Resume("admin-1", time.Now())
    require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestConsumedCredentialReplayFails(t *testing.T) {
    cred, token, _ := domain.NewActivationCredential("agent-1", "tenant-1", time.Hour)
    require.NoError(t, cred.Consume(token, time.Now()))
    err := cred.Consume(token, time.Now())
    require.ErrorIs(t, err, domain.ErrTokenExpired)
}
```

- [ ] **Step 2: 运行领域测试并确认失败**

Run: `cd backend && go test ./internal/identity/domain -run 'TestAgent|TestConsumed' -count=1`

Expected: FAIL，原因是 `identity/domain` 类型尚未定义。

- [ ] **Step 3: 实现 Agent 状态机和明确意图方法**

```go
const (
    AgentPendingActivation = "pending_activation"
    AgentActive            = "active"
    AgentSuspended         = "suspended"
    AgentRevoked           = "revoked"
)

type Agent struct {
    ID, TenantID, OwnerID, OwnerEmail, Team string
    Name, Description string
    Status string
    CurrentVersionID string
    Scopes []string
    RepoScope []string
    BudgetCents int64
    BudgetCurrency string
    LastSeenAt *time.Time
    CreatedAt, UpdatedAt time.Time
}
```

状态机只通过 `Activate`、`Suspend`、`Resume`、`Revoke` 方法迁移；每个方法返回 `*IdentityEvent` 并校验当前状态、Actor 权限和领域不变量。

- [ ] **Step 4: 实现 ActivationCredential 单次消费与过期**

```go
type ActivationCredential struct {
    ID, TenantID, AgentID string
    Hash []byte
    Status string
    ExpiresAt, ConsumedAt *time.Time
    CreatedAt time.Time
}
```

明文 Token 使用 `crypto/rand` 生成 32-byte URL-safe base64；`Consume` 使用 `subtle.ConstantTimeCompare` 比较哈希，消费后原子设置 `status=consumed` 并记录时间；过期或已消费 Token 返回统一 `ErrTokenExpired`，不泄露是否存在。

- [ ] **Step 5: 实现 AgentVersion 不可变快照**

```go
type AgentVersion struct {
    ID, TenantID, AgentID string
    VersionNumber int
    Runtime, Model string
    Capabilities []string
    ConfigFingerprint string
    CreatedAt time.Time
}
```

首版只创建 `version_number = 1` 的初始版本；Agent 聚合根保存 `current_version_id`。

- [ ] **Step 6: 添加表驱动合法/非法迁移和凭证重放测试**

```go
func TestAgentStateMachine(t *testing.T) {
    cases := []struct{ from, intent, to string; wantErr error }{...}
    for _, tc := range cases {
        agent := agentInState(tc.from)
        err := applyIntent(agent, tc.intent)
        if tc.wantErr != nil { require.ErrorIs(t, err, tc.wantErr); continue }
        require.Equal(t, tc.to, agent.Status)
    }
}
```

- [ ] **Step 7: 运行领域测试和格式检查**

Run: `cd backend && gofmt -w internal/identity/domain && go test ./internal/identity/domain -count=1`

Expected: PASS。

- [ ] **Step 8: 提交 identity 领域骨架**

```bash
git add backend/internal/identity/domain
git commit -m "feat: define agent onboarding identity domain"
```

---

## Task 2: 建立 PostgreSQL Schema、tenant 上下文和 Repository

- [x] **Completion gate: Task 2 identity persistence**

**Files:**
- Create: `backend/migrations/000002_agent_identity.up.sql`
- Create: `backend/migrations/000002_agent_identity.down.sql`
- Create: `backend/internal/identity/application/ports.go`
- Create: `backend/internal/identity/postgres/store.go`
- Create: `backend/internal/identity/postgres/agent_repository.go`
- Create: `backend/internal/identity/postgres/audit_repository.go`
- Test: `backend/internal/identity/postgres/repository_test.go`

**Interfaces:**
- Consumes: `domain.Agent`、`domain.AgentVersion`、`domain.ActivationCredential`、`domain.IdentityEvent`
- Produces: `identity.Store.WithTx(ctx, func(identity.Tx) error) error`
- Produces: `identity.Tx` 和 Agent/Version/Credential/Audit repository ports；ports 不依赖 PostgreSQL
- Produces: `identity.AgentRepository`、`VersionRepository`、`CredentialRepository`、`AuditRepository`

- [ ] **Step 1: 写迁移约束与 tenant 隔离集成测试**

```go
func TestAgentTenantIsolationCannotBeBypassed(t *testing.T) {
    db := testdb.StartPostgres(t)
    insertAgent(t, db, "tenant-1", "agent-1")
    repo := postgres.NewAgentRepository(db)
    _, err := repo.GetByID(ctx, "tenant-2", "agent-1")
    require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestConcurrentActivationConsumesOnlyOneToken(t *testing.T) {
    db := testdb.StartPostgres(t)
    seedPendingAgent(t, db, "agent-1")
    seedCredential(t, db, "agent-1", tokenHash)
    results := runConcurrent(50, func(i int) error {
        tx, _ := store.BeginTx(ctx)
        defer tx.Rollback(ctx)
        cred, _ := tx.Credentials().GetPending(ctx, "tenant-1", "agent-1")
        err := cred.Consume(token, now)
        if err != nil { return err }
        return tx.Credentials().Save(ctx, cred)
    })
    require.Equal(t, 1, countNil(results))
}
```

- [ ] **Step 2: 运行测试并确认迁移缺失**

Run: `cd backend && go test ./internal/identity/postgres -run TestAgentTenantIsolation -count=1`

Expected: FAIL，原因是迁移或测试数据库辅助代码不存在。

- [ ] **Step 3: 创建多租户表、约束和索引**

迁移创建以下表，所有主键/唯一约束必须包含 `tenant_id`：

```sql
CREATE TABLE agents (
    id TEXT NOT NULL, tenant_id TEXT NOT NULL,
    owner_id TEXT NOT NULL, owner_email TEXT NOT NULL, team TEXT,
    name TEXT NOT NULL, description TEXT,
    status TEXT NOT NULL, current_version_id TEXT,
    scopes TEXT[] NOT NULL DEFAULT '{}', repo_scope TEXT[] NOT NULL DEFAULT '{}',
    budget_cents BIGINT NOT NULL DEFAULT 0, budget_currency TEXT,
    last_seen_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE agent_versions (
    id TEXT NOT NULL, tenant_id TEXT NOT NULL, agent_id TEXT NOT NULL,
    version_number INT NOT NULL, runtime TEXT, model TEXT,
    capabilities TEXT[] NOT NULL DEFAULT '{}', config_fingerprint TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, agent_id, version_number)
);

CREATE TABLE activation_credentials (
    id TEXT NOT NULL, tenant_id TEXT NOT NULL, agent_id TEXT NOT NULL,
    hash BYTEA NOT NULL, status TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL, consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, agent_id)
);

CREATE TABLE identity_events (
    id BIGSERIAL PRIMARY KEY,
    tenant_id TEXT NOT NULL, agent_id TEXT NOT NULL,
    actor_type TEXT NOT NULL, actor_id TEXT NOT NULL,
    intent TEXT NOT NULL, from_state TEXT, to_state TEXT,
    reason TEXT, payload JSONB, created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_identity_events_agent ON identity_events (tenant_id, agent_id, created_at DESC);
```

- [ ] **Step 4: 实现事务内数据库时间和条件更新**

复用 `internal/postgres/store.go` 的数据库时间机制或新增 `identity/postgres/store.go`；Credential 消费和 Agent 状态更新必须使用条件 SQL，例如：

```sql
UPDATE activation_credentials
SET status='consumed', consumed_at=$1
WHERE tenant_id=$2 AND agent_id=$3 AND status='pending' AND expires_at>$1
RETURNING id;
```

- [ ] **Step 5: 实现 Repository 和 Audit 追加**

Agent 更新、Version 创建、Credential 消费和审计事件写入同一事务；审计事件只追加，不得修改或删除。

- [ ] **Step 6: 运行 tenant 隔离、并发消费和审计测试**

Run: `cd backend && go test ./internal/identity/postgres -count=1`

Expected: PASS。

- [ ] **Step 7: 提交 identity 持久化层**

```bash
git add backend/migrations/000002_agent_identity.* backend/internal/identity/postgres backend/internal/identity/application/ports.go
git commit -m "feat: persist agent identity atomically"
```

---

## Task 3: 实现 identity Application Service 与授权策略

- [x] **Completion gate: Task 3 identity application service**

**Files:**
- Create: `backend/internal/identity/application/contracts.go`
- Create: `backend/internal/identity/application/commands.go`
- Create: `backend/internal/identity/application/queries.go`
- Create: `backend/internal/identity/application/policy.go`
- Test: `backend/internal/identity/application/service_test.go`

**Interfaces:**
- Produces: `application.IdentityService.RegisterAgent`、`ActivateAgent`、`SuspendAgent`、`ResumeAgent`、`RevokeAgent`、`IssueAccessToken`、`AgentHeartbeat`
- Produces: `application.IdentityService.ListAgents`、`GetAgent`、`GetActivationStatus`
- Produces: `application.Policy.Require(principal, scope, resource)` 和 `Policy.RequireAgentStatus(agent, allowed...)`

- [ ] **Step 1: 写权限、状态和 tenant 边界测试**

```go
func TestNonOwnerCannotSuspendOthersAgent(t *testing.T) {
    svc := newServiceFixture()
    admin := auth.HumanPrincipal{TenantID: "tenant-1", OwnerID: "admin-1", IsAdmin: false}
    _, err := svc.SuspendAgent(ctx, admin, application.SuspendAgent{AgentID: "agent-owned-by-other"})
    require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestSuspendedAgentCannotRefreshToken(t *testing.T) {
    svc := newServiceFixture()
    agent := seedActiveAgent(t, svc)
    svc.SuspendAgent(ctx, owner(agent), application.SuspendAgent{AgentID: agent.ID})
    _, err := svc.IssueAccessToken(ctx, agentPrincipal(agent))
    require.ErrorIs(t, err, domain.ErrStateConflict)
}
```

- [ ] **Step 2: 运行 Application Service 测试并确认失败**

Run: `cd backend && go test ./internal/identity/application -count=1`

Expected: FAIL，原因是 Service 尚未定义。

- [ ] **Step 3: 定义命令、查询和响应 DTO**

```go
type RegisterAgent struct {
    RequestID, Name, Description, Team string
    Scopes, RepoScope []string
    BudgetCents int64
    BudgetCurrency string
}

type ActivateAgent struct {
    Token string
    Runtime, Model string
    Capabilities []string
    ConfigFingerprint string
}

type AgentView struct {
    ID, TenantID, Name, Description, Status, Team string
    OwnerID, OwnerEmail string
    Scopes, RepoScope []string
    CurrentVersion *AgentVersionView
    LastSeenAt *time.Time
    CreatedAt, UpdatedAt time.Time
}
```

- [ ] **Step 4: 实现授权 → 加载聚合 → 领域方法 → 持久化/审计 → 返回的固定命令管线**

```go
func (s *IdentityService) ActivateAgent(ctx context.Context, cmd ActivateAgent) (Envelope[AccessTokenView], error) {
    return s.store.WithTx(ctx, func(tx identity.Tx) error {
        now := tx.Now(ctx)
        cred, err := tx.Credentials().GetPendingByPlaintext(ctx, cmd.Token)
        if err != nil { return maskCredentialError(err) }
        agent, err := tx.Agents().GetByID(ctx, cred.TenantID, cred.AgentID)
        if err != nil { return err }
        version, err := domain.NewAgentVersion(agent.ID, agent.TenantID, 1, cmd.Runtime, cmd.Model, cmd.Capabilities, cmd.ConfigFingerprint)
        if err != nil { return err }
        evt, err := agent.Activate(cred, version, cmd.Token, now)
        if err != nil { return err }
        if err := tx.Credentials().Save(ctx, cred); err != nil { return err }
        if err := tx.Versions().Save(ctx, version); err != nil { return err }
        if err := tx.Agents().Save(ctx, agent); err != nil { return err }
        if err := tx.Audit().Append(ctx, evt); err != nil { return err }
        token, err := s.tokenIssuer.Issue(agent, version, now)
        if err != nil { return err }
        return respond(AccessTokenView{...})
    })
}
```

- [ ] **Step 5: 实现 Scope 与仓库范围校验策略**

```go
func (p *Policy) RequireRepoScope(principal auth.Principal, repo string) error {
    for _, allowed := range principal.RepoScope {
        if matchRepo(allowed, repo) { return nil }
    }
    return domain.ErrForbidden
}
```

- [ ] **Step 6: 运行 Application Service 测试**

Run: `cd backend && go test ./internal/identity/application -count=1`

Expected: PASS。

- [ ] **Step 7: 提交 identity 应用层**

```bash
git add backend/internal/identity/application
git commit -m "feat: add shared identity application service"
```

---

## Task 4: 实现 OIDC 会话、Agent Access Token 签发与校验

- [x] **Completion gate: Task 4 auth and tokens**

**Files:**
- Create: `backend/internal/auth/oidc.go`
- Create: `backend/internal/auth/session.go`
- Create: `backend/internal/auth/token_issuer.go`
- Modify: `backend/internal/auth/principal.go`
- Modify: `backend/internal/auth/token_verifier.go`（如 task-lifecycle 已存在）
- Test: `backend/internal/auth/oidc_test.go`
- Test: `backend/internal/auth/token_test.go`

**Interfaces:**
- Produces: `auth.OIDCProvider.BeginAuthURL(state) string`
- Produces: `auth.OIDCProvider.Exchange(ctx, code) (*Session, error)`
- Produces: `auth.Session{TenantID, OwnerID, OwnerEmail, IsAdmin}`
- Produces: `auth.TokenIssuer.Issue(agent, version, now) (string, error)`
- Produces: `auth.TokenVerifier.Verify(ctx, rawToken) (auth.Principal, error)` — 兼容 task-lifecycle

- [ ] **Step 1: 写 OIDC 回调、Session 创建和 Token 校验测试**

```go
func TestOIDCCallbackCreatesSessionWithAdminClaim(t *testing.T) {
    provider := newMockOIDC(oidcClaims{sub: "owner-1", email: "owner@example.com", admin: true})
    sess, err := provider.Exchange(ctx, "code-1")
    require.NoError(t, err)
    require.True(t, sess.IsAdmin)
    require.Equal(t, "owner-1", sess.OwnerID)
}

func TestTokenVerifierRejectsExpiredAgentToken(t *testing.T) {
    verifier := auth.NewRS256Verifier(publicKey)
    token := issueExpiredToken(issuer, "agent-1")
    _, err := verifier.Verify(ctx, token)
    require.ErrorIs(t, err, auth.ErrTokenExpired)
}
```

- [ ] **Step 2: 运行 auth 测试并确认失败**

Run: `cd backend && go test ./internal/auth -count=1`

Expected: FAIL，原因是 OIDC 适配和 TokenIssuer 尚未定义。

- [ ] **Step 3: 实现通用 OIDC provider 适配与 Session**

配置字段：`issuer`、`client_id`、`client_secret`、`redirect_uri`、`scopes`、`admin_claim`、`admin_emails`。
默认 claim 映射：`sub` → owner ID，`email` → owner email；`admin_claim` 或 `admin_emails` 判定管理员。
Session 使用安全 httpOnly cookie，携带 `tenant_id`、`owner_id`、`owner_email`、`is_admin`。

- [ ] **Step 4: 扩展 Principal 以支持人类和 Agent 主体**

```go
type Principal struct {
    TenantID string
    Type     string // "human" | "agent"
    OwnerID  string // human
    AgentID  string // agent
    AgentVersionID string
    Scopes   []string
    RepoScope []string
    IsAdmin  bool
}
```

- [ ] **Step 5: 实现 RS256 Agent Access Token 签发**

```go
func (i *TokenIssuer) Issue(agent *domain.Agent, version *domain.AgentVersion, now time.Time) (string, error) {
    claims := jwt.MapClaims{
        "tenant_id":        agent.TenantID,
        "agent_id":         agent.ID,
        "agent_version_id": version.ID,
        "scopes":           agent.Scopes,
        "exp":              now.Add(15 * time.Minute).Unix(),
    }
    return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(i.privateKey)
}
```

签名密钥通过配置指定 RSA 私钥路径或 PEM 内容；公钥用于 `TokenVerifier` 校验。

- [ ] **Step 6: 运行 OIDC 与 Token 测试**

Run: `cd backend && go test ./internal/auth -count=1`

Expected: PASS。

- [ ] **Step 7: 提交认证模块**

```bash
git add backend/internal/auth
git commit -m "feat: add OIDC session and agent access token issuance"
```

---

## Task 5: 暴露 REST API（人类管理端 + Agent 激活端）

- [x] **Completion gate: Task 5 REST API**

**Files:**
- Create: `backend/internal/transport/rest/identity_router.go`
- Create: `backend/internal/transport/rest/agent_self_router.go`
- Create: `backend/internal/transport/rest/session_middleware.go`
- Modify: `backend/internal/transport/rest/router.go`（如 task-lifecycle 已存在，挂载新路由）
- Modify: `backend/internal/transport/rest/openapi.yaml`
- Test: `backend/internal/transport/rest/identity_router_test.go`
- Test: `backend/internal/transport/rest/agent_self_router_test.go`

**Interfaces:**
- Produces: `GET /oauth/oidc/login`、`GET /oauth/oidc/callback`
- Produces: `POST /v1/agents`、`GET /v1/agents`、`GET /v1/agents/:id`
- Produces: `POST /v1/agents/:id:suspend`、`POST /v1/agents/:id:resume`、`POST /v1/agents/:id:revoke`
- Produces: `GET /v1/agents/:id:token`
- Produces: `POST /v1/agents/me:activate`、`POST /v1/agents/me:refresh`、`POST /v1/agents/me:heartbeat`、`GET /v1/agents/me`

- [ ] **Step 1: 写 REST 状态码、Session 和不可见资源测试**

```go
func TestRegisterAgentRequiresSession(t *testing.T) {
    res := postJSON(t, server, "/v1/agents", `{}`, "")
    require.Equal(t, http.StatusUnauthorized, res.Code)
}

func TestGetAgentHidesOthersFromNonOwner(t *testing.T) {
    res := get(t, server, "/v1/agents/agent-owned-by-other", sessionFor("owner-1"))
    require.Equal(t, http.StatusNotFound, res.Code)
    require.NotContains(t, res.Body.String(), "owner-other")
}
```

- [ ] **Step 2: 运行 REST 测试并确认失败**

Run: `cd backend && go test ./internal/transport/rest -run 'TestRegisterAgent|TestGetAgent' -count=1`

Expected: FAIL，路由和 Session middleware 不存在。

- [ ] **Step 3: 实现 OIDC Session middleware 和人类路由**

Session middleware 从 cookie 解析 `auth.Session`，注入 request context；人类路由调用 `IdentityService` 的命令/查询，将领域错误映射为稳定 HTTP 状态码：

| 领域错误 | HTTP |
|---|---|
| `FORBIDDEN` / `NOT_FOUND` | 404（非管理员） |
| `INVALID_ARGUMENT` | 400 |
| `STATE_CONFLICT` | 409 |
| `TOKEN_EXPIRED` / `TOKEN_REVOKED` | 401 |
| `RATE_LIMITED` | 429 with `Retry-After` |

- [ ] **Step 4: 实现 Agent 自服务路由**

`/v1/agents/me:*` 使用 bearer token 认证；`activate` 不需要已有 Token，`refresh`/`heartbeat`/`me` 需要有效 Agent Access Token。
`activate` 消费 Token 并返回新 Access Token；`refresh` 校验当前 Token 后签发新 Token。

- [ ] **Step 5: 编写并校验 OpenAPI 契约**

Run: `cd backend && go run github.com/getkin/kin-openapi/cmd/validate@latest internal/transport/rest/openapi.yaml`

Expected: `openapi document is valid`。

- [ ] **Step 6: 运行 REST 测试**

Run: `cd backend && go test ./internal/transport/rest -count=1`

Expected: PASS。

- [ ] **Step 7: 提交 REST 接口**

```bash
git add backend/internal/transport/rest
git commit -m "feat: expose agent identity REST API"
```

---

## Task 6: 提供 Agent 接入体验（skill.md、well-known 和激活 API）

- [ ] **Completion gate: Task 6 agent onboarding experience**

**Files:**
- Create: `/skill.md`
- Create: `backend/internal/transport/rest/well_known.go`
- Modify: `backend/internal/transport/rest/openapi.yaml`
- Test: `backend/internal/transport/rest/well_known_test.go`

**Interfaces:**
- Produces: `GET /.well-known/agentguild`
- Produces: `GET /v1/agents/me:activate` 文档与示例

- [ ] **Step 1: 写 well-known 端点测试**

```go
func TestWellKnownExposesActivationEndpoint(t *testing.T) {
    res := get(t, server, "/.well-known/agentguild")
    require.Equal(t, http.StatusOK, res.Code)
    require.Contains(t, res.Body.String(), "/v1/agents/me:activate")
}
```

- [ ] **Step 2: 运行测试并确认失败**

Run: `cd backend && go test ./internal/transport/rest -run TestWellKnown -count=1`

Expected: FAIL，well-known handler 不存在。

- [ ] **Step 3: 实现 well-known 元数据**

```json
{
  "version": "0.1.0",
  "activation_url": "https://api.agentguild.dev/v1/agents/me:activate",
  "refresh_url": "https://api.agentguild.dev/v1/agents/me:refresh",
  "heartbeat_url": "https://api.agentguild.dev/v1/agents/me:heartbeat",
  "scopes": ["tasks:read", "tasks:execute", "tasks:publish"]
}
```

- [ ] **Step 4: 编写 `/skill.md` 激活指南**

`/skill.md` 包含：
- 如何获取 Activation Token（由企业管理员在 React 页面注册 Agent 后一次性展示）。
- 如何调用 `POST /v1/agents/me:activate` 提交 manifest。
- 如何使用返回的 Access Token 调用 `tasks:*` API。
- Token 15 分钟有效期和 `me:refresh` 续期说明。
- 安全提示：不要记录 Activation Token。

- [ ] **Step 5: 运行 well-known 和文档测试**

Run: `cd backend && go test ./internal/transport/rest -run 'TestWellKnown|TestSkill' -count=1`

Expected: PASS。

- [ ] **Step 6: 提交接入体验**

```bash
git add /skill.md backend/internal/transport/rest/well_known.go
git commit -m "feat: add agent onboarding metadata and skill guide"
```

---

## Task 7: 实现 React Agents 管理页面

- [ ] **Completion gate: Task 7 React agents UI**

**Files:**
- Create: `frontend/src/features/agents/agents.api.ts`
- Create: `frontend/src/features/agents/AgentList.tsx`
- Create: `frontend/src/features/agents/AgentRegister.tsx`
- Create: `frontend/src/features/agents/AgentTokenReveal.tsx`
- Create: `frontend/src/features/agents/AgentDetail.tsx`
- Create: `frontend/src/features/agents/agents.types.ts`
- Test: `frontend/src/features/agents/agents.test.tsx`
- Test: `frontend/e2e/agent-onboarding.spec.ts`

**Interfaces:**
- Consumes: REST `Envelope<AgentView>`、`Envelope<AgentPage>`、`Envelope<AccessTokenView>`
- Produces: `/agents` 列表、`/agents/new` 注册、`/agents/:id` 详情

- [ ] **Step 1: 写组件行为测试**

```tsx
it("reveals activation token only once after registration", async () => {
  render(<AgentRegister onRegistered={mockReveal} />, { wrapper: testQueryClient() });
  await userEvent.type(screen.getByLabelText(/name/i), "Code Review Bot");
  await userEvent.click(screen.getByRole("button", { name: /register/i }));
  await waitFor(() => expect(mockReveal).toHaveBeenCalledWith(expect.objectContaining({ token: expect.any(String) })));
});

it("hides mutation controls for revoked agents", async () => {
  render(<AgentDetail agentId="agent-1" />, { wrapper: testQueryClient() });
  await waitFor(() => expect(screen.getByText(/revoked/i)).toBeVisible());
  expect(screen.queryByRole("button", { name: /suspend|resume/i })).toBeNull();
});
```

- [ ] **Step 2: 运行前端测试并确认失败**

Run: `cd frontend && npm test -- --run`

Expected: FAIL，Agents 组件尚未实现。

- [ ] **Step 3: 实现类型化 REST 客户端**

```ts
export type Agent = {
  id: string;
  name: string;
  description?: string;
  status: "pending_activation" | "active" | "suspended" | "revoked";
  team?: string;
  owner_email: string;
  scopes: string[];
  last_seen_at?: string;
  created_at: string;
};

export type RegisterAgentRequest = {
  name: string;
  description?: string;
  team?: string;
  scopes: string[];
  repo_scope?: string[];
  budget_cents?: number;
  budget_currency?: string;
};
```

- [ ] **Step 4: 实现列表、注册、Token 展示和详情页**

- 列表：展示名称、状态、owner、team、最后在线时间、scope 摘要；支持按状态筛选。
- 注册：表单包含名称、描述、team、scope、repo 范围、预算；提交成功后调用 `onRegistered` 展示一次性 Token。
- Token 展示：弹窗展示 Activation Token，提供复制按钮；不持久化在前端 state 或 localStorage。
- 详情：展示 Agent 完整信息；提供暂停、恢复、撤销按钮；撤销需二次确认。

- [ ] **Step 5: 复用主题与导航**

复用 `tokens.css` 的深色主题、状态色和间距；状态点颜色：`pending_activation` 黄色、`active` 绿色、`suspended` 橙色、`revoked` 红色。

- [ ] **Step 6: 运行组件测试**

Run: `cd frontend && npm test -- --run`

Expected: PASS。

- [ ] **Step 7: 提交 React Agents 页面**

```bash
git add frontend/src/features/agents frontend/e2e/agent-onboarding.spec.ts
git commit -m "feat: add React agents management page"
```

---

## Task 8: 组装服务、配置与端到端验收

- [ ] **Completion gate: Task 8 runnable onboarding app**

**Files:**
- Modify: `backend/cmd/agentguild-api/main.go`
- Modify: `backend/internal/config/config.go`
- Modify: `Makefile`
- Create: `backend/internal/acceptance/identity_test.go`
- Modify: `openspec/changes/agent-onboarding-and-identity/tasks.md`

**Interfaces:**
- Verifies: 注册 → 激活 → refresh → heartbeat → 暂停 → 撤销 完整链路
- Verifies: 凭证泄露模拟、重放攻击、越权访问、审计追溯

- [ ] **Step 1: 写完整激活链路验收测试**

```go
func TestAgentActivationAndLifecycle(t *testing.T) {
    env := acceptance.Start(t)
    registered := env.API.RegisterAgent(ownerSession(), application.RegisterAgent{Name: "Review Bot", Scopes: []string{"tasks:read"}})
    require.NotEmpty(t, registered.ActivationToken)

    activated := env.API.ActivateAgent(registered.ActivationToken, manifest())
    require.NotEmpty(t, activated.AccessToken)

    env.API.Heartbeat(activated.AccessToken)
    refreshed := env.API.RefreshToken(activated.AccessToken)
    require.NotEmpty(t, refreshed.AccessToken)

    env.API.SuspendAgent(ownerSession(), registered.ID)
    _, err := env.API.RefreshToken(refreshed.AccessToken)
    require.ErrorIs(t, err, domain.ErrStateConflict)
}
```

- [ ] **Step 2: 写安全验收测试**

```go
func TestActivationTokenReplayFails(t *testing.T) {
    env := acceptance.Start(t)
    token := env.API.RegisterAgent(ownerSession(), application.RegisterAgent{Name: "Bot"}).ActivationToken
    env.API.ActivateAgent(token, manifest())
    _, err := env.API.ActivateAgent(token, manifest())
    require.ErrorIs(t, err, domain.ErrTokenExpired)
}

func TestCredentialHashNotLeaked(t *testing.T) {
    env := acceptance.Start(t)
    res := env.API.GetActivationStatus(ownerSession(), "agent-1")
    require.Empty(t, res.TokenPlaintext)
    require.Equal(t, "pending", res.Status)
}
```

- [ ] **Step 3: 组装 Go 服务与配置**

`main.go` 组合 PostgreSQL Store、Identity Application Service、OIDC Provider、TokenIssuer、REST 路由、Session middleware；配置包含 OIDC provider、RSA 密钥路径、管理员邮箱/claim、cookie 密钥。

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

Expected: Go race tests、PostgreSQL 集成测试、React/Vitest、Playwright 和验收测试全部 PASS。

- [ ] **Step 6: 对照 OpenSpec 勾选任务**

逐项核对 `openspec/changes/agent-onboarding-and-identity/tasks.md`：每个勾选项必须有对应测试或构建证据；不得仅因代码文件存在而勾选。

- [ ] **Step 7: 提交验收闭环**

```bash
git add backend/cmd backend/internal/config backend/internal/acceptance Makefile openspec/changes/agent-onboarding-and-identity/tasks.md
git commit -m "test: verify agent onboarding and identity end to end"
```

## Self-Review Result

- Spec coverage：Agent 状态机、ActivationCredential 单次消费、AgentVersion 快照、OIDC Session、Agent Access Token、Scope/repo 授权、heartbeat、暂停/恢复/撤销、React Agents 页面、well-known、skill.md、凭证泄露/重放/越权/审计验收均有对应任务。
- Placeholder scan：已通过；每个实现步骤都给出了明确文件、接口、命令和预期结果。
- Type consistency：`auth.Principal`、`identity.ApplicationService`、`Envelope`、`AgentView`、`AccessTokenView` 在生产者与消费者任务中命名一致。
- Scope boundary：本 change 不实现 MCP 激活工具、多租户运营后台、开放互联网自注册、复杂版本晋级、集中 Token 撤销列表和 Refresh Token。

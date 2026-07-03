# Task 3 Report: identity Application Service 与授权策略

## What I implemented

- 新增 `backend/internal/identity/application` 的命令、查询、响应 DTO：
  - `RegisterAgent`, `ActivateAgent`, `SuspendAgent`, `ResumeAgent`, `RevokeAgent`, `IssueAccessToken`, `AgentHeartbeat`
  - `ListAgents`, `GetAgent`, `GetActivationStatus`
  - `AgentView`, `AgentVersionView`, `AccessTokenView`, `RegisterAgentResponse`, `ActivationStatusView`, `AgentPage`
- 新增 `IdentityService`：
  - `RegisterAgent`
  - `ActivateAgent`
  - `SuspendAgent`
  - `ResumeAgent`
  - `RevokeAgent`
  - `IssueAccessToken`
  - `AgentHeartbeat`
  - `ListAgents`
  - `GetAgent`
  - `GetActivationStatus`
- 新增应用层 `Policy`：
  - `Policy.Require(principal, scope, resource)`
  - `Policy.RequireAgentStatus(agent, allowed...)`
  - owner/admin/agent-self tenant 边界检查
  - repo scope 支持精确匹配、`*` 和 `owner/*` 前缀匹配
- 新增应用层 `TokenIssuer` 小接口，避免 transport/auth 签发细节进入 identity application。
- 新增 application service 测试，覆盖：
  - 非 owner 不可 suspend 他人 agent
  - admin 可在同 tenant suspend 并写 audit
  - suspended agent 不可 issue/refresh access token
  - tenant boundary 隐藏外租户 agent
  - agent-self 只能读取自己，不能读取 peer
  - scope/repo/status policy 检查

## Tests run and results

- `cd backend && go test ./internal/identity/application -count=1`
  - PASS: `ok agentguild.dev/agentguild/backend/internal/identity/application`
- `cd backend && go test ./internal/identity/... -count=1`
  - PASS: application/domain/postgres 全部通过
- `cd backend && go test ./... -count=1`
  - PASS: 后端全部包通过
- `cd backend && gofmt -w internal/identity/application/...`
  - PASS: Go 文件已格式化
- `git diff --check`
  - PASS: 无 whitespace 错误

## TDD RED/GREEN evidence

RED command:

```bash
cd backend && go test ./internal/identity/application -count=1
```

RED result summary:

```text
FAIL agentguild.dev/agentguild/backend/internal/identity/application [build failed]
undefined: application.Principal
undefined: application.SuspendAgent
undefined: application.IdentityService
undefined: application.AccessTokenView
undefined: application.AgentListQuery
```

This failed for the expected reason: Task 3 application service, DTOs, policy, and token contract did not exist yet.

GREEN command:

```bash
cd backend && go test ./internal/identity/application -count=1
```

GREEN result summary:

```text
ok agentguild.dev/agentguild/backend/internal/identity/application 0.911s
```

## Files changed

- `backend/internal/identity/application/contracts.go`
- `backend/internal/identity/application/commands.go`
- `backend/internal/identity/application/queries.go`
- `backend/internal/identity/application/policy.go`
- `backend/internal/identity/application/service_test.go`
- `.superpowers/sdd/task-3-report.md`

## Self-review findings/concerns

- Concern: `ActivateAgent` needs a plaintext activation-token lookup. Task 2's current `CredentialRepository` port only supports `GetPending(tenantID, agentID)` and `GetByID(tenantID, credID)`. To avoid breaking the completed Task 2 repositories from this task-owned file set, I implemented activation through an optional application-local `GetPendingByPlaintext(ctx, token)` interface. This keeps current repositories compatible and all tests passing, but the real PostgreSQL activation path will need a repository implementation or injected resolver in the follow-up integration task.
- Concern: `ListAgents` similarly uses an optional application-local list interface so the existing Task 2 `AgentRepository` contract remains source-compatible. PostgreSQL list support should be wired when REST/admin list integration is implemented.
- No generic `set_status` surface was added; status changes are only exposed through intent-specific service methods and domain methods.
- Existing controller coordination file `openspec/changes/agent-onboarding-and-identity/.comet/subagent-progress.md` was already modified before this task and was not touched or staged by me.

## Review round 1 fixes

- Promoted `AgentRepository.List`, `CredentialRepository.GetPendingByPlaintext`, and `CredentialRepository.GetLatestByAgent` into formal application ports.
- Removed the application-local optional `agentLister` and `plaintextCredentialRepository` type assertion fallbacks.
- Implemented PostgreSQL `AgentRepository.List`, plaintext activation credential lookup, and latest credential lookup.
- Changed nil `TokenIssuer` behavior from successful empty-token response to `invalid_argument` on `token_issuer`.
- Changed `GetActivationStatus` to read the latest credential and derive `pending`, `activated`, or `expired` from credential status, `expires_at`, and `consumed_at`.
- Added behavior coverage for RegisterAgent, ActivateAgent, ListAgents, GetActivationStatus, RevokeAgent, AgentHeartbeat, and nil TokenIssuer.
- Added PostgreSQL integration-ish coverage for formal list, postgres-backed ActivateAgent, postgres-backed ListAgents, and postgres-backed activation status paths.

## Review round 1 RED/GREEN evidence

RED commands:

```bash
cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/application -count=1
cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/postgres -count=1
```

RED result summary:

```text
FAIL TestIssueAccessTokenRequiresTokenIssuer: expected invalid_argument, got nil
FAIL TestGetActivationStatusReportsConsumedCredentialAsActivated: ActivatedAt was nil
FAIL TestGetActivationStatusReportsExpiredPendingCredentialAsExpired: expected expired, got pending
postgres compile failure: repo.List undefined (type application.AgentRepository has no field or method List)
```

GREEN commands:

```bash
cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/application -count=1
cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/postgres -run '^$' -count=1
cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/... -run '^$' -count=1
cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./... -run '^$' -count=1
git diff --check
```

GREEN result summary:

```text
ok agentguild.dev/agentguild/backend/internal/identity/application
ok agentguild.dev/agentguild/backend/internal/identity/postgres [no tests to run]
ok agentguild.dev/agentguild/backend/internal/identity/... [no tests to run]
ok agentguild.dev/agentguild/backend/... [no tests to run]
git diff --check: PASS
```

## Review round 1 verification results

- `cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/application -count=1`
  - PASS
- `cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/postgres -count=1`
  - BLOCKED in this sandbox: postgres tests call Docker/PostgreSQL via `internal/testdb`; the tool reported command/path/permission failure. Escalation was attempted and rejected because the approval service returned 503.
- `cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/... -count=1`
  - BLOCKED for the same Docker/PostgreSQL sandbox reason.
- `cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./internal/identity/... -run '^$' -count=1`
  - PASS compile-only verification for application/domain/postgres.
- `cd backend && GOCACHE=/private/tmp/agentguild-go-cache go test ./... -run '^$' -count=1`
  - PASS backend compile-only verification.
- `gofmt -w` on changed Go files
  - PASS
- `git diff --check`
  - PASS

## Review round 1 files changed

- `backend/internal/identity/application/ports.go`
- `backend/internal/identity/application/contracts.go`
- `backend/internal/identity/application/commands.go`
- `backend/internal/identity/application/queries.go`
- `backend/internal/identity/application/service_test.go`
- `backend/internal/identity/postgres/agent_repository.go`
- `backend/internal/identity/postgres/repository_test.go`
- `.superpowers/sdd/task-3-report.md`

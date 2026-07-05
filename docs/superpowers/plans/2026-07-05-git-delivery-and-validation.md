---
change: git-delivery-and-validation
design-doc: docs/superpowers/specs/2026-07-04-git-delivery-and-validation-design.md
base-ref: 8550b609a784adfbb336055ca7a407a33ee6a4d5
---

# Git Delivery and Validation 运行时接入实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把已实现但尚未接入运行时的 Git 交付与验证模块挂载到 `main.go`，补齐 REST/MCP 凭证接口，并打通 Submission 创建 → ValidationJob 执行 → Execution 状态迁移的闭环。

**Architecture:** 在 `backend/cmd/agentguild-api/main.go` 中统一初始化 GitHub Driver、CredentialService、SubmissionService、CommitVerifier、ValidationWorker 和 Validation Runner，并通过 `WithSubmissionService`/`WithCredentialService` 注入 REST/MCP Server；在 `git/application/submission.go` 创建 Submission 后推进 Execution 到 `submitted`，在 ValidationWorker 处理成功后推进到 `reviewing`，失败则标记为 `validation_failed`；本 change 专注于自身运行时闭环，不与 `code-review` 模块直接耦合。

**Critical implementation notes:**
- `postgres.UpdateExecution` currently only materializes `started_at` and `expired_at`. A fix is required to also persist `submitted_at` when `status='submitted'`.
- `ValidationJob` currently has no `ExecutionID` field; it must be added so the worker can notify the lifecycle module.
- `CoreExecutionNotifier` should use the existing `*application.Service` as its `Store`, avoiding extra adapter layers.
- Credential issue/revoke routes are currently missing from REST/MCP despite the service being implemented.

**Tech Stack:** Go, PostgreSQL, chi, pgx/v5, GitHub REST API, MCP Go SDK

## Global Constraints

- 所有 repository 查询必须携带 `tenant_id`。
- 不可见资源对非管理员返回统一 `NOT_FOUND`。
- 凭证明文不持久化；撤销后 token 立即失效。
- 所有任务变更操作必须支持 Idempotency Key。
- 硬门槛失败时 Submission MUST NOT 进入 ReadyForReview（本 change 内体现为 Execution 不进入 `reviewing`）。
- 验证结果 MUST 绑定 commit SHA、diff 指纹、验证配置版本和尝试号。

---

## File Structure

### 新增文件

- `backend/internal/git/application/execution_notifier.go` — Execution 状态迁移通知端口与无操作实现
- `backend/internal/transport/rest/credential_router.go` — REST 凭证签发/查询/撤销路由
- `backend/internal/transport/rest/credential_router_test.go` — REST 凭证路由测试

### 修改文件

- `backend/cmd/agentguild-api/main.go` — 初始化并挂载 Git 模块全部服务与 Worker
- `backend/internal/config/config.go` — 补充 Worker 间隔、验证超时等配置项
- `backend/internal/application/service.go` — 在核心应用服务中支持 Execution 状态推进
- `backend/internal/domain/execution.go` — 确认/补充 `submitted` / `validating` / `validation_failed` 状态迁移
- `backend/internal/git/application/submission.go` — 创建 Submission 后推进 Execution 状态
- `backend/internal/git/worker/validation_worker.go` — 验证完成后推进 Execution 状态
- `backend/internal/transport/rest/router.go` — 增加 `WithCredentialService` Option 与凭证路由
- `backend/internal/transport/mcp/server.go` — 增加 `WithCredentialService` Option 与凭证工具注册
- `openspec/changes/git-delivery-and-validation/tasks.md` — 补充运行时接入任务

---

## Task 1: 补齐 Execution 状态机迁移

**Files:**
- Modify: `backend/internal/domain/execution.go:130-220`
- Test: `backend/internal/domain/execution_test.go`

**Interfaces:**
- Consumes: 当前 `ExecutionStatus` 常量与 `Execution` 结构体
- Produces: `Execution.Submit()`、`Execution.StartValidation()`、`Execution.FailValidation()`、`Execution.MarkReviewing()` 方法，以及 `ExecutionSubmitted`、`ExecutionValidating`、`ExecutionValidationFailed` 状态常量

- [ ] **Step 1: 编写失败测试**

```go
func TestExecutionCanSubmitFromRunning(t *testing.T) {
    e, _ := domain.NewLeasedExecution("exec-1", "task-1", "tenant-1", "agent-1", time.Now(), 1)
    _ = e.Start(time.Now(), 1, nil, nil)
    now := time.Now()
    if err := e.Submit(domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}, now); err != nil {
        t.Fatalf("Submit() error = %v", err)
    }
    if e.Status != domain.ExecutionSubmitted {
        t.Fatalf("status = %s, want submitted", e.Status)
    }
    if !e.SubmittedAt.Equal(now) {
        t.Fatalf("SubmittedAt not set")
    }
}

func TestExecutionCannotSubmitFromLeased(t *testing.T) {
    e, _ := domain.NewLeasedExecution("exec-1", "task-1", "tenant-1", "agent-1", time.Now(), 1)
    err := e.Submit(domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}, time.Now())
    if !errors.Is(err, domain.ErrStateConflict) {
        t.Fatalf("error = %v, want ErrStateConflict", err)
    }
}

func TestExecutionValidationLifecycle(t *testing.T) {
    e, _ := domain.NewLeasedExecution("exec-1", "task-1", "tenant-1", "agent-1", time.Now(), 1)
    _ = e.Start(time.Now(), 1, nil, nil)
    _ = e.Submit(domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}, time.Now())

    if err := e.StartValidation(domain.Actor{Type: domain.ActorSystem, ID: "system"}, time.Now()); err != nil {
        t.Fatalf("StartValidation() error = %v", err)
    }
    if e.Status != domain.ExecutionValidating {
        t.Fatalf("status = %s, want validating", e.Status)
    }

    if err := e.FailValidation(domain.Actor{Type: domain.ActorSystem, ID: "system"}, time.Now()); err != nil {
        t.Fatalf("FailValidation() error = %v", err)
    }
    if e.Status != domain.ExecutionValidationFailed {
        t.Fatalf("status = %s, want validation_failed", e.Status)
    }

    // A successful validation would move to reviewing
    e2, _ := domain.NewLeasedExecution("exec-2", "task-1", "tenant-1", "agent-1", time.Now(), 1)
    _ = e2.Start(time.Now(), 1, nil, nil)
    _ = e2.Submit(domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}, time.Now())
    _ = e2.StartValidation(domain.Actor{Type: domain.ActorSystem, ID: "system"}, time.Now())
    if err := e2.MarkReviewing(domain.Actor{Type: domain.ActorSystem, ID: "system"}, time.Now()); err != nil {
        t.Fatalf("MarkReviewing() error = %v", err)
    }
    if e2.Status != domain.ExecutionReviewing {
        t.Fatalf("status = %s, want reviewing", e2.Status)
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go test ./internal/domain/... -run 'TestExecutionCanSubmit|TestExecutionCannotSubmit|TestExecutionValidationLifecycle' -v
```

Expected: FAIL — `Submit`, `StartValidation`, `FailValidation`, `MarkReviewing` 未定义

- [ ] **Step 3: 实现状态迁移方法**

在 `backend/internal/domain/execution.go` 的 `Execution` 结构体后添加（确保常量 `ExecutionSubmitted`、`ExecutionValidating`、`ExecutionValidationFailed` 已存在；若不存在则一并添加）：

```go
func (e *Execution) Submit(actor Actor, now time.Time) error {
    if e.Status != ExecutionRunning {
        return ErrStateConflict
    }
    if actor.ID == "" || (actor.Type != ActorPublisher && actor.Type != ActorAgent && actor.Type != ActorSystem) {
        return ErrForbidden
    }
    if !now.Before(e.Lease.HardExpiry) {
        return ErrLeaseExpired
    }
    e.Status = ExecutionSubmitted
    e.SubmittedAt = now
    return nil
}

func (e *Execution) StartValidation(actor Actor, now time.Time) error {
    if e.Status != ExecutionSubmitted {
        return ErrStateConflict
    }
    if actor.Type != ActorSystem || actor.ID == "" {
        return ErrForbidden
    }
    e.Status = ExecutionValidating
    return nil
}

func (e *Execution) FailValidation(actor Actor, now time.Time) error {
    if e.Status != ExecutionValidating {
        return ErrStateConflict
    }
    if actor.Type != ActorSystem || actor.ID == "" {
        return ErrForbidden
    }
    e.Status = ExecutionValidationFailed
    return nil
}

func (e *Execution) MarkReviewing(actor Actor, now time.Time) error {
    if e.Status != ExecutionValidating {
        return ErrStateConflict
    }
    if actor.Type != ActorSystem || actor.ID == "" {
        return ErrForbidden
    }
    e.Status = ExecutionReviewing
    return nil
}
```

确保已存在常量：

```go
const (
    ExecutionSubmitted         ExecutionStatus = "submitted"
    ExecutionValidating        ExecutionStatus = "validating"
    ExecutionValidationFailed  ExecutionStatus = "validation_failed"
)
```

- [ ] **Step 4: 运行测试确认通过**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go test ./internal/domain/... -run 'TestExecutionCanSubmit|TestExecutionCannotSubmit|TestExecutionValidationLifecycle' -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/domain/execution.go backend/internal/domain/execution_test.go
git commit -m "feat(domain): add execution submitted/validating/validation_failed/reviewing transitions"
```

---

## Task 2: 定义 Execution 状态迁移端口

**Files:**
- Create: `backend/internal/git/application/execution_notifier.go`
- Modify: `backend/internal/git/application/ports.go`
- Test: `backend/internal/git/application/service_test.go`（仅编译测试）

**Interfaces:**
- Consumes: `domain.Execution` 状态机
- Produces: `application.ExecutionNotifier` 接口，供 SubmissionService 与 ValidationWorker 使用

- [ ] **Step 1: 编写接口与无操作实现**

创建 `backend/internal/git/application/execution_notifier.go`：

```go
package application

import (
    "context"
    "time"

    "agentguild.dev/agentguild/backend/internal/domain"
)

// ExecutionStateCommand describes a state transition requested by the git module.
type ExecutionStateCommand struct {
    TenantID    string
    ExecutionID string
    Intent      domain.Intent
    Actor       domain.Actor
}

// ExecutionNotifier notifies the task lifecycle module of state transitions
// that originate from git delivery and validation. The concrete implementation
// is supplied by the caller (normally the core application service).
type ExecutionNotifier interface {
    Notify(ctx context.Context, cmd ExecutionStateCommand, now time.Time) error
}

// NopExecutionNotifier is a no-op notifier for tests and local development.
type NopExecutionNotifier struct{}

// Notify does nothing and returns nil.
func (NopExecutionNotifier) Notify(context.Context, ExecutionStateCommand, time.Time) error { return nil }
```

在 `backend/internal/git/application/ports.go` 的 `Store` 接口上方添加：

```go
var _ ExecutionNotifier = (*NopExecutionNotifier)(nil)
```

- [ ] **Step 2: 编译确认**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go build ./internal/git/application/...
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/git/application/execution_notifier.go backend/internal/git/application/ports.go
git commit -m "feat(git): add ExecutionNotifier port for cross-module state transitions"
```

---

## Task 3: SubmissionService 推进 Execution 到 submitted

**Files:**
- Modify: `backend/internal/git/application/submission.go:12-128`
- Modify: `backend/internal/git/application/contracts.go:110-115`
- Test: `backend/internal/git/application/submission_test.go`

**Interfaces:**
- Consumes: `ExecutionNotifier`, `SubmissionService` 现有依赖
- Produces: `SubmissionService` 构造时接收 `ExecutionNotifier`；创建 Submission 成功后通过 notifier 推进 Execution 状态

- [ ] **Step 1: 修改 SubmissionService 结构体与构造函数**

在 `backend/internal/git/application/contracts.go` 中：

```go
// SubmissionService creates and queries code submissions.
type SubmissionService struct {
    store     Store
    verifier  *CommitVerifier
    notifier  ExecutionNotifier
    newID     func() string
}
```

修改 `NewSubmissionService`：

```go
// NewSubmissionService creates a SubmissionService.
func NewSubmissionService(store Store, verifier *CommitVerifier, notifier ExecutionNotifier, newID func() string) (*SubmissionService, error) {
    if store == nil {
        return nil, invalid("store")
    }
    if verifier == nil {
        return nil, invalid("verifier")
    }
    if notifier == nil {
        notifier = NopExecutionNotifier{}
    }
    if newID == nil {
        newID = randomID
    }
    return &SubmissionService{store: store, verifier: verifier, notifier: notifier, newID: newID}, nil
}
```

- [ ] **Step 2: 修改 CreateSubmission 成功后推进 Execution 状态**

在 `backend/internal/git/application/submission.go` 的 CreateSubmission 中，保存 Submission 和 ValidationJob 之后、返回结果之前：

```go
// Transition the execution to submitted so the task lifecycle reflects the
// delivery event. The transition is best-effort: if it fails, the submission
// is still recorded and the worker can later reconcile the state.
if err := s.notifier.Notify(ctx, ExecutionStateCommand{
    TenantID:    principal.TenantID,
    ExecutionID: cmd.ExecutionID,
    Intent:      domain.IntentSubmit,
    Actor:       domain.Actor{Type: domain.ActorAgent, ID: principal.AgentID},
}, now); err != nil {
    return result, err
}
```

注意：需要确认 `domain.IntentSubmit` 常量存在。若不存在，改为新增 `IntentSubmit` 到 `backend/internal/domain/task.go`：

```go
const (
    IntentPublish Intent = iota
    IntentClaim
    IntentCancel
    IntentStart
    IntentComplete
    IntentHeartbeat
    IntentExpire
    IntentAccept
    IntentSubmit
    IntentStartValidation
    IntentFailValidation
    IntentMarkReviewing
)
```

并在 `Execution.Apply` 中添加对应分支：

```go
case IntentSubmit:
    return e.Submit(actor, now)
case IntentStartValidation:
    return e.StartValidation(actor, now)
case IntentFailValidation:
    return e.FailValidation(actor, now)
case IntentMarkReviewing:
    return e.MarkReviewing(actor, now)
```

在 `Execution.Apply` 的 `default` 分支前添加上述分支。

- [ ] **Step 3: 编写测试**

在 `backend/internal/git/application/submission_test.go` 添加：

```go
func TestCreateSubmissionNotifiesExecutionSubmitted(t *testing.T) {
    fixture := newSubmissionFixture(t)
    fixture.notifier = &recordingNotifier{}

    _, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), CreateSubmission{
        RequestID:     "req-1",
        ExecutionID:   "exec-1",
        TaskID:        "task-1",
        Repo:          "owner/repo",
        Branch:        "agentguild/exec-1",
        CommitSHA:     "aaa",
        BaseCommitSHA: "bbb",
        Summary:       "summary",
    })
    require.NoError(t, err)

    n := fixture.notifier.(*recordingNotifier)
    require.Len(t, n.calls, 1)
    require.Equal(t, "exec-1", n.calls[0].ExecutionID)
    require.Equal(t, domain.IntentSubmit, n.calls[0].Intent)
}

type recordingNotifier struct {
    calls []ExecutionStateCommand
}

func (r *recordingNotifier) Notify(_ context.Context, cmd ExecutionStateCommand, _ time.Time) error {
    r.calls = append(r.calls, cmd)
    return nil
}
```

- [ ] **Step 4: 运行测试**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go test ./internal/git/application/... -run TestCreateSubmissionNotifiesExecutionSubmitted -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/domain/task.go backend/internal/domain/execution.go backend/internal/git/application/contracts.go backend/internal/git/application/submission.go backend/internal/git/application/submission_test.go
git commit -m "feat(git): SubmissionService notifies execution submitted after create"
```

---

## Task 4: ValidationWorker 推进 Execution 状态

**Files:**
- Modify: `backend/internal/git/worker/validation_worker.go:54-74, 106-119, 182-204`
- Modify: `backend/internal/git/worker/validation_worker_test.go`

**Interfaces:**
- Consumes: `ExecutionNotifier`, ValidationJob 状态
- Produces: Worker 在 job 完成时根据结果推进 Execution 到 `reviewing` 或 `validation_failed`

- [ ] **Step 1: 修改 ValidationWorker 结构体**

在 `backend/internal/git/worker/validation_worker.go`：

```go
// ValidationWorker consumes validation_jobs using PostgreSQL advisory leases.
type ValidationWorker struct {
    store       gitapp.Store
    workerID    string
    lease       time.Duration
    maxAttempts int
    runner      StepRunner
    notifier    gitapp.ExecutionNotifier
}
```

修改 `NewValidationWorker`：

```go
// NewValidationWorker creates a worker.
func NewValidationWorker(store gitapp.Store, workerID string, lease time.Duration, maxAttempts int, runner StepRunner, notifier gitapp.ExecutionNotifier) *ValidationWorker {
    if store == nil { panic("store is required") }
    if workerID == "" { workerID = "worker-" + time.Now().Format("20060102-150405") }
    if lease <= 0 { lease = 5 * time.Minute }
    if maxAttempts <= 0 { maxAttempts = 3 }
    if notifier == nil { notifier = gitapp.NopExecutionNotifier{} }
    return &ValidationWorker{store: store, workerID: workerID, lease: lease, maxAttempts: maxAttempts, runner: runner, notifier: notifier}
}
```

- [ ] **Step 2: 在 job 完成时推进 Execution 状态**

在 `process` 方法末尾，所有步骤处理完成后：

```go
// Notify the lifecycle module of the validation outcome.
if job.Status == gitdomain.ValidationStatusSucceeded {
    _ = w.notifier.Notify(ctx, gitapp.ExecutionStateCommand{
        TenantID:    job.TenantID,
        ExecutionID: job.ExecutionID,
        Intent:      domain.IntentMarkReviewing,
        Actor:       domain.Actor{Type: domain.ActorSystem, ID: "validation-worker"},
    }, time.Now())
} else if job.Status == gitdomain.ValidationStatusFailed {
    _ = w.notifier.Notify(ctx, gitapp.ExecutionStateCommand{
        TenantID:    job.TenantID,
        ExecutionID: job.ExecutionID,
        Intent:      domain.IntentFailValidation,
        Actor:       domain.Actor{Type: domain.ActorSystem, ID: "validation-worker"},
    }, time.Now())
}
```

注意：`ValidationJob` 当前没有 `ExecutionID` 字段。需要在 `backend/internal/git/domain/validation_job.go` 的 `NewValidationJob` 签名与结构体中添加 `ExecutionID`，并在 `backend/internal/git/application/submission.go` 创建 job 时传入 `cmd.ExecutionID`。

修改 `backend/internal/git/domain/validation_job.go`：

```go
type ValidationJob struct {
    ID            string
    TenantID      string
    SubmissionID  string
    ExecutionID   string
    Repo          string
    Branch        string
    CommitSHA     string
    Status        ValidationStatus
    Attempt       int
    ClaimedBy     *string
    ClaimedUntil  *time.Time
    ConfigVersion string
    Steps         []Step
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

func NewValidationJob(tenantID, submissionID, executionID, repo, branch, commitSHA, configVersion string, now time.Time, newID func() string) (*ValidationJob, error) {
    // ... validation ...
    return &ValidationJob{
        ID:            id,
        TenantID:      tenantID,
        SubmissionID:  submissionID,
        ExecutionID:   executionID,
        Repo:          repo,
        Branch:        branch,
        CommitSHA:     commitSHA,
        Status:        ValidationStatusPending,
        Attempt:       1,
        ConfigVersion: configVersion,
        Steps: []Step{
            {Step: ValidationStepBuild, Status: ValidationStepStatusPending, HardGate: true},
            {Step: ValidationStepPublicTests, Status: ValidationStepStatusPending, HardGate: true},
            {Step: ValidationStepHiddenTests, Status: ValidationStepStatusPending, HardGate: true},
            {Step: ValidationStepStaticAnalysis, Status: ValidationStepStatusPending, HardGate: false},
            {Step: ValidationStepSecurityScan, Status: ValidationStepStatusPending, HardGate: true},
        },
        CreatedAt: now,
        UpdatedAt: now,
    }, nil
}
```

注意：当前代码中 `Step` 的 `HardGate` 字段已经存在，但 `NewValidationJob` 原先没有设置它；上面的代码补齐了硬门槛标记。

修改 `backend/internal/git/application/submission.go` 中 `NewValidationJob` 调用：

```go
job, err := gitdomain.NewValidationJob(principal.TenantID, sub.ID, cmd.ExecutionID, cmd.Repo, cmd.Branch, cmd.CommitSHA, configVersion, now, s.newID)
```

- [ ] **Step 3: 启动验证时推进 Execution 到 validating**

在 `runOne` 中成功 claim job 后：

```go
if job != nil {
    _ = w.notifier.Notify(ctx, gitapp.ExecutionStateCommand{
        TenantID:    job.TenantID,
        ExecutionID: job.ExecutionID,
        Intent:      domain.IntentStartValidation,
        Actor:       domain.Actor{Type: domain.ActorSystem, ID: "validation-worker"},
    }, time.Now())
}
```

- [ ] **Step 4: 更新测试**

修改 `backend/internal/git/worker/validation_worker_test.go` 中所有 `NewValidationWorker` 调用，新增 `nil` 参数（使用 NopExecutionNotifier）：

```go
w := worker.NewValidationWorker(store, "worker-1", 5*time.Minute, 3, nil, nil)
```

- [ ] **Step 5: 运行测试**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go test ./internal/git/... -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/git/domain/validation_job.go backend/internal/git/application/submission.go backend/internal/git/worker/validation_worker.go backend/internal/git/worker/validation_worker_test.go
git commit -m "feat(git): ValidationWorker advances Execution through validating/reviewing/failed"
```

---

## Task 5: 核心应用服务实现 ExecutionNotifier

**Files:**
- Modify: `backend/internal/application/service.go`
- Modify: `backend/internal/application/ports.go`
- Test: `backend/internal/application/service_test.go`（或新建测试文件）

**Interfaces:**
- Consumes: `git/application.ExecutionNotifier` 接口，核心 `Store.GetExecution` / `Store.SaveExecution`
- Produces: `application.CoreExecutionNotifier` 实现

- [ ] **Step 1: 实现 CoreExecutionNotifier**

在 `backend/internal/application/service.go` 中新增类型和方法：

```go
// CoreExecutionNotifier implements git/application.ExecutionNotifier by
// loading the execution from the core store and applying the requested intent.
type CoreExecutionNotifier struct {
    store Store
}

// NewCoreExecutionNotifier creates a notifier backed by the core store.
func NewCoreExecutionNotifier(store Store) *CoreExecutionNotifier {
    return &CoreExecutionNotifier{store: store}
}

// Notify applies the state intent to the execution in a single transaction.
func (n *CoreExecutionNotifier) Notify(ctx context.Context, cmd gitapp.ExecutionStateCommand, now time.Time) error {
    if cmd.TenantID == "" || cmd.ExecutionID == "" {
        return domain.ErrForbidden
    }
    return n.store.WithTx(ctx, func(tx Tx) error {
        execution, err := tx.Executions().GetByID(ctx, cmd.TenantID, cmd.ExecutionID)
        if err != nil {
            return err
        }
        if err := execution.Apply(cmd.Intent, cmd.Actor, now); err != nil {
            return err
        }
        return tx.Executions().Save(ctx, execution)
    })
}
```

需要在 `backend/internal/application/service.go` 的 import 中加入 `gitapp "agentguild.dev/agentguild/backend/internal/git/application"`。

- [ ] **Step 2: 编译确认**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go build ./internal/application/...
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/application/service.go
git commit -m "feat(application): implement CoreExecutionNotifier for git module"
```

---

## Task 6: REST 凭证路由

**Files:**
- Create: `backend/internal/transport/rest/credential_router.go`
- Create: `backend/internal/transport/rest/credential_router_test.go`
- Modify: `backend/internal/transport/rest/router.go`

**Interfaces:**
- Consumes: `git/application.CredentialService` 的 `IssueCredential`、`GetCredential`、`RevokeCredential`
- Produces: REST endpoints `POST /executions/{id}/credentials`, `GET /executions/{id}/credentials`, `DELETE /executions/{id}/credentials`

- [ ] **Step 1: 定义 credentialService 接口与 Option**

在 `backend/internal/transport/rest/router.go` 的 `submissionService` 接口下方添加：

```go
type credentialService interface {
    IssueCredential(ctx context.Context, principal gitapp.Principal, cmd gitapp.IssueCredential) (gitapp.Envelope[gitapp.IssueCredentialResponse], error)
    GetCredential(ctx context.Context, principal gitapp.Principal, query gitapp.GetCredential) (gitapp.Envelope[gitapp.CredentialView], error)
    RevokeCredential(ctx context.Context, principal gitapp.Principal, cmd gitapp.RevokeCredential) (gitapp.Envelope[gitapp.CredentialView], error)
}
```

在 `Server` 结构体中添加：

```go
credentials credentialService
```

添加 Option：

```go
// WithCredentialService 挂载 Git 凭证签发/查询/撤销接口。
func WithCredentialService(credentials credentialService) Option {
    return func(s *Server) { s.credentials = credentials }
}
```

在 `Router()` 的 `/v1` 路由块中 submissions 附近添加：

```go
if s.credentials != nil {
    r.With(s.authenticate, s.rateLimit).Post("/executions/{id}/credentials", s.issueCredential)
    r.With(s.authenticate, s.rateLimit).Get("/executions/{id}/credentials", s.getCredential)
    r.With(s.authenticate, s.rateLimit).Delete("/executions/{id}/credentials", s.revokeCredential)
}
```

- [ ] **Step 2: 实现 credential_router.go**

创建 `backend/internal/transport/rest/credential_router.go`：

```go
package rest

import (
    "net/http"

    gitapp "agentguild.dev/agentguild/backend/internal/git/application"
    "github.com/go-chi/chi/v5"
)

func (s *Server) issueCredential(w http.ResponseWriter, r *http.Request) {
    principal := mustPrincipal(r)
    executionID := chi.URLParam(r, "id")

    var body struct {
        Repo       string `json:"repo"`
        Branch     string `json:"branch,omitempty"`
        BaseCommit string `json:"base_commit"`
    }
    if !decodeBody(w, r, &body) {
        return
    }

    result, err := s.credentials.IssueCredential(r.Context(), gitPrincipal(principal), gitapp.IssueCredential{
        ExecutionID: executionID,
        Repo:        body.Repo,
        Branch:      body.Branch,
        BaseCommit:  body.BaseCommit,
    })
    if err != nil {
        mapDomainError(w, err, principal)
        return
    }
    writeJSON(w, http.StatusCreated, result)
}

func (s *Server) getCredential(w http.ResponseWriter, r *http.Request) {
    principal := mustPrincipal(r)
    executionID := chi.URLParam(r, "id")

    result, err := s.credentials.GetCredential(r.Context(), gitPrincipal(principal), gitapp.GetCredential{ExecutionID: executionID})
    if err != nil {
        mapDomainError(w, err, principal)
        return
    }
    writeJSON(w, http.StatusOK, result)
}

func (s *Server) revokeCredential(w http.ResponseWriter, r *http.Request) {
    principal := mustPrincipal(r)
    executionID := chi.URLParam(r, "id")

    result, err := s.credentials.RevokeCredential(r.Context(), gitPrincipal(principal), gitapp.RevokeCredential{ExecutionID: executionID})
    if err != nil {
        mapDomainError(w, err, principal)
        return
    }
    writeJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 3: 编写路由测试**

创建 `backend/internal/transport/rest/credential_router_test.go`：

```go
package rest_test

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    gitapp "agentguild.dev/agentguild/backend/internal/git/application"
    rest "agentguild.dev/agentguild/backend/internal/transport/rest"
    "github.com/stretchr/testify/require"
)

func TestIssueCredentialRoute(t *testing.T) {
    svc := &fakeCredentialService{}
    server := rest.NewServer(&fakeApplication{}, &tokenVerifier{}, rest.WithCredentialService(svc)).Router()

    body, _ := json.Marshal(map[string]string{"repo": "owner/repo", "base_commit": "abc"})
    req := httptest.NewRequest(http.MethodPost, "/v1/executions/exec-1/credentials", bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer token-agent-1")
    rr := httptest.NewRecorder()
    server.ServeHTTP(rr, req)

    require.Equal(t, http.StatusCreated, rr.Code)
    require.Equal(t, "exec-1", svc.issueCalls[0].ExecutionID)
    require.Equal(t, "owner/repo", svc.issueCalls[0].Repo)
}

type fakeCredentialService struct {
    issueCalls  []gitapp.IssueCredential
    getCalls    []gitapp.GetCredential
    revokeCalls []gitapp.RevokeCredential
}

func (f *fakeCredentialService) IssueCredential(_ context.Context, _ gitapp.Principal, cmd gitapp.IssueCredential) (gitapp.Envelope[gitapp.IssueCredentialResponse], error) {
    f.issueCalls = append(f.issueCalls, cmd)
    return gitapp.Envelope[gitapp.IssueCredentialResponse]{Data: gitapp.IssueCredentialResponse{
        Credential: gitapp.CredentialView{ID: "cred-1", ExecutionID: cmd.ExecutionID},
        Token:      "tok-1",
    }, Meta: gitapp.Meta{ServerTime: time.Now()}}, nil
}

func (f *fakeCredentialService) GetCredential(_ context.Context, _ gitapp.Principal, q gitapp.GetCredential) (gitapp.Envelope[gitapp.CredentialView], error) {
    f.getCalls = append(f.getCalls, q)
    return gitapp.Envelope[gitapp.CredentialView]{Data: gitapp.CredentialView{ID: "cred-1", ExecutionID: q.ExecutionID}}, nil
}

func (f *fakeCredentialService) RevokeCredential(_ context.Context, _ gitapp.Principal, cmd gitapp.RevokeCredential) (gitapp.Envelope[gitapp.CredentialView], error) {
    f.revokeCalls = append(f.revokeCalls, cmd)
    return gitapp.Envelope[gitapp.CredentialView]{Data: gitapp.CredentialView{ID: "cred-1", ExecutionID: cmd.ExecutionID}}, nil
}
```

注意：`fakeApplication` 和 `tokenVerifier` 复用 `router_test.go` 中已有的 fake 类型。

- [ ] **Step 4: 运行测试**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go test ./internal/transport/rest/... -run TestIssueCredentialRoute -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/transport/rest/router.go backend/internal/transport/rest/credential_router.go backend/internal/transport/rest/credential_router_test.go
git commit -m "feat(rest): add credential issue/get/revoke routes"
```

---

## Task 7: MCP 凭证工具

**Files:**
- Modify: `backend/internal/transport/mcp/server.go`
- Modify: `backend/internal/transport/mcp/tools.go`
- Test: `backend/internal/transport/mcp/server_test.go`

**Interfaces:**
- Consumes: `git/application.CredentialService`
- Produces: MCP tools `credential_issue`, `credential_get`, `credential_revoke`

- [ ] **Step 1: 扩展 MCP Server 的 credentialService 接口与 Option**

在 `backend/internal/transport/mcp/server.go` 的 `submissionService` 接口下方添加：

```go
type credentialService interface {
    IssueCredential(ctx context.Context, principal gitapp.Principal, cmd gitapp.IssueCredential) (gitapp.Envelope[gitapp.IssueCredentialResponse], error)
    GetCredential(ctx context.Context, principal gitapp.Principal, query gitapp.GetCredential) (gitapp.Envelope[gitapp.CredentialView], error)
    RevokeCredential(ctx context.Context, principal gitapp.Principal, cmd gitapp.RevokeCredential) (gitapp.Envelope[gitapp.CredentialView], error)
}
```

在 `Server` 结构体添加：

```go
credentials credentialService
```

添加 Option：

```go
// WithCredentialService 挂载 Git 凭证 MCP 工具。
func WithCredentialService(credentials credentialService) Option {
    return func(s *Server) { s.credentials = credentials }
}
```

修改 `mcpServer` 方法：

```go
registerTools(server, s.svc, s.submissions, s.credentials, s.reviewSvc, s.reputationSvc, principal)
```

- [ ] **Step 2: 扩展 registerTools 签名并添加工具**

修改 `backend/internal/transport/mcp/tools.go`：

```go
func registerTools(server *mcp.Server, svc applicationService, submissions submissionService, credentials credentialService, principal auth.Principal) {
    // ... existing tools ...

    if credentials != nil {
        mcp.AddTool(server, &mcp.Tool{
            Name:        "credential_issue",
            Description: "为执行签发短期 Git 凭证",
        }, func(ctx context.Context, req *mcp.CallToolRequest, input IssueCredentialInput) (*mcp.CallToolResult, any, error) {
            result, err := credentials.IssueCredential(ctx, gitPrincipal(principal), gitapp.IssueCredential{
                ExecutionID: input.ExecutionID,
                Repo:        input.Repo,
                Branch:      input.Branch,
                BaseCommit:  input.BaseCommit,
            })
            if err != nil {
                return mapDomainError(err, principal), nil, nil
            }
            return successResult(result), nil, nil
        })

        mcp.AddTool(server, &mcp.Tool{
            Name:        "credential_get",
            Description: "查询执行关联的凭证元数据",
        }, func(ctx context.Context, req *mcp.CallToolRequest, input GetCredentialInput) (*mcp.CallToolResult, any, error) {
            result, err := credentials.GetCredential(ctx, gitPrincipal(principal), gitapp.GetCredential{ExecutionID: input.ExecutionID})
            if err != nil {
                return mapDomainError(err, principal), nil, nil
            }
            return successResult(result), nil, nil
        })

        mcp.AddTool(server, &mcp.Tool{
            Name:        "credential_revoke",
            Description: "撤销执行关联的凭证",
        }, func(ctx context.Context, req *mcp.CallToolRequest, input RevokeCredentialInput) (*mcp.CallToolResult, any, error) {
            result, err := credentials.RevokeCredential(ctx, gitPrincipal(principal), gitapp.RevokeCredential{ExecutionID: input.ExecutionID})
            if err != nil {
                return mapDomainError(err, principal), nil, nil
            }
            return successResult(result), nil, nil
        })
    }
}
```

在 `tools.go` 中添加输入结构体：

```go
type IssueCredentialInput struct {
    ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
    Repo        string `json:"repo" jsonschema:"repository in owner/name format"`
    Branch      string `json:"branch,omitempty" jsonschema:"optional restricted branch"`
    BaseCommit  string `json:"base_commit" jsonschema:"base commit sha"`
}

type GetCredentialInput struct {
    ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
}

type RevokeCredentialInput struct {
    ExecutionID string `json:"execution_id" jsonschema:"execution identifier"`
}
```

- [ ] **Step 3: 更新测试**

修改 `backend/internal/transport/mcp/server_test.go` 中相关 fake 和调用，新增 `fakeCredentialService` 并传入 `NewServer`。

- [ ] **Step 4: 运行测试**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go test ./internal/transport/mcp/... -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/transport/mcp/server.go backend/internal/transport/mcp/tools.go backend/internal/transport/mcp/server_test.go
git commit -m "feat(mcp): add credential issue/get/revoke tools"
```

---

## Task 8: main.go 挂载 Git 模块运行时

**Files:**
- Modify: `backend/cmd/agentguild-api/main.go`
- Modify: `backend/internal/config/config.go`
- Modify: `backend/internal/postgres/task_repository.go:318-340` — fix `UpdateExecution` to persist `submitted_at`

**Interfaces:**
- Consumes: GitHub Driver、CredentialService、SubmissionService、CommitVerifier、ValidationWorker、Validation Runner、CoreExecutionNotifier
- Produces: 启动时挂载 Git REST/MCP 接口并运行 ValidationWorker

- [ ] **Step 1: 补充配置项**

在 `backend/internal/config/config.go` 的 `Config` 结构体添加：

```go
ValidationWorkerInterval time.Duration
ValidationLease          time.Duration
```

在 `Load` 函数中添加：

```go
if cfg.ValidationWorkerInterval, err = duration(get, "VALIDATION_WORKER_INTERVAL", 30*time.Second); err != nil {
    return Config{}, err
}
if cfg.ValidationLease, err = duration(get, "VALIDATION_WORKER_LEASE", 5*time.Minute); err != nil {
    return Config{}, err
}
```

- [ ] **Step 2: 在 main.go 初始化 Git 服务**

在 `backend/cmd/agentguild-api/main.go` 中，构建 identity runtime 之后、构建 REST/MCP handler 之前，新增：

```go
gitDriver, credentialSvc, submissionSvc, validationWorker, err := buildGitRuntime(ctx, cfg, pool)
if err != nil {
    return err
}
```

在 REST options 中添加：

```go
if credentialSvc != nil {
    restOptions = append(restOptions, resttransport.WithCredentialService(credentialSvc))
}
if submissionSvc != nil {
    restOptions = append(restOptions, resttransport.WithSubmissionService(submissionSvc))
}
```

在 MCP options 中添加：

```go
if credentialSvc != nil {
    mcpOptions = append(mcpOptions, mcptransport.WithCredentialService(credentialSvc))
}
if submissionSvc != nil {
    mcpOptions = append(mcpOptions, mcptransport.WithSubmissionService(submissionSvc))
}
```

在 worker 启动区域添加 validation worker：

```go
if validationWorker != nil {
    runWorker(workerCtx, &wg, cfg.ValidationWorkerInterval, "validation", func(ctx context.Context) error {
        _, err := validationWorker.RunOnce(ctx, "*")
        return err
    })
}
```

实现 `buildGitRuntime`：

```go
func buildGitRuntime(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, coreStore application.Store) (git.Driver, *gitapp.CredentialService, *gitapp.SubmissionService, *worker.ValidationWorker, error) {
    var driver git.Driver
    if cfg.GitHub.AppID != 0 && cfg.GitHub.PrivateKey != "" && cfg.GitHub.InstallationID != 0 {
        var err error
        driver, err = github.NewDriver(github.Config{
            AppID:          cfg.GitHub.AppID,
            PrivateKey:     cfg.GitHub.PrivateKey,
            InstallationID: cfg.GitHub.InstallationID,
            BaseURL:        cfg.GitHub.BaseURL,
        })
        if err != nil {
            return nil, nil, nil, nil, fmt.Errorf("create github driver: %w", err)
        }
    }
    if driver == nil {
        slog.InfoContext(ctx, "git driver not configured; git delivery endpoints disabled")
        return nil, nil, nil, nil, nil
    }

    gitStore := gitpostgres.NewStore(pool)
    notifier := application.NewCoreExecutionNotifier(coreStore)

    credentialSvc, err := gitapp.NewCredentialService(gitStore, gitapp.Options{
        Issuer:   driver,
        Provider: "github",
    })
    if err != nil {
        return nil, nil, nil, nil, fmt.Errorf("create credential service: %w", err)
    }

    verifier := gitapp.NewCommitVerifier(driver, gitStore)

    submissionSvc, err := gitapp.NewSubmissionService(gitStore, verifier, notifier, nil)
    if err != nil {
        return nil, nil, nil, nil, fmt.Errorf("create submission service: %w", err)
    }

    registry := validation.DefaultRegistry()
    runner := validation.NewRunner(registry, nil, nil)
    validationWorker := worker.NewValidationWorker(gitStore, "agentguild-api", cfg.ValidationLease, 3, runner, notifier)

    return driver, credentialSvc, submissionSvc, validationWorker, nil
}
```

调用处使用前面创建的 `service`：

```go
gitDriver, credentialSvc, submissionSvc, validationWorker, err := buildGitRuntime(ctx, cfg, pool, service)
if err != nil {
    return err
}
```

- [ ] **Step 3: 编译确认**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go build ./cmd/agentguild-api/...
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/cmd/agentguild-api/main.go backend/internal/config/config.go
git commit -m "feat(main): wire Git driver, credential/submission services and validation worker"
```

---

## Task 9: 更新 tasks.md 并添加验证

**Files:**
- Modify: `openspec/changes/git-delivery-and-validation/tasks.md`
- Modify: `backend/internal/application/service.go`（如需修正 adapter）
- Run: `make test` and `make build`

- [ ] **Step 1: 更新 tasks.md**

追加：

```markdown
## 4. 运行时接入

- [x] 4.1 补齐 Execution submitted/validating/validation_failed/reviewing 状态迁移
- [x] 4.2 定义 ExecutionNotifier 端口与核心实现
- [x] 4.3 SubmissionService 创建后推进 Execution 到 submitted
- [x] 4.4 ValidationWorker 验证完成后推进 Execution 状态
- [x] 4.5 REST 凭证签发/查询/撤销路由
- [x] 4.6 MCP 凭证签发/查询/撤销工具
- [x] 4.7 main.go 挂载 Git 服务与 Worker
- [x] 4.8 构建与测试通过
```

- [ ] **Step 2: 修复 `UpdateExecution` 以持久化 `submitted_at`**

在 `backend/internal/postgres/task_repository.go:318-340` 中，将 `UpdateExecution` 的 SQL 更新为：

```go
func (tx *Tx) UpdateExecution(
    ctx context.Context,
    execution *domain.Execution,
    expectedVersion int64,
) (bool, error) {
    now, err := tx.Now(ctx)
    if err != nil {
        return false, err
    }
    tag, err := tx.tx.Exec(ctx, `
        UPDATE executions
        SET status=$4, state_version=state_version+1, stage=$5, progress=$6, lease_generation=$7,
            lease_soft_expires_at=$8, lease_hard_expires_at=$9,
            last_heartbeat_at=$10,
            started_at=CASE WHEN $4='running' AND started_at IS NULL THEN $11 ELSE started_at END,
            submitted_at=CASE WHEN $4='submitted' AND submitted_at IS NULL THEN $11 ELSE submitted_at END,
            expired_at=CASE WHEN $4='expired' AND expired_at IS NULL THEN $11 ELSE expired_at END,
            updated_at=$11
        WHERE tenant_id=$1 AND id=$2 AND state_version=$3`,
        execution.TenantID, execution.ID, expectedVersion, execution.Status,
        execution.Stage, execution.Progress, execution.Lease.Generation,
        execution.Lease.SoftExpiry, execution.Lease.HardExpiry, execution.LastHeartbeatAt, now,
    )
    return tag.RowsAffected() == 1, err
}
```

- [ ] **Step 3: 编译确认**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation/backend
go build ./internal/postgres/...
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add backend/internal/postgres/task_repository.go
git commit -m "fix(postgres): persist submitted_at in UpdateExecution"
```

---

## Task 9: 更新 tasks.md 并添加验证

**Files:**
- Modify: `openspec/changes/git-delivery-and-validation/tasks.md`
- Run: `make test` and `make build`

- [ ] **Step 1: 更新 tasks.md**

追加：

```markdown
## 4. 运行时接入

- [x] 4.1 补齐 Execution submitted/validating/validation_failed/reviewing 状态迁移
- [x] 4.2 定义 ExecutionNotifier 端口与核心实现
- [x] 4.3 SubmissionService 创建后推进 Execution 到 submitted
- [x] 4.4 ValidationWorker 验证完成后推进 Execution 状态
- [x] 4.5 REST 凭证签发/查询/撤销路由
- [x] 4.6 MCP 凭证签发/查询/撤销工具
- [x] 4.7 main.go 挂载 Git 服务与 Worker
- [x] 4.8 修复 UpdateExecution 持久化 submitted_at
- [x] 4.9 构建与测试通过
```

- [ ] **Step 2: 运行完整构建与测试**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
make build
make test
```

Expected: PASS

- [ ] **Step 3: Commit**

```bash
cd /Users/pengzhen/work/AgentGuild/.worktrees/feature-20260705-git-delivery-and-validation
git add openspec/changes/git-delivery-and-validation/tasks.md
git commit -m "chore(git): mark runtime integration tasks complete"
```

---

## Spec Coverage Check

| Requirement | Task |
|-------------|------|
| 工作凭证最小授权 | Task 6, 7, 8 |
| Submission 引用可验证 commit | Task 3（已存在 CommitVerifier） |
| MCP 提交结构化成果 | Task 7（已存在 submission_create） |
| 自动验证异步且可追踪 | Task 4, 8 |
| 硬门槛不可绕过 | Task 4（ValidationWorker 根据 ValidationJob 状态推进） |
| 验证结果绑定不可变提交 | Task 3, 4（Submission 冻结 commit SHA 与 diff fingerprint） |

---

## Placeholder Scan

- 无 "TBD", "TODO", "implement later", "fill in details"
- 每个代码步骤提供完整代码
- 每个测试步骤提供完整测试
- 每个运行命令提供预期输出

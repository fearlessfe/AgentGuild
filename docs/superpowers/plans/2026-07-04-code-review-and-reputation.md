---
change: code-review-and-reputation
design-doc: docs/superpowers/specs/2026-07-04-code-review-and-reputation-design.md
base-ref: 9bfdf6900305421ec222063adde41d98960b4943
---

Language: 中文

# Code Review and Reputation 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans` 按任务分步实施。步骤使用 `- [ ]` 复选框语法以便跟踪。

**Goal:** 在 AgentGuild Coding MVP 中实现可追踪的人工代码审核、基于 rubric 的评分与退回流程，并将质量信号按 Agent Version + capability + task type 聚合为声望投影。

**Architecture:** 沿用现有分层结构，新增 `internal/review/*` 与 `internal/reputation/*` 两个限界上下文；Review 应用服务通过 `domain.Execution.Apply(intent)` 推进 Execution 状态，并依赖 Git/Validation 模块端口读取 Diff 与硬门槛；Reputation 通过后台 Worker 扫描已提交 Review 并重算投影。React 审核页通过 REST 读取规范化 Diff、评论与验证证据。

**Tech Stack:** Go 1.26.4、PostgreSQL 18.4、chi/v5、pgx/v5、modelcontextprotocol/go-sdk、React 19 + react-router-dom + @tanstack/react-query、Vitest + Playwright。

---

## Global Constraints

- 运行时基线：Go 1.26.4、PostgreSQL 18.4，与 `git-delivery-and-validation` 一致。
- 新增代码优先放到 `backend/internal/review/` 与 `backend/internal/reputation/`，前端放到 `frontend/src/features/reviews/`。
- REST 继续使用 chi/v5，统一返回 `Envelope[T]`，幂等键优先读取 `Idempotency-Key` Header。
- 所有 Repository 查询必须带 `tenant_id`，错误统一映射到 `domain.Error` 的 `not_found` / `state_conflict` / `forbidden` / `invalid_argument`。
- 审核权限角色：publisher、被分配的 reviewer、相关 agent 的 owner；通过 scope `reviews:read`、`reviews:write`、`reputation:read` 控制。
- 不实现自动合并、GitLab/GitHub 评论双向同步、单一全局总分、多审仲裁、实时事件总线。
- Diff/Validation 模块接口若未就绪，先以内存 stub 实现，并在端口处标注 TODO。

---

## 1. 文件结构

### 1.1 新增后端文件

```text
backend/internal/review/domain/review.go          # Review、Decision、ReviewStatus 状态机
backend/internal/review/domain/comment.go         # LineComment 与定位规则
backend/internal/review/domain/rubric.go          # RubricVersion、Dimension、Score
backend/internal/review/domain/reviewer.go        # ReviewerProfile
backend/internal/review/application/ports.go      # Review/Comment/Rubric/Reviewer 仓库端口
backend/internal/review/application/service.go    # CreateReview、SubmitDecision、AddComment、GetReview
backend/internal/review/application/allocator.go  # ReviewerAllocator
backend/internal/review/application/policy.go     # AuthorizationPolicy
backend/internal/review/postgres/review_repository.go
backend/internal/review/postgres/comment_repository.go
backend/internal/review/postgres/rubric_repository.go
backend/internal/review/postgres/reviewer_repository.go
backend/internal/reputation/domain/projection.go  # ReputationProjection 与算法版本
backend/internal/reputation/application/ports.go  # ProjectionRepository
backend/internal/reputation/application/projector.go
backend/internal/reputation/postgres/projection_repository.go
backend/internal/reputation/worker/worker.go      # 扫描未处理 Review 并重算
backend/internal/transport/rest/review_router.go  # REST 路由
backend/internal/transport/mcp/review_tools.go    # MCP 工具
backend/migrations/000003_code_review_and_reputation.up.sql
backend/migrations/000003_code_review_and_reputation.down.sql
```

### 1.2 修改现有后端文件

```text
backend/internal/domain/task.go                   # 新增 Intent* 常量
backend/internal/domain/execution.go              # 扩展 Accept / 新增 Reject / RequestRevision / Apply 分支
backend/internal/application/ports.go             # Tx 接口新增 Review/Comment/Rubric/Reviewer/Reputation 端口
backend/internal/transport/rest/router.go         # 注册 review / reputation / rubric 路由
backend/internal/transport/mcp/tools.go           # 注册 review_* / reputation_* 工具
backend/internal/postgres/store.go                # 无需修改，Tx 方法直接实现接口
```

### 1.3 新增前端文件

```text
frontend/src/features/reviews/reviews.api.ts
frontend/src/features/reviews/reviews.types.ts
frontend/src/features/reviews/ReviewPage.tsx
frontend/src/features/reviews/DiffViewer.tsx
frontend/src/features/reviews/FileTree.tsx
frontend/src/features/reviews/LineComment.tsx
frontend/src/features/reviews/RubricForm.tsx
frontend/src/features/reviews/RevisionSelector.tsx
frontend/src/features/reputation/reputation.api.ts
frontend/src/features/reputation/reputation.types.ts
frontend/src/features/reputation/ReputationPage.tsx
frontend/src/app/routes.tsx                      # 注册新路由（若存在）
```

---

## 2. 任务拆分

### Task 1: Review / LineComment / RubricVersion / ReviewerProfile 领域模型

**Files:**
- Create: `backend/internal/review/domain/review.go`
- Create: `backend/internal/review/domain/comment.go`
- Create: `backend/internal/review/domain/rubric.go`
- Create: `backend/internal/review/domain/reviewer.go`
- Test: `backend/internal/review/domain/review_test.go`
- Test: `backend/internal/review/domain/rubric_test.go`

**Interfaces:**
- Consumes: `domain.Execution` 的 `ExecutionReviewing` 状态、`domain.ActorReviewer`
- Produces:
  - `Review{ID, TenantID, SubmissionID, ReviewerID, RubricVersionID, RubricScores, Summary, Status, FinalDecision, SubmittedAt, CreatedAt}`
  - `Decision` 类型：`accepted | rejected | revision_requested`
  - `LineComment{ID, TenantID, ReviewID, SubmissionID, FilePath, Side, LineNumber, HunkHash, DiffFingerprint, Text, CreatedAt}`
  - `RubricVersion{ID, TenantID, VersionNumber, Name, Dimensions, Weights, AlgorithmVersion, IsActive, CreatedAt}`
  - `ReviewerProfile{ID, TenantID, UserID, Capabilities, CurrentLoad, IsActive, CreatedAt, UpdatedAt}`

- [x] **Step 1: 写 Review 状态机失败测试**

```go
func TestReviewSubmitRequiresPending(t *testing.T) {
    review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
    if err := review.Submit(decisionAccepted(), nil, time.Now()); err == nil {
        t.Fatal("expected error when scores are empty")
    }
}
```

- [x] **Step 2: 运行测试确认失败**

```bash
cd /Users/pengzhen/work/AgentGuild/backend
go test ./internal/review/domain/... -run TestReviewSubmitRequiresPending -v
```

- [x] **Step 3: 实现领域类型与 Submit 校验**

```go
func (r *Review) Submit(decision Decision, scores []RubricScore, now time.Time) error {
    if r.Status != ReviewPending {
        return domain.ErrStateConflict
    }
    if decision == DecisionAccepted && !rubricComplete(scores, r.RubricVersionID) {
        return invalidArgument("rubric_scores")
    }
    r.FinalDecision = decision
    r.RubricScores = scores
    r.Status = ReviewSubmitted
    r.SubmittedAt = now
    return nil
}
```

- [x] **Step 4: 运行测试确认通过**

```bash
go test ./internal/review/domain/... -v
```

- [x] **Step 5: 提交**  <!-- task-checkoff: Task 1 complete -->

```bash
git add backend/internal/review/domain

git commit -m "feat(review): add review, comment, rubric and reviewer domain models"
```

---

### Task 2: 数据库迁移与 Review 持久化

**Files:**
- Create: `backend/migrations/000003_code_review_and_reputation.up.sql`
- Create: `backend/migrations/000003_code_review_and_reputation.down.sql`
- Create: `backend/internal/review/application/ports.go`
- Create: `backend/internal/review/postgres/review_repository.go`
- Create: `backend/internal/review/postgres/comment_repository.go`
- Create: `backend/internal/review/postgres/rubric_repository.go`
- Create: `backend/internal/review/postgres/reviewer_repository.go`
- Test: `backend/internal/review/postgres/review_repository_test.go`

**Interfaces:**
- Consumes: `review/domain` 类型
- Produces:
  - `ReviewRepository.Insert/Update/Get/ListBySubmission`
  - `LineCommentRepository.Insert/ListByReview`
  - `RubricRepository.GetActive/GetByID/ListVersions/CreateVersion`
  - `ReviewerRepository.GetByID/ListActive/IncrementLoad/DecrementLoad`

- [ ] **Step 1: 编写迁移 SQL（含表、约束、索引）**

```sql
CREATE TABLE reviews (
    tenant_id text NOT NULL,
    id text NOT NULL,
    submission_id text NOT NULL,
    reviewer_id text NOT NULL,
    rubric_version_id text NOT NULL,
    rubric_scores jsonb NOT NULL DEFAULT '[]'::jsonb,
    summary text,
    status text NOT NULL,
    final_decision text,
    submitted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT reviews_status_valid CHECK (status IN ('pending', 'submitted')),
    CONSTRAINT reviews_decision_valid CHECK (
        final_decision IS NULL OR final_decision IN ('accepted', 'rejected', 'revision_requested')
    )
);
CREATE UNIQUE INDEX reviews_one_per_submission ON reviews (tenant_id, submission_id);

CREATE TABLE line_comments (
    tenant_id text NOT NULL,
    id text NOT NULL,
    review_id text NOT NULL,
    submission_id text NOT NULL,
    file_path text NOT NULL,
    side text NOT NULL,
    line_number int NOT NULL,
    hunk_hash text NOT NULL,
    diff_fingerprint text NOT NULL,
    text text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id)
);
CREATE INDEX line_comments_review ON line_comments (tenant_id, review_id);

CREATE TABLE rubric_versions (
    tenant_id text NOT NULL,
    id text NOT NULL,
    version_number int NOT NULL,
    name text NOT NULL,
    dimensions jsonb NOT NULL,
    weights jsonb NOT NULL,
    algorithm_version text NOT NULL,
    is_active boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, version_number)
);
CREATE UNIQUE INDEX rubric_active_one_per_tenant ON rubric_versions (tenant_id) WHERE is_active;

CREATE TABLE reviewer_profiles (
    tenant_id text NOT NULL,
    id text NOT NULL,
    user_id text NOT NULL,
    capabilities text[] NOT NULL DEFAULT '{}',
    current_load int NOT NULL DEFAULT 0 CHECK (current_load >= 0),
    is_active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, user_id)
);

CREATE TABLE reputation_projections (
    tenant_id text NOT NULL,
    id text NOT NULL,
    agent_version_id text NOT NULL,
    capability text NOT NULL,
    task_type text NOT NULL,
    total_reviews int NOT NULL DEFAULT 0,
    accepted_count int NOT NULL DEFAULT 0,
    rejected_count int NOT NULL DEFAULT 0,
    revision_requested_count int NOT NULL DEFAULT 0,
    pass_rate numeric(5,4),
    rework_rate numeric(5,4),
    avg_review_cost_cents bigint,
    avg_review_latency_ms bigint,
    sample_size_hint text NOT NULL DEFAULT 'low',
    algorithm_version text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (tenant_id, id),
    UNIQUE (tenant_id, agent_version_id, capability, task_type)
);
```

- [ ] **Step 2: 创建 down 迁移删除上述表**

- [ ] **Step 3: 实现 postgres 仓库方法并写失败测试**

```go
func TestInsertReviewIsTenantScoped(t *testing.T) {
    // 使用 backend/internal/testdb 初始化测试数据库并验证写入/读取
}
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/review/postgres/... -v
```

- [ ] **Step 5: 提交**

```bash
git add backend/migrations backend/internal/review/postgres backend/internal/review/application/ports.go
git commit -m "feat(review): add review persistence and migrations"
```

---

### Task 3: Execution 状态机扩展以支持审核决策

**Files:**
- Modify: `backend/internal/domain/task.go`
- Modify: `backend/internal/domain/execution.go`
- Test: `backend/internal/domain/execution_test.go`

**Interfaces:**
- Consumes: `domain.Intent` 枚举、`domain.Actor`
- Produces:
  - 新增 `IntentRequestRevision`、`IntentReject`
  - `Execution.Accept(actor, now)` 允许从 `reviewing` 到 `accepted`
  - `Execution.Reject(actor, now)`：`reviewing` → `rejected`
  - `Execution.RequestRevision(actor, now)`：`reviewing` → `revision_requested`

- [ ] **Step 1: 写状态机测试**

```go
func TestExecutionReviewingCanBeAcceptedByReviewer(t *testing.T) {
    now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
    e := mustNewExecution(t, "exe-1", "task-1", "tenant-1", "agent-1", now, 1)
    e.Status = domain.ExecutionReviewing
    if err := e.Apply(domain.IntentAccept, domain.Actor{Type: domain.ActorReviewer, ID: "r1"}, now); err != nil {
        t.Fatalf("accept failed: %v", err)
    }
    if e.Status != domain.ExecutionAccepted {
        t.Fatalf("status=%s, want accepted", e.Status)
    }
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/domain/... -run TestExecutionReviewingCanBeAcceptedByReviewer -v
```

- [ ] **Step 3: 修改 Execution 方法**

```go
func (e *Execution) Accept(actor Actor, now time.Time) error {
    if e.Status != ExecutionRunning && e.Status != ExecutionReviewing {
        return ErrStateConflict
    }
    if actor.Type != ActorReviewer || actor.ID == "" {
        return ErrForbidden
    }
    e.Status = ExecutionAccepted
    return nil
}

func (e *Execution) Reject(actor Actor, _ time.Time) error {
    if e.Status != ExecutionReviewing {
        return ErrStateConflict
    }
    if actor.Type != ActorReviewer || actor.ID == "" {
        return ErrForbidden
    }
    e.Status = ExecutionRejected
    return nil
}

func (e *Execution) RequestRevision(actor Actor, _ time.Time) error {
    if e.Status != ExecutionReviewing {
        return ErrStateConflict
    }
    if actor.Type != ActorReviewer || actor.ID == "" {
        return ErrForbidden
    }
    e.Status = ExecutionRevisionRequested
    return nil
}
```

- [ ] **Step 4: 运行测试并提交**

```bash
go test ./internal/domain/... -v
git add backend/internal/domain
git commit -m "feat(domain): extend execution state machine for review decisions"
```

---

### Task 4: Review 应用服务与权限策略

**Files:**
- Create: `backend/internal/review/application/service.go`
- Create: `backend/internal/review/application/policy.go`
- Create: `backend/internal/review/application/allocator.go`
- Modify: `backend/internal/application/ports.go`
- Test: `backend/internal/review/application/service_test.go`

**Interfaces:**
- Consumes:
  - `ReviewRepository`、`LineCommentRepository`、`RubricRepository`、`ReviewerRepository`
  - `ExecutionRepository`（来自 `application.Tx`）
  - `DiffProvider.GetDiff(ctx, submissionID)`、`ValidationProvider.GetValidationStatus(ctx, submissionID)`
- Produces:
  - `CreateReview(ctx, principal, cmd CreateReview) (Envelope[ReviewView], error)`
  - `SubmitDecision(ctx, principal, cmd SubmitDecision) (Envelope[ReviewView], error)`
  - `AddComment(ctx, principal, cmd AddComment) (Envelope[CommentView], error)`
  - `GetReview(ctx, principal, query GetReview) (Envelope[ReviewView], error)`

- [ ] **Step 1: 定义端口和命令类型**

```go
type CreateReview struct {
    SubmissionID string
    Capabilities []string
}
type SubmitDecision struct {
    ReviewID string
    Decision Decision
    Scores   []RubricScore
    Summary  string
}
type AddComment struct {
    ReviewID        string
    SubmissionID    string
    FilePath        string
    Side            string
    LineNumber      int
    HunkHash        string
    DiffFingerprint string
    Text            string
}
```

- [ ] **Step 2: 实现硬门槛复检**

```go
if cmd.Decision == DecisionAccepted {
    status, err := validation.GetValidationStatus(ctx, review.SubmissionID)
    if err != nil {
        return result, err
    }
    if !status.AllHardGatesPassed() {
        return result, &domain.Error{Code: "hard_gates_failed", Message: "cannot accept submission with failed hard gates"}
    }
}
```

- [ ] **Step 3: 实现权限策略（publisher / reviewer / agent owner）**

```go
func (p *Policy) CanViewReview(ctx context.Context, principal auth.Principal, review ReviewRecord, task TaskSummary) error
func (p *Policy) CanSubmitDecision(principal auth.Principal, review ReviewRecord) error
```

- [ ] **Step 4: 实现 ReviewerAllocator**

```go
func (a *Allocator) Allocate(ctx context.Context, tx application.Tx, tenantID string, caps []string) (string, error)
```

选择逻辑：匹配 capability → 最低 current_load → 同负载时按创建时间/ID 轮询。

- [ ] **Step 5: 写应用服务测试并提交**

```bash
go test ./internal/review/application/... -v
git add backend/internal/review/application backend/internal/application/ports.go
git commit -m "feat(review): add review application service, policy and allocator"
```

---

### Task 5: REST 路由：Review / Comment / Rubric / Reputation

**Files:**
- Create: `backend/internal/transport/rest/review_router.go`
- Modify: `backend/internal/transport/rest/router.go`
- Modify: `backend/internal/transport/rest/router_test.go`

**Interfaces:**
- Consumes: `reviewService` 接口
- Produces:
  - `GET /v1/reviews/:id`
  - `POST /v1/submissions/:id/reviews`（创建 Review，publisher/owner 或系统调用）
  - `POST /v1/reviews/:id/decision`
  - `POST /v1/reviews/:id/comments`
  - `GET /v1/rubrics/active`
  - `GET /v1/reputation?agent_version_id=...&capability=...&task_type=...`

- [ ] **Step 1: 定义 reviewService 接口并注入 Server**

```go
type reviewService interface {
    CreateReview(context.Context, auth.Principal, reviewapp.CreateReview) (reviewapp.Envelope[reviewapp.ReviewView], error)
    SubmitDecision(context.Context, auth.Principal, reviewapp.SubmitDecision) (reviewapp.Envelope[reviewapp.ReviewView], error)
    AddComment(context.Context, auth.Principal, reviewapp.AddComment) (reviewapp.Envelope[reviewapp.CommentView], error)
    GetReview(context.Context, auth.Principal, reviewapp.GetReview) (reviewapp.Envelope[reviewapp.ReviewView], error)
}
type reputationService interface {
    GetProjection(context.Context, auth.Principal, reputationapp.GetProjection) (reputationapp.Envelope[reputationapp.ProjectionView], error)
}
type rubricService interface {
    GetActiveRubric(context.Context, auth.Principal) (reviewapp.Envelope[reviewapp.RubricView], error)
}
```

- [ ] **Step 2: 注册路由**

```go
r.With(s.authenticate, s.rateLimit).Get("/reviews/{id}", s.getReview)
r.With(s.authenticate, s.rateLimit).Post("/submissions/{id}/reviews", s.createReview)
r.With(s.authenticate, s.rateLimit).Post("/reviews/{id}/decision", s.submitDecision)
r.With(s.authenticate, s.rateLimit).Post("/reviews/{id}/comments", s.addComment)
r.With(s.authenticate, s.rateLimit).Get("/rubrics/active", s.getActiveRubric)
r.With(s.authenticate, s.rateLimit).Get("/reputation", s.getReputation)
```

- [ ] **Step 3: 写契约测试并提交**

```bash
go test ./internal/transport/rest/... -v
git add backend/internal/transport/rest
git commit -m "feat(rest): add review, rubric and reputation routes"
```

---

### Task 6: MCP 工具

**Files:**
- Create: `backend/internal/transport/mcp/review_tools.go`
- Modify: `backend/internal/transport/mcp/tools.go`

**Interfaces:**
- Consumes: `reviewService`、`reputationService`
- Produces:
  - `review_submit`（提交决策）
  - `review_get`（查询 Review）
  - `reputation_get`（查询声望投影）

- [ ] **Step 1: 定义输入结构体**

```go
type ReviewSubmitInput struct {
    RequestID string                `json:"request_id"`
    ReviewID  string                `json:"review_id"`
    Decision  string                `json:"decision"`
    Scores    []reviewapp.RubricScore `json:"scores,omitempty"`
    Summary   string                `json:"summary,omitempty"`
}
type ReviewGetInput struct {
    ReviewID string `json:"review_id"`
}
type ReputationGetInput struct {
    AgentVersionID string `json:"agent_version_id"`
    Capability     string `json:"capability"`
    TaskType       string `json:"task_type"`
}
```

- [ ] **Step 2: 注册工具并转发到应用服务**

```go
mcp.AddTool(server, &mcp.Tool{Name: "review_submit", Description: "提交审核决策"},
    func(ctx context.Context, req *mcp.CallToolRequest, input ReviewSubmitInput) (*mcp.CallToolResult, any, error) {
        result, err := svc.SubmitDecision(ctx, principal, reviewapp.SubmitDecision{...})
        ...
    })
```

- [ ] **Step 3: 运行 MCP 测试并提交**

```bash
go test ./internal/transport/mcp/... -v
git add backend/internal/transport/mcp
git commit -m "feat(mcp): add review_submit, review_get and reputation_get tools"
```

---

### Task 7: 声望领域模型与投影算法

**Files:**
- Create: `backend/internal/reputation/domain/projection.go`
- Create: `backend/internal/reputation/application/projector.go`
- Create: `backend/internal/reputation/application/ports.go`
- Test: `backend/internal/reputation/domain/projection_test.go`

**Interfaces:**
- Consumes: Review 决策记录、Execution agent_version_id、task_type、capabilities
- Produces:
  - `ProjectionKey{AgentVersionID, Capability, TaskType}`
  - `Projection{TotalReviews, AcceptedCount, RejectedCount, RevisionRequestedCount, PassRate, ReworkRate, AvgReviewCostCents, AvgReviewLatencyMs, SampleSizeHint, AlgorithmVersion}`
  - `Projector.Project(ctx, signals []ReviewSignal) ([]Projection, error)`

- [ ] **Step 1: 写投影测试**

```go
func TestProjectionShowsLowSampleHint(t *testing.T) {
    p := reputation.NewProjection("agent-v1", "go", "code")
    p.Apply(reputation.ReviewSignal{Decision: review.DecisionAccepted, CostCents: 100, LatencyMs: 5000})
    if p.SampleSizeHint != "low" {
        t.Fatalf("hint=%s, want low", p.SampleSizeHint)
    }
}
```

- [ ] **Step 2: 实现算法**

```go
func (p *Projection) Apply(s ReviewSignal) {
    p.TotalReviews++
    switch s.Decision {
    case review.DecisionAccepted:
        p.AcceptedCount++
    case review.DecisionRejected:
        p.RejectedCount++
    case review.DecisionRevisionRequested:
        p.RevisionRequestedCount++
    }
    p.PassRate = float64(p.AcceptedCount) / float64(p.TotalReviews)
    p.ReworkRate = float64(p.RevisionRequestedCount) / float64(p.TotalReviews)
    // 样本量提示阈值：low < 5, medium < 20, high >= 20
    p.SampleSizeHint = sampleSizeHint(p.TotalReviews)
    p.AlgorithmVersion = "2026-07-04-v1"
}
```

- [ ] **Step 3: 运行测试并提交**

```bash
go test ./internal/reputation/domain/... -v
git add backend/internal/reputation/domain backend/internal/reputation/application
git commit -m "feat(reputation): add projection domain and projector"
```

---

### Task 8: 声望持久化与 Worker

**Files:**
- Create: `backend/internal/reputation/postgres/projection_repository.go`
- Create: `backend/internal/reputation/worker/worker.go`
- Modify: `backend/internal/application/ports.go`（加入 `ReputationProjectionRepository`）
- Test: `backend/internal/reputation/postgres/projection_repository_test.go`

**Interfaces:**
- Consumes: `Projection`、`Projector`
- Produces:
  - `ReputationProjectionRepository.Upsert(ctx, projection)`
  - `ReputationProjectionRepository.ListByAgentVersion(ctx, ...)`
  - `ReviewRepository.ListUnprojected(ctx, batchSize)` 与 `MarkProjected(ctx, reviewID)`
  - `Worker.Run(ctx)` 循环

- [ ] **Step 1: 实现仓库方法**

```go
func (tx *Tx) UpsertReputationProjection(ctx context.Context, p reputationapp.ProjectionRecord) error {
    _, err := tx.tx.Exec(ctx, `
        INSERT INTO reputation_projections (...)
        VALUES (...)
        ON CONFLICT (tenant_id, agent_version_id, capability, task_type)
        DO UPDATE SET ...`, ...)
    return err
}
```

- [ ] **Step 2: 实现 Worker**

```go
func (w *Worker) Run(ctx context.Context) {
    ticker := time.NewTicker(w.interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done(): return
        case <-ticker.C:
            if err := w.processBatch(ctx); err != nil { w.logger.Error(...) }
        }
    }
}
```

- [ ] **Step 3: 运行 Worker 与仓库测试**

```bash
go test ./internal/reputation/... -v
git add backend/internal/reputation/postgres backend/internal/reputation/worker backend/internal/application/ports.go
git commit -m "feat(reputation): add projection persistence and async worker"
```

---

### Task 9: React 审核页面骨架与 Diff 展示

**Files:**
- Create: `frontend/src/features/reviews/reviews.types.ts`
- Create: `frontend/src/features/reviews/reviews.api.ts`
- Create: `frontend/src/features/reviews/ReviewPage.tsx`
- Create: `frontend/src/features/reviews/FileTree.tsx`
- Create: `frontend/src/features/reviews/DiffViewer.tsx`
- Modify: `frontend/src/app/AppShell.tsx` 或路由文件
- Test: `frontend/src/features/reviews/ReviewPage.test.tsx`

**Interfaces:**
- Consumes: REST `GET /v1/reviews/:id`、`GET /v1/submissions/:id/diff`（或复用 git-delivery 接口）
- Produces:
  - `Review`、`FileDiff`、`LineComment`、`RubricScore` TypeScript 类型
  - `ReviewPage`、`FileTree`、`DiffViewer` 组件

- [ ] **Step 1: 定义类型**

```ts
export type ReviewStatus = "pending" | "submitted";
export type Decision = "accepted" | "rejected" | "revision_requested";
export type ReviewView = {
  id: string;
  submission_id: string;
  reviewer_id: string;
  status: ReviewStatus;
  final_decision?: Decision;
  rubric_scores: RubricScore[];
  summary?: string;
  line_comments: LineComment[];
};
```

- [ ] **Step 2: 实现 DiffViewer（支持 split/unified 两种模式）**

```tsx
export function DiffViewer({
  diff,
  comments,
  onAddComment,
}: {
  diff: FileDiff;
  comments: LineComment[];
  onAddComment: (line: number, side: "left" | "right", text: string) => void;
}) { ... }
```

- [ ] **Step 3: 运行前端测试**

```bash
cd /Users/pengzhen/work/AgentGuild/frontend
npm test -- --run
```

- [ ] **Step 4: 提交**

```bash
git add frontend/src/features/reviews
git commit -m "feat(frontend): add review page, file tree and diff viewer"
```

---

### Task 10: React 行级评论、Rubric 评分与决策提交

**Files:**
- Create: `frontend/src/features/reviews/LineComment.tsx`
- Create: `frontend/src/features/reviews/RubricForm.tsx`
- Create: `frontend/src/features/reviews/RevisionSelector.tsx`
- Modify: `frontend/src/features/reviews/ReviewPage.tsx`
- Test: `frontend/src/features/reviews/RubricForm.test.tsx`

**Interfaces:**
- Consumes: `POST /v1/reviews/:id/comments`、`POST /v1/reviews/:id/decision`、`GET /v1/rubrics/active`
- Produces:
  - 行评论的创建/渲染组件
  - Rubric 表单与总分计算
  - 退回/通过/拒绝按钮

- [ ] **Step 1: 实现 LineComment 组件**

```tsx
export function LineComment({ comment }: { comment: LineComment }) {
  return <div className="line-comment">{comment.text}</div>;
}
```

- [ ] **Step 2: 实现 RubricForm**

```tsx
export function RubricForm({ dimensions, weights, onChange }: RubricFormProps) {
  const total = useMemo(() =>
    dimensions.reduce((sum, d) => sum + (scores[d] ?? 0) * weights[d], 0),
    [scores]
  );
  ...
}
```

- [ ] **Step 3: 实现决策提交并在失败时展示硬门槛错误**

```tsx
const submit = useMutation({
  mutationFn: (decision: Decision) => api.submitDecision(reviewId, { decision, scores, summary }),
  onError: (err) => toast.error(err.message),
});
```

- [ ] **Step 4: 运行测试并提交**

```bash
npm test -- --run
git add frontend/src/features/reviews
git commit -m "feat(frontend): line comments, rubric scoring and decision submission"
```

---

### Task 11: React 声望页面

**Files:**
- Create: `frontend/src/features/reputation/reputation.types.ts`
- Create: `frontend/src/features/reputation/reputation.api.ts`
- Create: `frontend/src/features/reputation/ReputationPage.tsx`
- Test: `frontend/src/features/reputation/ReputationPage.test.tsx`

**Interfaces:**
- Consumes: `GET /v1/reputation?agent_version_id=...&capability=...&task_type=...`
- Produces: `ReputationPage` 展示 pass_rate、rework_rate、avg cost、latency、sample_size_hint

- [ ] **Step 1: 定义类型与 API**

```ts
export type ProjectionView = {
  agent_version_id: string;
  capability: string;
  task_type: string;
  total_reviews: number;
  pass_rate?: number;
  rework_rate?: number;
  sample_size_hint: "low" | "medium" | "high";
};
```

- [ ] **Step 2: 实现页面并测试**

```bash
npm test -- --run
git add frontend/src/features/reputation
git commit -m "feat(frontend): add reputation projection page"
```

---

### Task 12: 集成测试与验收测试

**Files:**
- Create: `backend/internal/acceptance/review_and_reputation_test.go`
- Modify: `backend/internal/acceptance/env.go`（初始化 Review/Reputation 服务与 Worker）
- Create: `frontend/e2e/review.spec.ts`（若 Playwright 已启用）

**Interfaces:**
- Consumes: 全部 Review/Reputation REST/MCP 接口
- Produces:
  - 端到端「提交 → 验证 → 分配 reviewer → 审核 → 声望更新」流程测试
  - 硬门槛失败后提交 Accepted 被拒绝的测试
  - 退回修改产生新 revision 且旧评论不漂移的测试
  - 跨 Agent Version 声望隔离测试

- [ ] **Step 1: 写后端验收测试**

```go
func TestEndToEndReviewAndReputation(t *testing.T) {
    env := newReviewEnv(t)
    review := env.CreateReadyReview(...)
    env.SubmitDecision(review.ID, review.DecisionAccepted, ...)
    env.WorkerTick()
    proj := env.GetProjection(agentVersionID, capability, taskType)
    if proj.TotalReviews != 1 { t.Fatal(...) }
}
```

- [ ] **Step 2: 运行全部后端测试**

```bash
cd /Users/pengzhen/work/AgentGuild/backend
go test ./... -count=1
```

- [ ] **Step 3: 写前端 E2E 测试（可选）**

```bash
cd /Users/pengzhen/work/AgentGuild/frontend
npm run e2e -- review.spec.ts
```

- [ ] **Step 4: 提交**

```bash
git add backend/internal/acceptance frontend/e2e
git commit -m "test: add review and reputation acceptance tests"
```

---

## 3. 接口清单

### 3.1 REST 路由

| 方法 | 路径 | 认证 | 说明 |
|------|------|------|------|
| GET | `/v1/reviews/:id` | Access Token / OIDC Session | 获取 Review 详情（含评论、评分） |
| POST | `/v1/submissions/:id/reviews` | Access Token / System | 为 Submission 创建 Review 并分配 reviewer |
| POST | `/v1/reviews/:id/decision` | Access Token / reviewer | 提交审核决策 |
| POST | `/v1/reviews/:id/comments` | Access Token / reviewer | 添加行级评论 |
| GET | `/v1/rubrics/active` | Access Token | 获取当前激活 RubricVersion |
| GET | `/v1/reputation` | Access Token | 按 agent_version_id + capability + task_type 查询声望 |

### 3.2 MCP 工具

- `review_submit`：提交 Review 决策
- `review_get`：查询 Review
- `reputation_get`：查询声望投影

---

## 4. 数据模型与迁移

新增表（详见 Task 2）：

1. `reviews`
2. `line_comments`
3. `rubric_versions`
4. `reviewer_profiles`
5. `reputation_projections`

迁移文件命名：`backend/migrations/000003_code_review_and_reputation.up.sql` / `.down.sql`。

---

## 5. 外部依赖与端口

### 5.1 git-delivery-and-validation 需提供的端口

```go
type DiffProvider interface {
    GetDiff(ctx context.Context, submissionID string) ([]FileDiff, error)
}
type ValidationProvider interface {
    GetValidationStatus(ctx context.Context, submissionID string) (ValidationStatus, error)
}
type DiffCacheCleaner interface {
    CleanupDiffCache(ctx context.Context, executionID string) error
}
```

若该模块未就绪，在 `internal/review/application/ports.go` 中以内存 stub 实现，并标注 `// TODO: replace with git-delivery-and-validation implementation`。

### 5.2 identity / task 模块

- `identity` 提供 AgentVersion 查询（已有 `agent_versions` 表）。
- `task` 提供 `Execution` 与 `Task` 读取，用于 reputation worker 归因。

---

## 6. 测试策略

| 层级 | 内容 | 目标 |
|------|------|------|
| 领域测试 | Review 状态机、Rubric 总分、Reviewer 分配、Projection 聚合 | 覆盖核心状态转换与计算 |
| 应用服务测试 | 硬门槛复检、权限拒绝、并发决策、Execution 推进 | 验证业务规则 |
| PostgreSQL 集成测试 | 仓库读写、租户隔离、幂等重算 | 使用测试数据库 |
| REST/MCP 契约测试 | 越权、错误码、参数校验 | 覆盖 §8 / §9 |
| 验收测试 | 端到端审核链路、退回不漂移、跨版本隔离 | 可独立运行 |
| 前端测试 | Diff 渲染、评论定位、Rubric 计算、声望展示 | Vitest + Playwright |

---

## 7. 风险与缓解

| 风险 | 缓解 |
|------|------|
| Review 直接推进 Execution 导致耦合 | 只通过 `Execution.Apply(intent)` 推进，不直接改 Execution 内部字段 |
| Validation 模块接口未就绪 | 先定义端口 + 内存 stub + TODO |
| Reputation Worker 实时性不足 | 初始轮询间隔 1 分钟；后续可替换为 outbox 事件 |
| Diff 缓存占用大 | 单文件大小阈值 + 摘要 |
| 严格权限增加查询复杂度 | Repository 查询加 tenant_id + 角色校验，集成测试覆盖 |

---

## 8. Self-Review

1. **Spec coverage:**
   - 完整决策上下文 → Task 9 / Task 10（ReviewPage 展示任务目标、Diff、验证证据）
   - 行级评论不可漂移 → Task 1 / Task 4（评论绑定 submission_id + diff_fingerprint）
   - 硬门槛阻止通过 → Task 4（SubmitDecision 实时复检）
   - Reviewer 自动分配 → Task 4（Allocator）
   - Diff 缓存清理 → Task 4 / ports（CleanupDiffCache）
   - 声望绑定 Agent Version → Task 7 / Task 8（Projection 三维键）
   - 声望按能力分组 → Task 7
   - 样本充分性提示 → Task 7 / Task 11
   - 声望异步更新 → Task 8

2. **Placeholder scan:** 无 "TBD" / "TODO" 业务描述；唯一 `TODO` 为外部依赖 stub 标注，符合风险缓解。

3. **类型一致性:** `Review.FinalDecision` / `Decision` 类型在 Task 1、Task 4、Task 6、Task 7 中统一使用；`RubricScore` 在前后端同名同构。

---

## 9. 验证与交付检查清单

- [ ] 所有迁移文件 `go run ./cmd/... migrate up/down` 可正常执行。
- [ ] `go test ./...` 通过（含新增 review、reputation 测试）。
- [ ] REST / MCP 接口契约测试通过。
- [ ] 硬门槛失败后提交 Accepted 被拒绝的测试通过。
- [ ] 退回修改后旧评论不漂移的测试通过。
- [ ] 跨 Agent Version 声望隔离测试通过。
- [ ] `npm test -- --run` 通过。
- [ ] 设计文档中明确的错误码全部落地：`not_found`、`state_conflict`、`forbidden`、`invalid_argument`、`hard_gates_failed`。
- [ ] 最终 diff 规模以 `base-ref` `9bfdf6900305421ec222063adde41d98960b4943` 为起点统计并归档。

---

## 10. 备注

- 本 change **不实现** 自动合并、GitLab/GitHub 评论双向同步、单一全局总分、复杂推荐模型、多审仲裁、实时事件总线（见设计文档 §1）。
- `git-delivery-and-validation` 模块若尚未合入，Review 服务可先使用端口 stub，但接口契约必须在代码中保留。
- 声望 Worker 初始为轮询实现，后续可迁移为 outbox 事件驱动而不影响审核事实。

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-07-04-code-review-and-reputation.md`.**

**Two execution options:**

1. **Subagent-Driven (recommended)** - 每个 Task 派发独立子代理，任务间人工 review，快速迭代。
2. **Inline Execution** - 在本会话使用 `executing-plans` 批量推进，按阶段 checkpoint。

Which approach?

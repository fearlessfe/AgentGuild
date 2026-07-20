package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Store coordinates transactions for the evaluation module.
type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

// Tx is the execution context passed to command handlers. It exposes the raw
// query surface so repositories can participate in the same transaction.
type Tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Now(context.Context) (time.Time, error)
}

// BenchmarkSetRepository persists benchmark sets and their task references.
type BenchmarkSetRepository interface {
	Create(context.Context, Tx, *domain.BenchmarkSet) error
	GetByID(context.Context, string, string) (*domain.BenchmarkSet, error)
	ListByTenant(context.Context, string) ([]domain.BenchmarkSet, error)
	SetActive(context.Context, Tx, string, string) error
	SetInactiveAll(context.Context, Tx, string) error
	GetActiveByTenant(context.Context, string) (*domain.BenchmarkSet, error)
	NextVersionNumber(context.Context, Tx, string) (int, error)
}

// EvaluationRunRepository persists evaluation runs and their per-task results.
type EvaluationRunRepository interface {
	Create(context.Context, Tx, *domain.EvaluationRun) error
	GetByID(context.Context, string, string) (*domain.EvaluationRun, error)
	ListByAgentVersion(context.Context, string, string) ([]domain.EvaluationRun, error)
	Complete(context.Context, Tx, *domain.EvaluationRun) error
	CreateResult(context.Context, Tx, *domain.EvaluationRunResult) error
	ListResults(context.Context, string, string) ([]domain.EvaluationRunResult, error)
	// ListRunning returns up to batchSize running runs across all tenants,
	// oldest first. The harvest worker uses it to find candidate runs.
	ListRunning(context.Context, int) ([]domain.EvaluationRun, error)
	// LockRunningForUpdate re-reads a run inside the worker transaction and
	// locks its row while it is still running; it returns domain.ErrNotFound
	// when the run is missing or was already completed by a concurrent tick.
	LockRunningForUpdate(context.Context, Tx, string, string) (*domain.EvaluationRun, error)
}

// VersionLifecyclePort abstracts the agentversion module so evaluation can
// transition a version to eligible or rejected without importing agentversion.
type VersionLifecyclePort interface {
	GetByID(context.Context, string, string) (*VersionInfo, error)
	MarkEvaluating(context.Context, Tx, string, string, string) error
	MarkEligible(context.Context, Tx, string, string, string) error
	MarkRejected(context.Context, Tx, string, string, string, string) error
}

// VersionInfo contains the minimal information needed by the evaluation
// module to interact with an AgentVersion.
type VersionInfo struct {
	ID       string
	TenantID string
	AgentID  string
	Status   string
}

// BenchmarkExecutor runs the benchmark tasks and returns raw task results. It
// is intentionally synchronous and injectable for the initial implementation.
type BenchmarkExecutor interface {
	// ExecutorID identifies the executor implementation in evaluation run
	// summaries so the provenance of the evidence stays visible.
	ExecutorID() string
	Execute(ctx context.Context, benchmarkSet *domain.BenchmarkSet, environmentDigest string) ([]domain.TaskResult, error)
}

// PublishEvaluationTaskCommand carries everything needed to publish one
// benchmark task as a real platform task.
type PublishEvaluationTaskCommand struct {
	TenantID     string
	RequestID    string
	Type         string
	Title        string
	Problem      string
	Constraints  []string
	Requirements []string
	Deadline     time.Time
}

// EvaluationTaskPublisher publishes a benchmark task as a real platform task.
// It is implemented in main.go by an adapter over the core task service, which
// runs in its own transaction; it must never be called inside the evaluation
// store transaction.
type EvaluationTaskPublisher interface {
	PublishEvaluationTask(ctx context.Context, cmd PublishEvaluationTaskCommand) (taskID string, err error)
}

// RunTaskRepository persists the mapping between an evaluation run's benchmark
// tasks and the real platform tasks published for them.
//
// The harvest worker (phase 2) drives the resolution lifecycle: ListUnresolved
// feeds the per-tick harvest loop, Resolve writes the harvested outcome, and
// ListByRunTx re-reads all rows inside the worker transaction so completion
// scoring sees resolutions written earlier in the same transaction.
type RunTaskRepository interface {
	Insert(context.Context, Tx, []domain.EvaluationRunTask) error
	ListByRun(context.Context, string, string) ([]domain.EvaluationRunTask, error)
	SetTaskID(context.Context, Tx, string, string, string, string) error
	RecordPublishFailure(context.Context, Tx, string, string, string, string) error
	ListUnresolved(context.Context, Tx, string, string) ([]domain.EvaluationRunTask, error)
	ListByRunTx(context.Context, Tx, string, string) ([]domain.EvaluationRunTask, error)
	Resolve(context.Context, Tx, string, string, string, RunTaskResolution) error
}

// RunTaskResolution is the outcome the harvest worker writes when resolving a
// run-task row. Details is merged into the row's existing details document.
type RunTaskResolution struct {
	Passed     bool
	LatencyMs  *float64
	CostCents  *int64
	Details    map[string]any
	ResolvedAt time.Time
}

// EvaluationTaskSnapshot is the read model the harvest worker needs to resolve
// one published evaluation task. Execution* fields describe the task's latest
// execution (at most one execution per task is active at a time) and are empty
// when the task was never claimed; SubmissionStatus is empty while no
// submission exists for that execution; ObservedCostCents is nil when no cost
// observation was recorded.
type EvaluationTaskSnapshot struct {
	TaskID               string
	TaskStatus           string
	ExecutionID          string
	ExecutionVersionID   string
	ExecutionStatus      string
	ExecutionStartedAt   *time.Time
	ExecutionSubmittedAt *time.Time
	SubmissionStatus     string
	ObservedCostCents    *int64
}

// EvaluationTaskObserver provides read-only snapshots of platform task state
// for the evaluation harvest worker. It is implemented in the evaluation
// module's own postgres layer with tenant-scoped queries over the core task,
// execution and submission tables; it must never mutate platform state.
type EvaluationTaskObserver interface {
	// Observe returns the snapshot for one task, or domain.ErrNotFound when the
	// task does not exist in the tenant.
	Observe(ctx context.Context, tenantID, taskID string) (*EvaluationTaskSnapshot, error)
}

// VersionEnvironmentProvider reads the environment digest frozen on an agent
// version, so an auto-started evaluation run inherits the version's
// environment provenance. It is implemented in the evaluation module's own
// postgres layer with tenant-scoped read-only queries (same precedent as
// EvaluationTaskObserver).
type VersionEnvironmentProvider interface {
	// GetVersionEnvironmentDigest returns the version's environment digest (""
	// when the version has none), or domain.ErrNotFound when the version does
	// not exist in the tenant.
	GetVersionEnvironmentDigest(ctx context.Context, tenantID, versionID string) (string, error)
}

// EvaluationService orchestrates benchmark set and evaluation run commands
// and queries.
type EvaluationService struct {
	store         Store
	benchmarkSets BenchmarkSetRepository
	runs          EvaluationRunRepository
	versions      VersionLifecyclePort
	executor      BenchmarkExecutor
	policy        *Policy
	newID         func() string
	// taskPublisher, runTasks and taskDeadline are only set for the platform
	// executor: StartEvaluationRun then publishes real tasks asynchronously and
	// leaves the run running instead of completing it synchronously.
	taskPublisher EvaluationTaskPublisher
	runTasks      RunTaskRepository
	taskDeadline  time.Duration
}

// EvaluationOptions configures the evaluation service.
type EvaluationOptions struct {
	NewID func() string
	// TaskPublisher and RunTasks enable the platform execution path; they must
	// be set together or not at all. TaskDeadline bounds how long a published
	// evaluation task stays open and defaults to two hours.
	TaskPublisher EvaluationTaskPublisher
	RunTasks      RunTaskRepository
	TaskDeadline  time.Duration
}

// Command DTOs

type CreateBenchmarkSet struct {
	TenantID    string
	Name        string
	Description string
	Tasks       []domain.BenchmarkTask
	IsActive    bool
	CreatedBy   string
	IsAdmin     bool
}

type CreateBenchmarkSetResponse struct {
	BenchmarkSet *domain.BenchmarkSet
}

type StartEvaluationRun struct {
	TenantID           string
	AgentID            string
	VersionID          string
	BenchmarkSetID     string
	EnvironmentDigest  string
	ScoringRuleVersion string
	ActorID            string
	IsAdmin            bool
}

type StartEvaluationRunResponse struct {
	EvaluationRun *domain.EvaluationRun
}

type CompleteEvaluationRun struct {
	TenantID         string
	AgentID          string
	VersionID        string
	EvaluationRunID  string
	ThresholdResults []domain.ThresholdResult
	Summary          domain.EvaluationSummary
	ActorID          string
	IsAdmin          bool
}

// EvaluationRunResult is a per-task result persisted for auditing.
type EvaluationRunResult = domain.EvaluationRunResult

// Query DTOs

type BenchmarkSetSummary struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenant_id"`
	VersionNumber int       `json:"version_number"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	IsActive      bool      `json:"is_active"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}

type EvaluationRunSummary struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"tenant_id"`
	AgentVersionID     string     `json:"agent_version_id"`
	BenchmarkSetID     string     `json:"benchmark_set_id"`
	Status             string     `json:"status"`
	EnvironmentDigest  string     `json:"environment_digest"`
	ScoringRuleVersion string     `json:"scoring_rule_version"`
	StartedAt          time.Time  `json:"started_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

type EvaluationRunDetail struct {
	ID                 string                       `json:"id"`
	TenantID           string                       `json:"tenant_id"`
	AgentVersionID     string                       `json:"agent_version_id"`
	BenchmarkSetID     string                       `json:"benchmark_set_id"`
	Status             string                       `json:"status"`
	EnvironmentDigest  string                       `json:"environment_digest"`
	ScoringRuleVersion string                       `json:"scoring_rule_version"`
	ThresholdResults   []domain.ThresholdResult     `json:"threshold_results,omitempty"`
	Summary            domain.EvaluationSummary     `json:"summary"`
	TaskResults        []domain.EvaluationRunResult `json:"task_results,omitempty"`
	StartedAt          time.Time                    `json:"started_at"`
	CompletedAt        *time.Time                   `json:"completed_at,omitempty"`
}

type BenchmarkSetPage struct {
	Items []BenchmarkSetSummary `json:"items"`
}

type EvaluationRunPage struct {
	Items []EvaluationRunDetail `json:"items"`
}

// AgentOwnerProvider reads the owner of an agent.
type AgentOwnerProvider interface {
	GetAgentOwner(context.Context, string, string) (string, error)
}

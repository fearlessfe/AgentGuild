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
	Execute(ctx context.Context, benchmarkSet *domain.BenchmarkSet, environmentDigest string) ([]domain.TaskResult, error)
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
}

// EvaluationOptions configures the evaluation service.
type EvaluationOptions struct {
	NewID func() string
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
	ID                 string                   `json:"id"`
	TenantID           string                   `json:"tenant_id"`
	AgentVersionID     string                   `json:"agent_version_id"`
	BenchmarkSetID     string                   `json:"benchmark_set_id"`
	Status             string                   `json:"status"`
	EnvironmentDigest  string                   `json:"environment_digest"`
	ScoringRuleVersion string                   `json:"scoring_rule_version"`
	ThresholdResults   []domain.ThresholdResult `json:"threshold_results,omitempty"`
	Summary            domain.EvaluationSummary `json:"summary"`
	StartedAt          time.Time                `json:"started_at"`
	CompletedAt        *time.Time               `json:"completed_at,omitempty"`
}

// AgentOwnerProvider reads the owner of an agent.
type AgentOwnerProvider interface {
	GetAgentOwner(context.Context, string, string) (string, error)
}

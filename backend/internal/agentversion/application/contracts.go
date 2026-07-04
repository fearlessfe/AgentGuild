package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Store coordinates transactions for the agentversion module.
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

// VersionRepository persists AgentVersion snapshots and agent row locks.
type VersionRepository interface {
	Create(context.Context, Tx, *domain.AgentVersion) error
	UpdateStatus(context.Context, Tx, *domain.AgentVersion) error
	GetByID(context.Context, string, string, string) (*domain.AgentVersion, error)
	ListByAgent(context.Context, string, string) ([]domain.AgentVersion, error)
	GetLatestByAgent(context.Context, string, string) (*domain.AgentVersion, error)
	GetActiveByAgent(context.Context, string, string) (*domain.AgentVersion, error)
	LockAgent(context.Context, Tx, string, string) error
	UpdateAgentCurrentVersion(context.Context, Tx, string, string, string) error
	GetAgentOwner(context.Context, string, string) (string, error)
}

// EvaluationRunProvider abstracts the evaluation module so promotion can verify
// that the target version has a passed evaluation run.
type EvaluationRunProvider interface {
	GetLatestPassed(context.Context, Tx, string, string) (*EvaluationRunInfo, error)
}

// EvaluationRunInfo contains the minimal information needed by the version
// lifecycle service.
type EvaluationRunInfo struct {
	ID     string
	Status string
}

// VersionService orchestrates version lifecycle commands and queries.
type VersionService struct {
	store         Store
	versions      VersionRepository
	evalProvider  EvaluationRunProvider
	policy        *Policy
	newID         func() string
}

// VersionOptions configures the version service.
type VersionOptions struct {
	NewID func() string
}

// Command DTOs

type CreateDraft struct {
	TenantID          string
	AgentID           string
	CreatedBy         string
	IsAdmin           bool
	Runtime           string
	Model             string
	Capabilities      []string
	PromptRef         string
	SkillRefs         []string
	MemoryRef         string
	ToolRefs          []string
	EnvironmentDigest string
}

type CreateDraftResponse struct {
	Version *domain.AgentVersion
}

type Promote struct {
	TenantID  string
	AgentID   string
	VersionID string
	ActorID   string
	IsAdmin   bool
}

type Rollback struct {
	TenantID  string
	AgentID   string
	VersionID string
	ActorID   string
	IsAdmin   bool
}

type StartEvaluation struct {
	TenantID  string
	AgentID   string
	VersionID string
	ActorID   string
}

// Query DTOs

type VersionSummary struct {
	ID                string
	VersionNumber     int
	Status            domain.VersionStatus
	ParentVersionID   string
	ConfigFingerprint string
	CreatedAt         time.Time
	PromotedAt        *time.Time
	RetiredAt         *time.Time
}

type VersionDetail struct {
	ID                string
	TenantID          string
	AgentID           string
	VersionNumber     int
	ParentVersionID   string
	Status            domain.VersionStatus
	Runtime           string
	Model             string
	Capabilities      []string
	ConfigFingerprint string
	ContentHash       string
	EnvironmentDigest string
	PromptRef         string
	SkillRefs         []string
	MemoryRef         string
	ToolRefs          []string
	CreatedBy         string
	CreatedAt         time.Time
	PromotedAt        *time.Time
	RetiredAt         *time.Time
}

type RefChange struct {
	From string
	To   string
}

type VersionDiff struct {
	BaseVersionID string
	Added         map[string]RefChange
	Removed       map[string]RefChange
	Changed       map[string]RefChange
}

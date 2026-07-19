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
	UpdateContent(context.Context, Tx, *domain.AgentVersion) error
	GetByID(context.Context, string, string, string) (*domain.AgentVersion, error)
	ListByAgent(context.Context, string, string) ([]domain.AgentVersion, error)
	GetLatestByAgent(context.Context, string, string) (*domain.AgentVersion, error)
	GetActiveByAgent(context.Context, string, string) (*domain.AgentVersion, error)
	LockAgent(context.Context, Tx, string, string) error
	UpdateAgentCurrentVersion(context.Context, Tx, string, string, string) error
	// GetAgentCurrentVersionID returns the agent's current_version_id, or ""
	// when the agent has no current version. It returns domain.ErrNotFound
	// when the agent does not exist.
	GetAgentCurrentVersionID(context.Context, string, string) (string, error)
	// PromoteAgentCurrentVersion sets the agent's current_version_id only when
	// it still equals expectedVersionID ("" matches NULL). It returns
	// domain.ErrStateConflict when the current version changed concurrently.
	PromoteAgentCurrentVersion(ctx context.Context, tx Tx, tenantID, agentID, versionID, expectedVersionID string) error
	GetAgentOwner(context.Context, string, string) (string, error)
}

// EvaluationRunProvider abstracts the evaluation module so promotion can verify
// that the target version has a passed evaluation run.
type EvaluationRunProvider interface {
	GetLatestPassed(context.Context, Tx, string, string) (*EvaluationRunInfo, error)
}

// ExperienceCandidateProvider abstracts the agentexperience module so a new
// draft can bind approved experience evidence references.
type ExperienceCandidateProvider interface {
	ListApprovedByAgent(ctx context.Context, tenantID, agentID string) ([]ExperienceCandidateRef, error)
	ListApprovedByAgentTx(ctx context.Context, tx Tx, tenantID, agentID string) ([]ExperienceCandidateRef, error)
}

// ExperienceCandidateRef contains the minimal information needed to bind an
// approved experience candidate to a new version.
type ExperienceCandidateRef struct {
	ID          string
	EvidenceRef string
}

// EvaluationRunInfo contains the minimal information needed by the version
// lifecycle service.
type EvaluationRunInfo struct {
	ID     string
	Status string
}

// VersionService orchestrates version lifecycle commands and queries.
type VersionService struct {
	store        Store
	versions     VersionRepository
	evalProvider EvaluationRunProvider
	xpProvider   ExperienceCandidateProvider
	policy       *Policy
	newID        func() string
}

// VersionOptions configures the version service.
type VersionOptions struct {
	NewID func() string
}

// Command DTOs

type CreateDraft struct {
	TenantID              string
	AgentID               string
	CreatedBy             string
	IsAdmin               bool
	Runtime               string
	Model                 string
	Capabilities          []string
	PromptRef             string
	SkillRefs             []string
	MemoryRef             string
	ToolRefs              []string
	EnvironmentDigest     string
	ApprovedExperienceIDs []string
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
	ID                string               `json:"id"`
	VersionNumber     int                  `json:"version_number"`
	Status            domain.VersionStatus `json:"status"`
	ParentVersionID   string               `json:"parent_version_id,omitempty"`
	ConfigFingerprint string               `json:"config_fingerprint"`
	CreatedAt         time.Time            `json:"created_at"`
	PromotedAt        *time.Time           `json:"promoted_at,omitempty"`
	RetiredAt         *time.Time           `json:"retired_at,omitempty"`
}

type VersionDetail struct {
	ID                string               `json:"id"`
	TenantID          string               `json:"tenant_id"`
	AgentID           string               `json:"agent_id"`
	VersionNumber     int                  `json:"version_number"`
	ParentVersionID   string               `json:"parent_version_id,omitempty"`
	Status            domain.VersionStatus `json:"status"`
	Runtime           string               `json:"runtime"`
	Model             string               `json:"model"`
	Capabilities      []string             `json:"capabilities,omitempty"`
	ConfigFingerprint string               `json:"config_fingerprint"`
	ContentHash       string               `json:"content_hash"`
	EnvironmentDigest string               `json:"environment_digest"`
	PromptRef         string               `json:"prompt_ref,omitempty"`
	SkillRefs         []string             `json:"skill_refs,omitempty"`
	MemoryRef         string               `json:"memory_ref,omitempty"`
	ToolRefs          []string             `json:"tool_refs,omitempty"`
	CreatedBy         string               `json:"created_by"`
	CreatedAt         time.Time            `json:"created_at"`
	PromotedAt        *time.Time           `json:"promoted_at,omitempty"`
	PromotedBy        string               `json:"promoted_by,omitempty"`
	RetiredAt         *time.Time           `json:"retired_at,omitempty"`
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

type VersionPage struct {
	Items []VersionSummary `json:"items"`
}

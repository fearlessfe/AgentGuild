package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Store coordinates transactions for the agentexperience module.
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

// ExperienceCandidateRepository persists ExperienceCandidate records.
type ExperienceCandidateRepository interface {
	Create(context.Context, Tx, *domain.ExperienceCandidate) error
	GetByID(context.Context, string, string, string) (*domain.ExperienceCandidate, error)
	ListByAgent(context.Context, string, string) ([]domain.ExperienceCandidate, error)
	ListByAgentAndStatus(context.Context, string, string, domain.CandidateStatus) ([]domain.ExperienceCandidate, error)
	UpdateStatus(context.Context, Tx, *domain.ExperienceCandidate) error
	ListApprovedByAgent(context.Context, string, string) ([]domain.ExperienceCandidate, error)
	ListApprovedByAgentTx(context.Context, Tx, string, string) ([]domain.ExperienceCandidate, error)
}

// SubmissionStore abstracts the git-delivery-and-validation / code-review
// modules. Production wiring uses the PostgreSQL-backed implementation in
// this module's postgres package; FixedSubmissionStore remains for tests.
type SubmissionStore interface {
	GetAcceptedSubmission(ctx context.Context, tenantID, submissionID string) (*Submission, error)
}

// ExecutionStore reads executions that provide capabilities and version binding.
type ExecutionStore interface {
	GetExecution(ctx context.Context, tenantID, executionID string) (*Execution, error)
}

// Submission represents an accepted submission from which experience can be
// extracted.
type Submission struct {
	ID          string
	TenantID    string
	AgentID     string
	TaskID      string
	ReviewID    string
	ExecutionID string
	Status      string
}

// Execution represents a task execution that produced evidence.
type Execution struct {
	ID             string
	TenantID       string
	AgentID        string
	AgentVersionID string
	TaskID         string
	Capabilities   []string
}

// AgentOwnerProvider reads the owner of an agent.
type AgentOwnerProvider interface {
	GetAgentOwner(context.Context, string, string) (string, error)
}

// CandidateService orchestrates experience candidate commands and queries.
type CandidateService struct {
	store       Store
	candidates  ExperienceCandidateRepository
	submissions SubmissionStore
	executions  ExecutionStore
	policy      *Policy
	classifier  domain.SensitivityPolicy
	newID       func() string
}

// CandidateOptions configures the candidate service.
type CandidateOptions struct {
	NewID func() string
}

// Command DTOs

type ExtractCandidate struct {
	TenantID      string
	AgentID       string
	SubmissionID  string
	EvidenceBytes []byte
	CreatedBy     string
	IsAdmin       bool
}

type ExtractCandidateResponse struct {
	Candidate *domain.ExperienceCandidate
}

type ReviewCandidate struct {
	TenantID    string
	AgentID     string
	CandidateID string
	Action      string // "approve" or "reject"
	Reason      string
	ReviewerID  string
	IsAdmin     bool
}

// Query DTOs

type CandidateSummary struct {
	ID                     string     `json:"id"`
	TenantID               string     `json:"tenant_id"`
	AgentID                string     `json:"agent_id"`
	SourceTaskID           string     `json:"source_task_id,omitempty"`
	SourceSubmissionID     string     `json:"source_submission_id,omitempty"`
	SourceReviewID         string     `json:"source_review_id,omitempty"`
	EvidenceRef            string     `json:"evidence_ref"`
	ContentHash            string     `json:"content_hash"`
	ApplicableCapabilities []string   `json:"applicable_capabilities,omitempty"`
	TenantScope            string     `json:"tenant_scope"`
	SensitivityClass       string     `json:"sensitivity_class"`
	Status                 string     `json:"status"`
	PolicyReason           string     `json:"policy_reason,omitempty"`
	ReviewedBy             string     `json:"reviewed_by,omitempty"`
	ReviewedAt             *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
}

type ExperienceCandidatePage struct {
	Items []CandidateSummary `json:"items"`
}

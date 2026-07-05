package application

import (
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

// CredentialService issues and revokes short-lived, execution-scoped Git
// credentials through a CredentialIssuer while persisting only metadata.
type CredentialService struct {
	store    Store
	issuer   git.CredentialIssuer
	provider string
	newID    func() string
}

// Options configures a CredentialService.
type Options struct {
	Issuer   git.CredentialIssuer
	Provider string
	NewID    func() string
}

// NewCredentialService creates a CredentialService.
func NewCredentialService(store Store, options Options) (*CredentialService, error) {
	if store == nil {
		return nil, invalid("store")
	}
	if options.Issuer == nil {
		return nil, invalid("issuer")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	provider := options.Provider
	if provider == "" {
		provider = "github"
	}
	return &CredentialService{
		store:    store,
		issuer:   options.Issuer,
		provider: provider,
		newID:    options.NewID,
	}, nil
}

// Principal identifies the actor requesting a credential operation.
type Principal struct {
	TenantID       string
	OwnerID        string
	OwnerEmail     string
	IsAdmin        bool
	AgentID        string
	AgentVersionID string
	Scopes         []string
	RepoScope      []string
}

// Envelope wraps a response with transaction-level metadata.
type Envelope[T any] struct {
	Data T   `json:"data"`
	Meta Meta `json:"meta"`
}

// Meta carries metadata about the response.
type Meta struct {
	ServerTime time.Time `json:"server_time"`
}

// CredentialRecord is the persistent metadata for a Git credential. It MUST
// NOT contain the token plaintext.
type CredentialRecord struct {
	ID          string
	TenantID    string
	ExecutionID string
	Provider    string
	RepoURL     string
	Branch      string
	BaseCommit  string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	Status      gitdomain.CredentialStatus
	CreatedAt   time.Time
}

// CredentialView is the public shape of a persisted credential.
type CredentialView struct {
	ID          string
	TenantID    string
	ExecutionID string
	Provider    string
	RepoURL     string
	Branch      string
	BaseCommit  string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	Status      gitdomain.CredentialStatus
	CreatedAt   time.Time
}

// IssueCredentialResponse returns the plaintext token exactly once together
// with the persisted credential metadata.
type IssueCredentialResponse struct {
	Credential CredentialView
	Token      string
}

// SubmissionService creates and queries code submissions.
type SubmissionService struct {
	store    Store
	verifier *CommitVerifier
	newID    func() string
}

// NewSubmissionService creates a SubmissionService.
func NewSubmissionService(store Store, verifier *CommitVerifier, newID func() string) (*SubmissionService, error) {
	if store == nil {
		return nil, invalid("store")
	}
	if verifier == nil {
		return nil, invalid("verifier")
	}
	if newID == nil {
		newID = randomID
	}
	return &SubmissionService{store: store, verifier: verifier, newID: newID}, nil
}

// CreateSubmission creates a new submission for the current execution.
type CreateSubmission struct {
	RequestID      string
	ExecutionID    string
	TaskID         string
	Repo           string
	Branch         string
	CommitSHA      string
	BaseCommitSHA  string
	Summary        string
	Tests          *string
	Evidence       []byte
	AllowedPaths   []string
	ForbiddenPaths []string
}

// GetSubmission retrieves a submission by ID.
type GetSubmission struct {
	SubmissionID string
}

// SubmissionView is the public shape of a submission.
type SubmissionView struct {
	ID              string                    `json:"id"`
	TenantID        string                    `json:"tenant_id"`
	TaskID          string                    `json:"task_id"`
	ExecutionID     string                    `json:"execution_id"`
	Branch          string                    `json:"branch"`
	CommitSHA       string                    `json:"commit_sha"`
	BaseCommitSHA   string                    `json:"base_commit_sha"`
	Summary         string                    `json:"summary"`
	Tests           *string                   `json:"tests,omitempty"`
	Evidence        []byte                    `json:"evidence,omitempty"`
	DiffFingerprint string                    `json:"diff_fingerprint"`
	Status          gitdomain.SubmissionStatus `json:"status"`
	ValidationJobID *string                   `json:"validation_job_id,omitempty"`
	CreatedAt       time.Time                 `json:"created_at"`
	UpdatedAt       time.Time                 `json:"updated_at"`
}

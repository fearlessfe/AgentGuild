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
	Data T
	Meta Meta
}

// Meta carries metadata about the response.
type Meta struct {
	ServerTime time.Time
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

// SubmissionRecord is the persistent metadata for a code submission.
type SubmissionRecord struct {
	ID              string
	TenantID        string
	TaskID          string
	ExecutionID     string
	Branch          string
	CommitSHA       string
	BaseCommitSHA   string
	Summary         string
	TestDeclaration *string
	Evidence        []byte
	DiffFingerprint string
	Status          gitdomain.SubmissionStatus
	ValidationJobID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SubmissionView is the public shape of a persisted submission.
type SubmissionView struct {
	ID              string
	TenantID        string
	TaskID          string
	ExecutionID     string
	Branch          string
	CommitSHA       string
	BaseCommitSHA   string
	Summary         string
	TestDeclaration *string
	Evidence        []byte
	DiffFingerprint string
	Status          gitdomain.SubmissionStatus
	ValidationJobID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

package application

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

// GitHubAppService resolves per-tenant GitHub App configuration into drivers.
type GitHubAppService interface {
	Driver(ctx context.Context, tenantID string) (git.Driver, error)
}

// GitHubAppManager extends GitHubAppService with administrative operations.
type GitHubAppManager interface {
	GitHubAppService
	IssueSource(ctx context.Context, tenantID string) (git.IssueSource, error)
	IssueSourceForApp(ctx context.Context, tenantID, appID string) (git.IssueSource, error)
	Upsert(context.Context, UpsertGitHubApp) error
	Install(context.Context, string, int64) (GitHubAppView, error)
	InstallByID(context.Context, string, string, int64, string) (GitHubAppView, error)
	InstallationAccount(context.Context, string, string, int64) (string, error)
	Get(context.Context, string) (GitHubAppView, error)
	GetByID(context.Context, string, string) (GitHubAppView, error)
	List(context.Context, string) ([]GitHubAppView, error)
	Delete(context.Context, string) error
	DeleteByID(context.Context, string, string) error
	DriverForApp(context.Context, string, string) (git.Driver, error)
}

// CredentialService issues and revokes short-lived, execution-scoped Git
// credentials through a CredentialIssuer while persisting only metadata.
type CredentialService struct {
	store        Store
	resolver     RepositoryGitResolver
	authorizer   CredentialGrantAuthorizer
	provider     string
	newID        func() string
	proxyBaseURL string
	tokenSecret  []byte
}

// Options configures a CredentialService.
type Options struct {
	Provider     string
	NewID        func() string
	Authorizer   CredentialGrantAuthorizer
	ProxyBaseURL string
	TokenSecret  []byte
}

// NewCredentialService creates a CredentialService.
func NewCredentialService(store Store, resolver RepositoryGitResolver, options Options) (*CredentialService, error) {
	if store == nil {
		return nil, invalid("store")
	}
	if resolver == nil {
		return nil, invalid("repository_git_resolver")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	proxyBaseURL := strings.TrimRight(options.ProxyBaseURL, "/")
	proxyURL, err := url.Parse(proxyBaseURL)
	if err != nil || proxyURL.Host == "" || proxyURL.User != nil || proxyURL.RawQuery != "" || proxyURL.Fragment != "" {
		return nil, invalid("proxy_base_url")
	}
	loopback := proxyURL.Hostname() == "localhost"
	if ip := net.ParseIP(proxyURL.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	if proxyURL.Scheme != "https" && !(proxyURL.Scheme == "http" && loopback) {
		return nil, invalid("proxy_base_url")
	}
	if len(options.TokenSecret) < 32 {
		return nil, invalid("token_secret")
	}
	provider := options.Provider
	if provider == "" {
		provider = "github"
	}
	return &CredentialService{
		store:        store,
		resolver:     resolver,
		authorizer:   options.Authorizer,
		provider:     provider,
		newID:        options.NewID,
		proxyBaseURL: proxyBaseURL,
		tokenSecret:  append([]byte(nil), options.TokenSecret...),
	}, nil
}

// CredentialGrant contains the repository state that the server has bound to
// an execution. Callers must not construct it from untrusted request fields.
type CredentialGrant struct {
	Repo       string
	BaseCommit string
	ExpiresAt  time.Time
}

// CredentialGrantAuthorizer resolves an execution-scoped credential grant.
// Implementations must verify that the execution exists in the tenant, belongs
// to the current Agent Version, is in a credential-eligible state with a live
// lease, and that the requested repository/base match the task source.
type CredentialGrantAuthorizer interface {
	AuthorizeCredential(context.Context, Principal, IssueCredential, time.Time) (CredentialGrant, error)
}

// RepositoryBaseResolver freezes the canonical default-branch commit for a
// tenant-scoped onboarded repository.
type RepositoryBaseResolver interface {
	ResolveBaseCommit(context.Context, string, string) (string, error)
}

// CredentialGrantAuthorizerFunc adapts a function to CredentialGrantAuthorizer.
type CredentialGrantAuthorizerFunc func(context.Context, Principal, IssueCredential, time.Time) (CredentialGrant, error)

func (f CredentialGrantAuthorizerFunc) AuthorizeCredential(ctx context.Context, principal Principal, cmd IssueCredential, now time.Time) (CredentialGrant, error) {
	return f(ctx, principal, cmd, now)
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

// OnboardedRepositoryStore persists tenant-scoped repositories made available
// for task repository selection.
type OnboardedRepositoryStore interface {
	ListOnboardedRepositories(context.Context, string) ([]OnboardedRepositoryRecord, error)
	GetOnboardedRepositoryByFullName(context.Context, string, string) (*OnboardedRepositoryRecord, error)
	CreateOnboardedRepository(context.Context, *OnboardedRepositoryRecord) error
	UpsertOnboardedRepository(context.Context, *OnboardedRepositoryRecord) error
	DeleteOnboardedRepository(context.Context, string, string) error
}

// PublicRepositoryResolver resolves public repository metadata without using a
// tenant GitHub App installation.
type PublicRepositoryResolver interface {
	ResolvePublicRepository(context.Context, string) (git.Repository, error)
}

// OnboardedRepositoryRecord is the persistent tenant-scoped repository record.
type OnboardedRepositoryRecord struct {
	ID            string
	TenantID      string
	SourceType    string
	FullName      string
	DefaultBranch string
	Visibility    string
	GitHubAppID   string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Envelope wraps a response with transaction-level metadata.
type Envelope[T any] struct {
	Data T    `json:"data"`
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
	Repo        string
	RepoURL     string
	Branch      string
	BaseCommit  string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	Status      gitdomain.CredentialStatus
	RequestHash []byte
	TokenHash   []byte
	CreatedAt   time.Time
}

// CredentialView is the public shape of a persisted credential.
type CredentialView struct {
	ID          string                     `json:"id"`
	TenantID    string                     `json:"tenant_id"`
	ExecutionID string                     `json:"execution_id"`
	Provider    string                     `json:"provider"`
	RepoURL     string                     `json:"repo_url"`
	Branch      string                     `json:"branch"`
	BaseCommit  string                     `json:"base_commit"`
	ExpiresAt   time.Time                  `json:"expires_at"`
	RevokedAt   *time.Time                 `json:"revoked_at,omitempty"`
	Status      gitdomain.CredentialStatus `json:"status"`
	CreatedAt   time.Time                  `json:"created_at"`
}

// IssueCredentialResponse returns the proxy token together with persisted
// metadata. The same idempotency key can deterministically replay the token;
// plaintext is never stored.
type IssueCredentialResponse struct {
	Credential CredentialView `json:"credential"`
	Token      string         `json:"token"`
}

// SubmissionService creates and queries code submissions.
type SubmissionService struct {
	store      Store
	verifier   *CommitVerifier
	notifier   ExecutionNotifier
	authorizer SubmissionAuthorizer
	newID      func() string
}

// SubmissionGrant contains the immutable execution/task binding used to
// validate a submission. Request fields can only assert these values.
type SubmissionGrant struct {
	TaskID         string
	Repo           string
	BaseCommit     string
	AllowedPaths   []string
	ForbiddenPaths []string
}

// SubmissionAuthorizer validates ownership, execution state, lease and task
// repository constraints before any commit is inspected or persisted.
type SubmissionAuthorizer interface {
	AuthorizeSubmission(context.Context, Principal, CreateSubmission, time.Time) (SubmissionGrant, error)
}

type SubmissionAuthorizerFunc func(context.Context, Principal, CreateSubmission, time.Time) (SubmissionGrant, error)

func (f SubmissionAuthorizerFunc) AuthorizeSubmission(ctx context.Context, principal Principal, cmd CreateSubmission, now time.Time) (SubmissionGrant, error) {
	return f(ctx, principal, cmd, now)
}

// NewSubmissionService creates a SubmissionService.
func NewSubmissionService(store Store, verifier *CommitVerifier, notifier ExecutionNotifier, authorizer SubmissionAuthorizer, newID func() string) (*SubmissionService, error) {
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
	return &SubmissionService{store: store, verifier: verifier, notifier: notifier, authorizer: authorizer, newID: newID}, nil
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
	ConfigVersion  string
}

// GetSubmission retrieves a submission by ID.
type GetSubmission struct {
	SubmissionID string
}

// ListSubmissions retrieves all submissions for an execution in creation order.
type ListSubmissions struct {
	ExecutionID string
}

// CheckSubmissionIntegrity verifies the submission's commit is still reachable
// from the expected branch and marks the submission invalid if it was
// force-pushed away.
type CheckSubmissionIntegrity struct {
	SubmissionID string
}

// StepView is the public shape of a validation step.
type StepView struct {
	Step          string     `json:"step"`
	Status        string     `json:"status"`
	LogSummary    string     `json:"log_summary,omitempty"`
	ResourceUsage []byte     `json:"resource_usage,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

// ValidationJobView is the public shape of a validation job.
type ValidationJobView struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	SubmissionID  string     `json:"submission_id"`
	Status        string     `json:"status"`
	Attempt       int        `json:"attempt"`
	ClaimedUntil  *time.Time `json:"claimed_until,omitempty"`
	ClaimedBy     *string    `json:"claimed_by,omitempty"`
	ConfigVersion string     `json:"config_version"`
	Steps         []StepView `json:"steps"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// SubmissionView is the public shape of a submission.
type SubmissionView struct {
	ID              string                     `json:"id"`
	TenantID        string                     `json:"tenant_id"`
	TaskID          string                     `json:"task_id"`
	ExecutionID     string                     `json:"execution_id"`
	Repo            string                     `json:"repo"`
	Branch          string                     `json:"branch"`
	CommitSHA       string                     `json:"commit_sha"`
	BaseCommitSHA   string                     `json:"base_commit_sha"`
	Summary         string                     `json:"summary"`
	Tests           *string                    `json:"tests,omitempty"`
	Evidence        []byte                     `json:"evidence,omitempty"`
	DiffFingerprint string                     `json:"diff_fingerprint"`
	Status          gitdomain.SubmissionStatus `json:"status"`
	ValidationJobID *string                    `json:"validation_job_id,omitempty"`
	CreatedAt       time.Time                  `json:"created_at"`
	UpdatedAt       time.Time                  `json:"updated_at"`
}

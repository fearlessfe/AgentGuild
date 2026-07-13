package application

import (
	"context"
	"time"

	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

// Store begins a transaction against the credential repository.
type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

// GitHubAppRepository persists tenant-level GitHub App configuration.
type GitHubAppRepository interface {
	Upsert(context.Context, *GitHubAppRecord) error
	ListByTenant(context.Context, string) ([]GitHubAppRecord, error)
	GetByID(context.Context, string, string) (*GitHubAppRecord, error)
	GetDefault(context.Context, string) (*GitHubAppRecord, error)
	Delete(context.Context, string, string) error
}

var _ ExecutionNotifier = (*NopExecutionNotifier)(nil)

// Tx is the set of operations available inside one credential transaction.
type Tx interface {
	Credentials() CredentialRepository
	Submissions() SubmissionRepository
	ValidationJobs() ValidationJobRepository
	GitHubApps() GitHubAppRepository
	Now(context.Context) (time.Time, error)
}

// ValidationJobRepository persists validation jobs and their steps.
type ValidationJobRepository interface {
	Insert(context.Context, *gitdomain.ValidationJob) error
	GetByID(context.Context, string, string) (*gitdomain.ValidationJob, error)
	GetBySubmissionID(context.Context, string, string) (*gitdomain.ValidationJob, error)
	ClaimNextPending(context.Context, string, string, time.Time, time.Time) (*gitdomain.ValidationJob, error)
	Update(context.Context, *gitdomain.ValidationJob) error
	UpdateStep(context.Context, string, string, gitdomain.Step) error
}

// CredentialRepository persists credential metadata without the token
// plaintext.
type CredentialRepository interface {
	Insert(context.Context, *CredentialRecord) error
	GetByID(context.Context, string, string) (*CredentialRecord, error)
	GetByExecutionID(context.Context, string, string) (*CredentialRecord, error)
	Update(context.Context, *CredentialRecord) error
	Revoke(context.Context, string, string) error
}

// SubmissionRepository persists submission aggregates.
type SubmissionRepository interface {
	Save(context.Context, *gitdomain.Submission) error
	GetByID(context.Context, string, string) (*gitdomain.Submission, error)
	GetByExecutionID(context.Context, string, string) ([]*gitdomain.Submission, error)
}

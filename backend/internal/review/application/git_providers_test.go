package application_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	"github.com/stretchr/testify/require"
)

func TestGitDiffProviderReturnsStructuredCommitDiff(t *testing.T) {
	submissions := &providerSubmissionRepository{submission: &gitdomain.Submission{
		ID: "submission-1", TenantID: "tenant-1", Repo: "owner/repo",
		BaseCommitSHA: "base", CommitSHA: "head",
	}}
	driver := &providerDriver{files: []git.ChangedFile{{
		Filename: "main.go",
		Patch:    "@@ -2,2 +2,3 @@\n keep\n-old\n+new\n+extra",
	}}}
	provider, err := reviewapp.NewGitDiffProvider(submissions, &providerResolver{driver: driver})
	require.NoError(t, err)

	files, err := provider.GetDiff(context.Background(), "tenant-1", "submission-1")
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "main.go", files[0].Path)
	require.Len(t, files[0].Hunks, 1)
	hunk := files[0].Hunks[0]
	require.NotEmpty(t, hunk.HunkHash)
	require.Equal(t, 2, hunk.OldStart)
	require.Equal(t, 2, hunk.NewStart)
	require.Equal(t, []reviewapp.DiffLine{
		{Type: "context", Text: " keep", OldLine: 2, NewLine: 2},
		{Type: "remove", Text: "-old", OldLine: 3},
		{Type: "add", Text: "+new", NewLine: 3},
		{Type: "add", Text: "+extra", NewLine: 4},
	}, hunk.Lines)
	require.Equal(t, "tenant-1", submissions.tenantID)
	require.Equal(t, "owner/repo", driver.repo)
	require.Equal(t, "base", driver.base)
	require.Equal(t, "head", driver.head)
}

func TestGitValidationProviderRequiresSucceededHardGates(t *testing.T) {
	job := &gitdomain.ValidationJob{
		SubmissionID: "submission-1",
		Status:       gitdomain.ValidationStatusSucceeded,
		Steps: []gitdomain.Step{
			{Step: gitdomain.ValidationStepBuild, HardGate: true, Status: gitdomain.ValidationStepStatusSucceeded},
			{Step: gitdomain.ValidationStepStaticAnalysis, HardGate: false, Status: gitdomain.ValidationStepStatusFailed},
		},
	}
	provider, err := reviewapp.NewGitValidationProvider(&providerValidationJobRepository{job: job})
	require.NoError(t, err)
	status, err := provider.GetValidationStatus(context.Background(), "tenant-1", "submission-1")
	require.NoError(t, err)
	require.True(t, status.AllHardGatesPassed())

	job.Steps[0].Status = gitdomain.ValidationStepStatusSkipped
	status, err = provider.GetValidationStatus(context.Background(), "tenant-1", "submission-1")
	require.NoError(t, err)
	require.False(t, status.AllHardGatesPassed())
}

type providerSubmissionRepository struct {
	submission *gitdomain.Submission
	tenantID   string
}

func (r *providerSubmissionRepository) Save(context.Context, *gitdomain.Submission) error { return nil }
func (r *providerSubmissionRepository) GetByID(_ context.Context, tenantID, _ string) (*gitdomain.Submission, error) {
	r.tenantID = tenantID
	copy := *r.submission
	return &copy, nil
}
func (r *providerSubmissionRepository) GetByExecutionID(context.Context, string, string) ([]*gitdomain.Submission, error) {
	return nil, nil
}

type providerResolver struct{ driver git.Driver }

func (r *providerResolver) Driver(context.Context, string, string) (git.ResolvedDriver, error) {
	return git.ResolvedDriver{Driver: r.driver, FullName: "owner/repo"}, nil
}
func (*providerResolver) IssueSource(context.Context, string, string, string) (git.ResolvedIssueSource, error) {
	return git.ResolvedIssueSource{}, nil
}

type providerDriver struct {
	files            []git.ChangedFile
	repo, base, head string
}

func (*providerDriver) CreateCredential(context.Context, string, string, string) (git.Credential, error) {
	return git.Credential{}, nil
}
func (*providerDriver) GetCommit(context.Context, string, string) (git.Commit, error) {
	return git.Commit{}, nil
}
func (d *providerDriver) CompareCommits(_ context.Context, repo, base, head string) ([]git.ChangedFile, error) {
	d.repo, d.base, d.head = repo, base, head
	return d.files, nil
}
func (*providerDriver) IsAncestor(context.Context, string, string, string) (bool, error) {
	return true, nil
}

type providerValidationJobRepository struct{ job *gitdomain.ValidationJob }

func (*providerValidationJobRepository) Insert(context.Context, *gitdomain.ValidationJob) error {
	return nil
}
func (r *providerValidationJobRepository) GetByID(context.Context, string, string) (*gitdomain.ValidationJob, error) {
	return r.job, nil
}
func (r *providerValidationJobRepository) GetBySubmissionID(context.Context, string, string) (*gitdomain.ValidationJob, error) {
	return r.job, nil
}
func (*providerValidationJobRepository) ClaimNextPending(context.Context, string, string, time.Time, time.Time) (*gitdomain.ValidationJob, error) {
	return nil, nil
}
func (*providerValidationJobRepository) Update(context.Context, *gitdomain.ValidationJob) error {
	return nil
}
func (*providerValidationJobRepository) UpdateStep(context.Context, string, string, gitdomain.Step) error {
	return nil
}

var _ gitapp.SubmissionRepository = (*providerSubmissionRepository)(nil)
var _ gitapp.ValidationJobRepository = (*providerValidationJobRepository)(nil)

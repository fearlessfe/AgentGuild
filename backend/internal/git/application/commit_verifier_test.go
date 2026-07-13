package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/stretchr/testify/require"
)

func TestVerifyCommitPassesWhenAllChecksSucceed(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
		{base: "head-sha", head: "head-sha"}: true,
	}
	fixture.driver.compareFiles = []git.ChangedFile{
		{Filename: "src/main.go", Status: "modified"},
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
		AllowedPaths:  []string{"src/*"},
	})

	require.NoError(t, err)
}

func TestCommitVerifierUsesRepositoryBoundDriver(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha": {SHA: "head-sha"}, "agentguild/exec-1": {SHA: "head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
		{base: "head-sha", head: "head-sha"}: true,
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID: "tenant-1", ExecutionID: "exec-1", Repo: "acme/api",
		Branch: "agentguild/exec-1", CommitSHA: "head-sha", BaseCommitSHA: "base-sha",
	})
	require.NoError(t, err)
	_, err = fixture.verifier.ChangedFiles(context.Background(), "tenant-1", "acme/api", "base-sha", "head-sha")
	require.NoError(t, err)
	_, err = fixture.verifier.IsCommitReachable(context.Background(), "tenant-1", "acme/api", "agentguild/exec-1", "head-sha")
	require.NoError(t, err)
	require.Equal(t, []string{"tenant-1/acme/api", "tenant-1/acme/api", "tenant-1/acme/api"}, fixture.resolver.driverCalls)
}

func TestVerifyCommitRequiresFields(t *testing.T) {
	fixture := newVerifierFixture(t)
	base := application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	}

	cases := []struct {
		name string
		cmd  application.VerifyCommit
	}{
		{"tenant_id", func(c application.VerifyCommit) application.VerifyCommit { c.TenantID = ""; return c }(base)},
		{"execution_id", func(c application.VerifyCommit) application.VerifyCommit { c.ExecutionID = ""; return c }(base)},
		{"repo", func(c application.VerifyCommit) application.VerifyCommit { c.Repo = ""; return c }(base)},
		{"branch", func(c application.VerifyCommit) application.VerifyCommit { c.Branch = ""; return c }(base)},
		{"commit_sha", func(c application.VerifyCommit) application.VerifyCommit { c.CommitSHA = ""; return c }(base)},
		{"base_commit_sha", func(c application.VerifyCommit) application.VerifyCommit { c.BaseCommitSHA = ""; return c }(base)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := fixture.verifier.Verify(context.Background(), tc.cmd)
			require.Error(t, err)
			require.Equal(t, "invalid_argument", domain.CodeOf(err))
			require.Equal(t, tc.name, domain.FieldOf(err))
		})
	}
}

func TestVerifyCommitReturnsNotFoundWhenCommitMissing(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commitErr = git.ErrRepoNotFound

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "missing",
		BaseCommitSHA: "base-sha",
	})

	require.Error(t, err)
	require.Equal(t, "not_found", domain.CodeOf(err))
	require.Equal(t, "commit_sha", domain.FieldOf(err))
}

func TestVerifyCommitReturnsInvalidArgumentWhenBaseNotAncestor(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha": {SHA: "head-sha"},
		"base-sha": {SHA: "base-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: false,
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "base_commit_sha", domain.FieldOf(err))
}

func TestVerifyCommitReturnsInvalidArgumentWhenBaseMissing(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha": {SHA: "head-sha"},
	}
	fixture.driver.ancestorErr = git.ErrRepoNotFound

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "missing-base",
	})

	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "base_commit_sha", domain.FieldOf(err))
}

func TestVerifyCommitReturnsInvalidArgumentWhenBranchMissing(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha": {SHA: "head-sha"},
		"base-sha": {SHA: "base-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
	}
	fixture.driver.branchErr = git.ErrRepoNotFound

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "missing-branch",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "branch", domain.FieldOf(err))
}

func TestVerifyCommitReturnsInvalidArgumentWhenCommitNotOnBranch(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "branch-head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}:        true,
		{base: "head-sha", head: "branch-head-sha"}: false,
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "commit_sha", domain.FieldOf(err))
}

func TestVerifyCommitReturnsInvalidArgumentWhenPathOutsideAllowed(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
		{base: "head-sha", head: "head-sha"}: true,
	}
	fixture.driver.compareFiles = []git.ChangedFile{
		{Filename: "src/main.go", Status: "modified"},
		{Filename: "README.md", Status: "modified"},
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
		AllowedPaths:  []string{"src/*"},
	})

	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "changed_paths", domain.FieldOf(err))
	var pve *application.PathViolationError
	require.True(t, errors.As(err, &pve))
	require.Len(t, pve.Violations, 1)
	require.Equal(t, "README.md", pve.Violations[0].Path)
}

func TestVerifyCommitReturnsInvalidArgumentWhenPathForbidden(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
		{base: "head-sha", head: "head-sha"}: true,
	}
	fixture.driver.compareFiles = []git.ChangedFile{
		{Filename: "src/main.go", Status: "modified"},
		{Filename: "src/secrets.env", Status: "added"},
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:       "tenant-1",
		ExecutionID:    "exec-1",
		Repo:           "owner/repo",
		Branch:         "agentguild/exec-1",
		CommitSHA:      "head-sha",
		BaseCommitSHA:  "base-sha",
		AllowedPaths:   []string{"src/*"},
		ForbiddenPaths: []string{"src/*.env"},
	})

	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	var pve *application.PathViolationError
	require.True(t, errors.As(err, &pve))
	require.Len(t, pve.Violations, 1)
	require.Equal(t, "src/secrets.env", pve.Violations[0].Path)
}

func TestVerifyCommitAllowsAllPathsWhenAllowedEmptyAndNoForbidden(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
		{base: "head-sha", head: "head-sha"}: true,
	}
	fixture.driver.compareFiles = []git.ChangedFile{
		{Filename: "any/path/file.go", Status: "added"},
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.NoError(t, err)
}

func TestVerifyCommitReturnsSuccessWhenDuplicateCommitSHA(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.submissions.submissions = []*gitdomain.Submission{
		{TenantID: "tenant-1", ExecutionID: "exec-1", CommitSHA: "head-sha"},
	}

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.NoError(t, err)
}

func TestVerifyCommitPropagatesIsAncestorError(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha": {SHA: "head-sha"},
		"base-sha": {SHA: "base-sha"},
	}
	fixture.driver.ancestorErr = domain.ErrRateLimited

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.ErrorIs(t, err, domain.ErrRateLimited)
}

func TestVerifyCommitPropagatesCompareCommitsError(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
		{base: "head-sha", head: "head-sha"}: true,
	}
	fixture.driver.compareErr = domain.ErrRateLimited

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.ErrorIs(t, err, domain.ErrRateLimited)
}

func TestVerifyCommitPropagatesDriverErrors(t *testing.T) {
	fixture := newVerifierFixture(t)
	fixture.driver.commitErr = domain.ErrRateLimited

	err := fixture.verifier.Verify(context.Background(), application.VerifyCommit{
		TenantID:      "tenant-1",
		ExecutionID:   "exec-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
	})

	require.ErrorIs(t, err, domain.ErrRateLimited)
}

type verifierFixture struct {
	verifier    *application.CommitVerifier
	driver      *fakeDriver
	resolver    *fakeAppService
	submissions *fakeSubmissionRepository
}

func newVerifierFixture(t *testing.T) *verifierFixture {
	t.Helper()
	d := &fakeDriver{commits: map[string]git.Commit{}}
	s := &fakeSubmissionRepository{}
	r := &fakeAppService{driver: d}
	return &verifierFixture{
		verifier:    application.NewCommitVerifier(r, s),
		driver:      d,
		resolver:    r,
		submissions: s,
	}
}

type ancestorKey struct {
	base string
	head string
}

type fakeDriver struct {
	commits      map[string]git.Commit
	compareFiles []git.ChangedFile
	ancestors    map[ancestorKey]bool

	commitErr   error
	ancestorErr error
	branchErr   error
	compareErr  error
}

func (f *fakeDriver) CreateCredential(_ context.Context, _, _, _ string) (git.Credential, error) {
	return git.Credential{Token: "fake", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (f *fakeDriver) GetCommit(_ context.Context, _, sha string) (git.Commit, error) {
	if f.branchErr != nil && sha == "missing-branch" {
		return git.Commit{}, f.branchErr
	}
	if f.commitErr != nil {
		return git.Commit{}, f.commitErr
	}
	c, ok := f.commits[sha]
	if !ok {
		return git.Commit{}, git.ErrRepoNotFound
	}
	return c, nil
}

func (f *fakeDriver) CompareCommits(_ context.Context, _, _, _ string) ([]git.ChangedFile, error) {
	if f.compareErr != nil {
		return nil, f.compareErr
	}
	return f.compareFiles, nil
}

func (f *fakeDriver) IsAncestor(_ context.Context, _, base, head string) (bool, error) {
	if f.ancestorErr != nil {
		return false, f.ancestorErr
	}
	v, ok := f.ancestors[ancestorKey{base: base, head: head}]
	if !ok {
		return false, nil
	}
	return v, nil
}

type fakeSubmissionRepository struct {
	submissions []*gitdomain.Submission
}

func (r *fakeSubmissionRepository) Save(context.Context, *gitdomain.Submission) error {
	return nil
}

func (r *fakeSubmissionRepository) GetByID(context.Context, string, string) (*gitdomain.Submission, error) {
	return nil, git.ErrSubmissionNotFound
}

func (r *fakeSubmissionRepository) GetByExecutionID(context.Context, string, string) ([]*gitdomain.Submission, error) {
	return r.submissions, nil
}

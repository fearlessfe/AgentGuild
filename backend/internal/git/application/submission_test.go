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

func TestCreateSubmissionRequiresAgentWithExecuteScope(t *testing.T) {
	fixture := newSubmissionFixture(t)

	_, err := fixture.svc.CreateSubmission(context.Background(), application.Principal{}, newSubmissionCmd())
	require.ErrorIs(t, err, domain.ErrForbidden)

	_, err = fixture.svc.CreateSubmission(context.Background(), application.Principal{TenantID: "tenant-1", AgentID: "agent-1"}, newSubmissionCmd())
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestCreateSubmissionRequiresFields(t *testing.T) {
	fixture := newSubmissionFixture(t)
	base := newSubmissionCmd()

	cases := []struct {
		field  string
		mutate func(*application.CreateSubmission)
	}{
		{"execution_id", func(c *application.CreateSubmission) { c.ExecutionID = "" }},
		{"task_id", func(c *application.CreateSubmission) { c.TaskID = "" }},
		{"repo", func(c *application.CreateSubmission) { c.Repo = "" }},
		{"branch", func(c *application.CreateSubmission) { c.Branch = "" }},
		{"commit_sha", func(c *application.CreateSubmission) { c.CommitSHA = "" }},
		{"base_commit_sha", func(c *application.CreateSubmission) { c.BaseCommitSHA = "" }},
		{"summary", func(c *application.CreateSubmission) { c.Summary = "" }},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			cmd := base
			tc.mutate(&cmd)
			_, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), cmd)
			require.Error(t, err)
			require.Equal(t, "invalid_argument", domain.CodeOf(err))
			require.Equal(t, tc.field, domain.FieldOf(err))
		})
	}
}

func TestCreateSubmissionReturnsExistingSubmissionForDuplicateCommitSHA(t *testing.T) {
	fixture := newSubmissionFixture(t)

	first, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)

	second, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)
	require.Equal(t, first.Data.ID, second.Data.ID)
	require.Equal(t, first.Data.DiffFingerprint, second.Data.DiffFingerprint)
}

func TestCreateSubmissionRejectsCommitNotOnBranch(t *testing.T) {
	fixture := newSubmissionFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "branch-head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}:        true,
		{base: "head-sha", head: "branch-head-sha"}: false,
	}

	_, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "commit_sha", domain.FieldOf(err))
}

func TestCreateSubmissionRejectsForbiddenPath(t *testing.T) {
	fixture := newSubmissionFixture(t)
	fixture.driver.commits = map[string]git.Commit{
		"head-sha":          {SHA: "head-sha"},
		"base-sha":          {SHA: "base-sha"},
		"agentguild/exec-1": {SHA: "head-sha"},
	}
	fixture.driver.ancestors = map[ancestorKey]bool{
		{base: "base-sha", head: "head-sha"}: true,
		{base: "head-sha", head: "head-sha"}: true,
	}
	fixture.driver.compareFiles = []git.ChangedFile{{Filename: "README.md", Status: "modified"}}

	cmd := newSubmissionCmd()
	cmd.AllowedPaths = []string{"src/*"}
	_, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), cmd)
	require.Error(t, err)
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "changed_paths", domain.FieldOf(err))
}

func TestCreateSubmissionComputesDiffFingerprint(t *testing.T) {
	fixture := newSubmissionFixture(t)
	cmd := newSubmissionCmd()

	got, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), cmd)
	require.NoError(t, err)
	require.NotEmpty(t, got.Data.DiffFingerprint)
	require.Equal(t, gitdomain.SubmissionStatusPendingVerification, got.Data.Status)
	require.Equal(t, "tenant-1", got.Data.TenantID)
	require.Equal(t, "task-1", got.Data.TaskID)
	require.Equal(t, "exec-1", got.Data.ExecutionID)
	require.NotNil(t, got.Data.ValidationJobID)

	job := fixture.store.validationJobs[*got.Data.ValidationJobID]
	require.NotNil(t, job)
	require.Equal(t, got.Data.ID, job.SubmissionID)
	require.Equal(t, gitdomain.ValidationStatusPending, job.Status)
	require.Equal(t, "default", job.ConfigVersion)
	require.Len(t, job.Steps, len(gitdomain.DefaultValidationSteps))

	record := fixture.store.submissions[got.Data.ID]
	require.NotNil(t, record)
	require.Equal(t, got.Data.DiffFingerprint, record.DiffFingerprint)
}

func TestGetSubmissionRequiresAuthorizedCaller(t *testing.T) {
	fixture := newSubmissionFixture(t)

	_, err := fixture.svc.GetSubmission(context.Background(), application.Principal{}, application.GetSubmission{SubmissionID: "sub-1"})
	require.ErrorIs(t, err, domain.ErrForbidden)

	_, err = fixture.svc.GetSubmission(context.Background(), application.Principal{TenantID: "tenant-1"}, application.GetSubmission{SubmissionID: "sub-1"})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestGetSubmissionReturnsSubmission(t *testing.T) {
	fixture := newSubmissionFixture(t)
	created, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)

	got, err := fixture.svc.GetSubmission(context.Background(), agentPrincipal(), application.GetSubmission{SubmissionID: created.Data.ID})
	require.NoError(t, err)
	require.Equal(t, created.Data.ID, got.Data.ID)
}

func TestCheckSubmissionIntegrityMarksInvalidOnForcePush(t *testing.T) {
	fixture := newSubmissionFixture(t)
	created, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)

	// Simulate force-push: branch now points to a new head that does not
	// contain the submission commit.
	fixture.driver.commits["new-head"] = git.Commit{SHA: "new-head"}
	fixture.driver.commits["agentguild/exec-1"] = git.Commit{SHA: "new-head"}
	fixture.driver.ancestors[ancestorKey{base: "head-sha", head: "new-head"}] = false

	require.NoError(t, fixture.svc.CheckSubmissionIntegrity(context.Background(), agentPrincipal(), application.CheckSubmissionIntegrity{SubmissionID: created.Data.ID}))

	got, err := fixture.svc.GetSubmission(context.Background(), agentPrincipal(), application.GetSubmission{SubmissionID: created.Data.ID})
	require.NoError(t, err)
	require.Equal(t, gitdomain.SubmissionStatusInvalid, got.Data.Status)
}

func TestCheckSubmissionIntegrityKeepsValidWhenCommitReachable(t *testing.T) {
	fixture := newSubmissionFixture(t)
	created, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)

	// Branch moved forward but the submission commit is still an ancestor.
	fixture.driver.commits["new-head"] = git.Commit{SHA: "new-head"}
	fixture.driver.commits["agentguild/exec-1"] = git.Commit{SHA: "new-head"}
	fixture.driver.ancestors[ancestorKey{base: "head-sha", head: "new-head"}] = true

	require.NoError(t, fixture.svc.CheckSubmissionIntegrity(context.Background(), agentPrincipal(), application.CheckSubmissionIntegrity{SubmissionID: created.Data.ID}))

	got, err := fixture.svc.GetSubmission(context.Background(), agentPrincipal(), application.GetSubmission{SubmissionID: created.Data.ID})
	require.NoError(t, err)
	require.Equal(t, gitdomain.SubmissionStatusPendingVerification, got.Data.Status)
}

func TestGetSubmissionTenantIsolation(t *testing.T) {
	fixture := newSubmissionFixture(t)
	created, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)

	_, err = fixture.svc.GetSubmission(context.Background(), application.Principal{TenantID: "tenant-2", OwnerID: "owner-1"}, application.GetSubmission{SubmissionID: created.Data.ID})
	require.ErrorIs(t, err, git.ErrSubmissionNotFound)
}

type submissionFixture struct {
	svc      *application.SubmissionService
	store    *memoryStore
	driver   *fakeDriver
	verifier *application.CommitVerifier
	now      time.Time
}

func newSubmissionFixture(t *testing.T) *submissionFixture {
	return newSubmissionFixtureWithNotifier(t, nil)
}

func newSubmissionFixtureWithNotifier(t *testing.T, notifier application.ExecutionNotifier) *submissionFixture {
	t.Helper()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newMemoryStore(now)
	driver := &fakeDriver{
		commits: map[string]git.Commit{
			"head-sha":          {SHA: "head-sha"},
			"base-sha":          {SHA: "base-sha"},
			"agentguild/exec-1": {SHA: "head-sha"},
		},
		ancestors: map[ancestorKey]bool{
			{base: "base-sha", head: "head-sha"}: true,
			{base: "head-sha", head: "head-sha"}: true,
		},
		compareFiles: []git.ChangedFile{{Filename: "src/main.go", Status: "modified"}},
	}
	verifier := application.NewCommitVerifier(&fakeAppService{driver: driver}, &fakeSubmissionRepository{})
	svc, err := application.NewSubmissionService(store, verifier, notifier, sequenceIDs("sub-1"))
	require.NoError(t, err)
	return &submissionFixture{svc: svc, store: store, driver: driver, verifier: verifier, now: now}
}

func newSubmissionCmd() application.CreateSubmission {
	return application.CreateSubmission{
		ExecutionID:   "exec-1",
		TaskID:        "task-1",
		Repo:          "owner/repo",
		Branch:        "agentguild/exec-1",
		CommitSHA:     "head-sha",
		BaseCommitSHA: "base-sha",
		Summary:       "fix parser",
	}
}

func agentPrincipal() application.Principal {
	return application.Principal{TenantID: "tenant-1", AgentID: "agent-1", AgentVersionID: "agent-1", Scopes: []string{"tasks:execute"}}
}

func TestCreateSubmissionNotifiesExecutionSubmitted(t *testing.T) {
	recorder := &recordingNotifier{}
	fixture := newSubmissionFixtureWithNotifier(t, recorder)

	_, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)

	require.Len(t, recorder.calls, 1)
	require.Equal(t, "exec-1", recorder.calls[0].ExecutionID)
	require.Equal(t, domain.IntentSubmit, recorder.calls[0].Intent)
	require.Equal(t, "tenant-1", recorder.calls[0].TenantID)
	require.Equal(t, domain.ActorAgent, recorder.calls[0].Actor.Type)
	require.Equal(t, "agent-1", recorder.calls[0].Actor.ID)
}

func TestCreateSubmissionSucceedsWhenNotificationFails(t *testing.T) {
	fixture := newSubmissionFixtureWithNotifier(t, &failingNotifier{})

	got, err := fixture.svc.CreateSubmission(context.Background(), agentPrincipal(), newSubmissionCmd())
	require.NoError(t, err)
	require.NotEmpty(t, got.Data.ID)
	require.NotNil(t, got.Data.ValidationJobID)
}

type failingNotifier struct{}

func (f *failingNotifier) Notify(context.Context, application.ExecutionStateCommand, time.Time) error {
	return errors.New("notification failed")
}

type recordingNotifier struct {
	calls []application.ExecutionStateCommand
}

func (r *recordingNotifier) Notify(_ context.Context, cmd application.ExecutionStateCommand, _ time.Time) error {
	r.calls = append(r.calls, cmd)
	return nil
}

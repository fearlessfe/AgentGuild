package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

func TestNewSubmissionDefaultsToPendingVerification(t *testing.T) {
	sub, err := newSubmission()
	require.NoError(t, err)
	require.Equal(t, gitdomain.SubmissionStatusPendingVerification, sub.Status)
	require.Equal(t, "sub-1", sub.ID)
	require.NotEmpty(t, sub.DiffFingerprint)
	require.True(t, sub.CreatedAt.Equal(now))
	require.True(t, sub.UpdatedAt.Equal(now))
}

func TestNewSubmissionRequiresMandatoryFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*submissionArgs)
		field  string
	}{
		{"tenant_id", func(a *submissionArgs) { a.tenantID = "" }, "tenant_id"},
		{"task_id", func(a *submissionArgs) { a.taskID = "" }, "task_id"},
		{"execution_id", func(a *submissionArgs) { a.executionID = "" }, "execution_id"},
		{"repo", func(a *submissionArgs) { a.repo = "" }, "repo"},
		{"branch", func(a *submissionArgs) { a.branch = "" }, "branch"},
		{"commit_sha", func(a *submissionArgs) { a.commitSHA = "" }, "commit_sha"},
		{"base_commit_sha", func(a *submissionArgs) { a.baseCommitSHA = "" }, "base_commit_sha"},
		{"summary", func(a *submissionArgs) { a.summary = "" }, "summary"},
		{"now", func(a *submissionArgs) { a.now = time.Time{} }, "now"},
		{"new_id", func(a *submissionArgs) { a.newID = nil }, "new_id"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := defaultArgs()
			tc.mutate(&a)
			_, err := buildSubmission(a)
			require.Error(t, err)
			require.Equal(t, tc.field, domain.FieldOf(err))
			require.Equal(t, "invalid_argument", domain.CodeOf(err))
		})
	}
}

func TestDiffFingerprintIsDeterministicAndPathOrderIndependent(t *testing.T) {
	branch := "agentguild/exec-1"
	base := "base-abc"
	head := "head-def"
	pathsA := []string{"b.go", "a.go", "c/d.go"}
	pathsB := []string{"c/d.go", "a.go", "b.go"}

	fp1 := gitdomain.DiffFingerprint(branch, base, head, pathsA)
	fp2 := gitdomain.DiffFingerprint(branch, base, head, pathsB)
	require.Equal(t, fp1, fp2)

	fp3 := gitdomain.DiffFingerprint(branch, base, head, []string{"a.go"})
	require.NotEqual(t, fp1, fp3)
}

func TestDiffFingerprintIncludesBranchAndCommits(t *testing.T) {
	paths := []string{"a.go"}
	fp1 := gitdomain.DiffFingerprint("branch-a", "base", "head", paths)
	fp2 := gitdomain.DiffFingerprint("branch-b", "base", "head", paths)
	fp3 := gitdomain.DiffFingerprint("branch-a", "base2", "head", paths)
	fp4 := gitdomain.DiffFingerprint("branch-a", "base", "head2", paths)

	require.NotEqual(t, fp1, fp2)
	require.NotEqual(t, fp1, fp3)
	require.NotEqual(t, fp1, fp4)
}

func TestMarkValidatedTransitionsFromPending(t *testing.T) {
	sub, err := newSubmission()
	require.NoError(t, err)

	later := now.Add(time.Minute)
	require.NoError(t, sub.MarkValidated(later))
	require.Equal(t, gitdomain.SubmissionStatusValidated, sub.Status)
	require.True(t, sub.UpdatedAt.Equal(later))
}

func TestMarkValidationFailedTransitionsFromPending(t *testing.T) {
	sub, err := newSubmission()
	require.NoError(t, err)

	require.NoError(t, sub.MarkValidationFailed(now.Add(time.Minute)))
	require.Equal(t, gitdomain.SubmissionStatusValidationFailed, sub.Status)
}

func TestMarkInvalidTransitionsFromPending(t *testing.T) {
	sub, err := newSubmission()
	require.NoError(t, err)

	require.NoError(t, sub.MarkInvalid(now.Add(time.Minute)))
	require.Equal(t, gitdomain.SubmissionStatusInvalid, sub.Status)
}

func TestTerminalStatusesRejectOtherTransitions(t *testing.T) {
	terminal := []struct {
		name   string
		mutate func(*gitdomain.Submission) error
		status gitdomain.SubmissionStatus
	}{
		{"validated", func(s *gitdomain.Submission) error { return s.MarkValidated(now) }, gitdomain.SubmissionStatusValidated},
		{"validation_failed", func(s *gitdomain.Submission) error { return s.MarkValidationFailed(now) }, gitdomain.SubmissionStatusValidationFailed},
		{"invalid", func(s *gitdomain.Submission) error { return s.MarkInvalid(now) }, gitdomain.SubmissionStatusInvalid},
	}

	for _, tc := range terminal {
		t.Run(tc.name, func(t *testing.T) {
			sub, err := newSubmission()
			require.NoError(t, err)
			require.NoError(t, tc.mutate(sub))

			require.ErrorIs(t, sub.MarkValidated(now.Add(time.Minute)), domain.ErrStateConflict)
			require.ErrorIs(t, sub.MarkValidationFailed(now.Add(time.Minute)), domain.ErrStateConflict)
			require.ErrorIs(t, sub.MarkInvalid(now.Add(time.Minute)), domain.ErrStateConflict)
		})
	}
}

func TestSameStatusTransitionIsConflict(t *testing.T) {
	sub, err := newSubmission()
	require.NoError(t, err)
	require.NoError(t, sub.MarkValidated(now))
	require.ErrorIs(t, sub.MarkValidated(now.Add(time.Minute)), domain.ErrStateConflict)
	require.Equal(t, gitdomain.SubmissionStatusValidated, sub.Status)
}

func TestSetValidationJobID(t *testing.T) {
	sub, err := newSubmission()
	require.NoError(t, err)

	later := now.Add(time.Minute)
	sub.SetValidationJobID("job-1", later)
	require.Equal(t, "job-1", *sub.ValidationJobID)
	require.True(t, sub.UpdatedAt.Equal(later))
}

type submissionArgs struct {
	tenantID      string
	taskID        string
	executionID   string
	repo          string
	branch        string
	commitSHA     string
	baseCommitSHA string
	summary       string
	testDecl      *string
	evidence      []byte
	changedPaths  []string
	now           time.Time
	newID         func() string
}

func defaultArgs() submissionArgs {
	testDecl := "go test ./..."
	return submissionArgs{
		tenantID:      "tenant-1",
		taskID:        "task-1",
		executionID:   "exec-1",
		repo:          "owner/repo",
		branch:        "agentguild/exec-1",
		commitSHA:     "head-abc",
		baseCommitSHA: "base-abc",
		summary:       "fix bug",
		testDecl:      &testDecl,
		evidence:      []byte(`{"tests": 42}`),
		changedPaths:  []string{"main.go"},
		now:           now,
		newID:         sequenceIDs("sub-1"),
	}
}

func buildSubmission(a submissionArgs) (*gitdomain.Submission, error) {
	return gitdomain.NewSubmission(
		a.tenantID,
		a.taskID,
		a.executionID,
		a.repo,
		a.branch,
		a.commitSHA,
		a.baseCommitSHA,
		a.summary,
		a.testDecl,
		a.evidence,
		a.changedPaths,
		a.now,
		a.newID,
	)
}

func newSubmission(mods ...func(*submissionArgs)) (*gitdomain.Submission, error) {
	a := defaultArgs()
	for _, m := range mods {
		m(&a)
	}
	return buildSubmission(a)
}

func sequenceIDs(values ...string) func() string {
	next := 0
	return func() string {
		if next >= len(values) {
			return values[len(values)-1]
		}
		value := values[next]
		next++
		return value
	}
}

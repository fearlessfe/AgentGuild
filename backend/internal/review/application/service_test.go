package application_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/stretchr/testify/require"
)

func TestCreateReviewAllocatesReviewerAndCreatesPendingReview(t *testing.T) {
	fixture := newReviewFixture(t)
	seedExecution(t, fixture, "tenant-1", "submission-1", domain.ExecutionReviewing)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	got, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-1",
		SubmissionID: "submission-1",
		Capabilities: []string{"go"},
	})

	require.NoError(t, err)
	require.Equal(t, "tenant-1", got.Data.TenantID)
	require.Equal(t, "submission-1", got.Data.SubmissionID)
	require.Equal(t, "reviewer-1", got.Data.ReviewerID)
	require.Equal(t, reviewdomain.ReviewPending, reviewdomain.ReviewStatus(got.Data.Status))
	require.Equal(t, 1, fixture.store.reviewers["tenant-1/reviewer-1"].CurrentLoad)
}

func TestCreateReviewRejectsSubmissionNotReady(t *testing.T) {
	fixture := newReviewFixture(t)
	seedExecution(t, fixture, "tenant-1", "submission-1", domain.ExecutionRunning)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	_, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-1",
		SubmissionID: "submission-1",
		Capabilities: []string{"go"},
	})

	require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestCreateReviewPicksLowestLoadReviewer(t *testing.T) {
	fixture := newReviewFixture(t)
	seedExecution(t, fixture, "tenant-1", "submission-1", domain.ExecutionReviewing)
	fixture.seedReviewerWithLoad("tenant-1", "reviewer-1", "user-1", []string{"go"}, 2)
	fixture.seedReviewerWithLoad("tenant-1", "reviewer-2", "user-2", []string{"go"}, 0)
	fixture.seedReviewerWithLoad("tenant-1", "reviewer-3", "user-3", []string{"go"}, 1)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	got, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-1",
		SubmissionID: "submission-1",
		Capabilities: []string{"go"},
	})

	require.NoError(t, err)
	require.Equal(t, "reviewer-2", got.Data.ReviewerID)
}

func TestCreateReviewFiltersByCapability(t *testing.T) {
	fixture := newReviewFixture(t)
	seedExecution(t, fixture, "tenant-1", "submission-1", domain.ExecutionReviewing)
	fixture.seedReviewer("tenant-1", "reviewer-go", "user-go", []string{"go"}, 0)
	fixture.seedReviewer("tenant-1", "reviewer-py", "user-py", []string{"python"}, 0)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	got, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-1",
		SubmissionID: "submission-1",
		Capabilities: []string{"python"},
	})

	require.NoError(t, err)
	require.Equal(t, "reviewer-py", got.Data.ReviewerID)
}

func TestCreateReviewReturnsNotFoundForMissingSubmission(t *testing.T) {
	fixture := newReviewFixture(t)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	_, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-missing",
		SubmissionID: "missing",
		Capabilities: []string{"go"},
	})

	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSubmitDecisionAcceptsWhenHardGatesPass(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")
	fixture.validation.pass = true

	got, err := fixture.svc.SubmitDecision(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.SubmitDecision{
		RequestID: "req-submit",
		ReviewID:  review.ID,
		Decision:  reviewdomain.DecisionAccepted,
		Scores:    []reviewdomain.RubricScore{{Dimension: "quality", Score: 80}},
		Summary:   "lgtm",
	})

	require.NoError(t, err)
	require.Equal(t, string(reviewdomain.DecisionAccepted), got.Data.FinalDecision)
	require.Equal(t, string(reviewdomain.ReviewSubmitted), got.Data.Status)
	require.Equal(t, 0, fixture.store.reviewers["tenant-1/reviewer-1"].CurrentLoad)
	exec := fixture.store.executions["tenant-1/submission-1"]
	require.Equal(t, domain.ExecutionAccepted, exec.Status)
}

func TestSubmitDecisionRejectsWithoutHardGateValidation(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")
	fixture.validation.pass = false

	_, err := fixture.svc.SubmitDecision(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.SubmitDecision{
		RequestID: "req-submit",
		ReviewID:  review.ID,
		Decision:  reviewdomain.DecisionAccepted,
		Scores:    []reviewdomain.RubricScore{{Dimension: "quality", Score: 80}},
	})

	require.Equal(t, "hard_gates_failed", domain.CodeOf(err))
}

func TestSubmitDecisionRequiresCompleteScoresForAccept(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")
	fixture.validation.pass = true

	_, err := fixture.svc.SubmitDecision(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.SubmitDecision{
		RequestID: "req-submit",
		ReviewID:  review.ID,
		Decision:  reviewdomain.DecisionAccepted,
		Scores:    []reviewdomain.RubricScore{},
	})

	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "rubric_scores", domain.FieldOf(err))
}

func TestSubmitDecisionRejectsUnauthorizedReviewer(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	_, err := fixture.svc.SubmitDecision(context.Background(), reviewerPrincipal("tenant-1", "reviewer-2"), reviewapp.SubmitDecision{
		RequestID: "req-submit",
		ReviewID:  review.ID,
		Decision:  reviewdomain.DecisionRejected,
	})

	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestSubmitDecisionRequestsRevision(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	got, err := fixture.svc.SubmitDecision(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.SubmitDecision{
		RequestID: "req-submit",
		ReviewID:  review.ID,
		Decision:  reviewdomain.DecisionRevisionRequested,
	})

	require.NoError(t, err)
	require.Equal(t, string(reviewdomain.DecisionRevisionRequested), got.Data.FinalDecision)
	exec := fixture.store.executions["tenant-1/submission-1"]
	require.Equal(t, domain.ExecutionRevisionRequested, exec.Status)
}

func TestAddCommentByReviewerSucceeds(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	got, err := fixture.svc.AddComment(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.AddComment{
		RequestID:       "req-comment",
		ReviewID:        review.ID,
		SubmissionID:    review.SubmissionID,
		FilePath:        "main.go",
		Side:            "right",
		LineNumber:      10,
		HunkHash:        "hunk",
		DiffFingerprint: "fp",
		Text:            "fix this",
	})

	require.NoError(t, err)
	require.Equal(t, review.ID, got.Data.ReviewID)
	require.Equal(t, "main.go", got.Data.FilePath)
	require.Equal(t, "right", got.Data.Side)
}

func TestAddCommentRejectsNonReviewer(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	_, err := fixture.svc.AddComment(context.Background(), reviewerPrincipal("tenant-1", "reviewer-2"), reviewapp.AddComment{
		RequestID:       "req-comment",
		ReviewID:        review.ID,
		SubmissionID:    review.SubmissionID,
		FilePath:        "main.go",
		Side:            "right",
		LineNumber:      10,
		HunkHash:        "hunk",
		DiffFingerprint: "fp",
		Text:            "fix this",
	})

	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestGetReviewAllowsReviewerAndPublisher(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	_, err := fixture.svc.GetReview(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.GetReview{ReviewID: review.ID})
	require.NoError(t, err)

	_, err = fixture.svc.GetReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.GetReview{ReviewID: review.ID})
	require.NoError(t, err)
}

func TestGetReviewRejectsForeignTenant(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	_, err := fixture.svc.GetReview(context.Background(), reviewerPrincipal("tenant-2", "reviewer-1"), reviewapp.GetReview{ReviewID: review.ID})
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestGetReviewAllowsExecutingAgent(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")
	// Execution.AgentID in the fixture is "agent-v1".
	_, err := fixture.svc.GetReview(context.Background(), auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeAgent, AgentID: "exec-agent", AgentVersionID: "agent-v1"}, reviewapp.GetReview{ReviewID: review.ID})
	require.NoError(t, err)
}

func TestCreateReviewRequiresExecutionReviewing(t *testing.T) {
	fixture := newReviewFixture(t)
	seedExecution(t, fixture, "tenant-1", "submission-submitted", domain.ExecutionSubmitted)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	_, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-submitted",
		SubmissionID: "submission-submitted",
		Capabilities: []string{"go"},
	})
	require.ErrorIs(t, err, domain.ErrStateConflict)

	seedExecution(t, fixture, "tenant-1", "submission-validating", domain.ExecutionValidating)
	_, err = fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-validating",
		SubmissionID: "submission-validating",
		Capabilities: []string{"go"},
	})
	require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestAddCommentRejectsMismatchedSubmissionID(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	_, err := fixture.svc.AddComment(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.AddComment{
		RequestID:       "req-comment-mismatch",
		ReviewID:        review.ID,
		SubmissionID:    "wrong-submission",
		FilePath:        "main.go",
		Side:            "right",
		LineNumber:      10,
		HunkHash:        "hunk",
		DiffFingerprint: "fp",
		Text:            "fix this",
	})

	require.Equal(t, "invalid_argument", domain.CodeOf(err))
	require.Equal(t, "submission_id", domain.FieldOf(err))
}

func TestCreateReviewIsIdempotent(t *testing.T) {
	fixture := newReviewFixture(t)
	seedExecution(t, fixture, "tenant-1", "submission-1", domain.ExecutionReviewing)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	cmd := reviewapp.CreateReview{
		RequestID:    "req-idem",
		SubmissionID: "submission-1",
		Capabilities: []string{"go"},
	}
	first, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), cmd)
	require.NoError(t, err)

	second, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), cmd)
	require.NoError(t, err)
	require.Equal(t, first.Data.ID, second.Data.ID)
}

func TestCreateReviewRejectsMismatchedIdempotencyPayload(t *testing.T) {
	fixture := newReviewFixture(t)
	seedExecution(t, fixture, "tenant-1", "submission-1", domain.ExecutionReviewing)
	seedExecution(t, fixture, "tenant-1", "submission-2", domain.ExecutionReviewing)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)
	fixture.seedRubric("tenant-1", "rubric-1", 1)

	_, err := fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-idem-mismatch",
		SubmissionID: "submission-1",
		Capabilities: []string{"go"},
	})
	require.NoError(t, err)

	_, err = fixture.svc.CreateReview(context.Background(), publisherPrincipal("tenant-1", "publisher-v1"), reviewapp.CreateReview{
		RequestID:    "req-idem-mismatch",
		SubmissionID: "submission-2",
		Capabilities: []string{"go"},
	})
	require.Equal(t, "idempotency_mismatch", domain.CodeOf(err))
}

func TestGetReviewDiffReturnsDiffForAuthorizedViewer(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	got, err := fixture.svc.GetReviewDiff(context.Background(), reviewerPrincipal("tenant-1", "reviewer-1"), reviewapp.GetReviewDiff{ReviewID: review.ID})
	require.NoError(t, err)
	require.Equal(t, []byte("diff"), got.Data)
}

func TestGetReviewDiffRejectsUnauthorizedViewer(t *testing.T) {
	fixture := newReviewFixture(t)
	review := seedPendingReview(t, fixture, "tenant-1", "submission-1", "reviewer-1")

	_, err := fixture.svc.GetReviewDiff(context.Background(), auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeHuman, OwnerID: "user-other"}, reviewapp.GetReviewDiff{ReviewID: review.ID})
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestPolicyRequiresReviewerForDecision(t *testing.T) {
	policy := reviewapp.Policy{}
	review := reviewapp.ReviewRecord{TenantID: "tenant-1", ReviewerUserID: "user-1"}

	require.NoError(t, policy.CanSubmitDecision(auth.Principal{TenantID: "tenant-1", OwnerID: "user-1"}, review))
	require.NoError(t, policy.CanSubmitDecision(auth.Principal{TenantID: "tenant-1", IsAdmin: true}, review))
	require.ErrorIs(t, policy.CanSubmitDecision(auth.Principal{TenantID: "tenant-1", OwnerID: "user-2"}, review), domain.ErrForbidden)
	require.ErrorIs(t, policy.CanSubmitDecision(auth.Principal{TenantID: "tenant-2", OwnerID: "user-1"}, review), domain.ErrForbidden)
}

func TestPolicyAllowsPublisherReviewerAndExecutingAgentToView(t *testing.T) {
	policy := reviewapp.Policy{}
	review := reviewapp.ReviewRecord{TenantID: "tenant-1", ReviewerUserID: "user-1"}
	task := application.TaskSummary{TenantID: "tenant-1", PublisherAgentVersionID: "publisher-v1", ExecutionAgentVersionID: "exec-v1"}

	require.NoError(t, policy.CanViewReview(context.Background(), auth.Principal{TenantID: "tenant-1", AgentID: "agent-1", AgentVersionID: "publisher-v1"}, review, task))
	require.NoError(t, policy.CanViewReview(context.Background(), auth.Principal{TenantID: "tenant-1", AgentID: "agent-2", AgentVersionID: "exec-v1"}, review, task))
	require.NoError(t, policy.CanViewReview(context.Background(), auth.Principal{TenantID: "tenant-1", OwnerID: "user-1"}, review, task))
	require.NoError(t, policy.CanViewReview(context.Background(), auth.Principal{TenantID: "tenant-1", IsAdmin: true}, review, task))
	require.ErrorIs(t, policy.CanViewReview(context.Background(), auth.Principal{TenantID: "tenant-1", OwnerID: "user-2"}, review, task), domain.ErrForbidden)
	require.ErrorIs(t, policy.CanViewReview(context.Background(), auth.Principal{TenantID: "tenant-1", AgentID: "agent-3", AgentVersionID: "other-v1"}, review, task), domain.ErrForbidden)
}

func TestAllocatorPicksLowestLoadAndTieBreaksByTimeAndID(t *testing.T) {
	fixture := newReviewFixture(t)
	now := fixture.store.now
	fixture.seedReviewerWithLoadAndTime("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0, now)
	fixture.seedReviewerWithLoadAndTime("tenant-1", "reviewer-2", "user-2", []string{"go"}, 0, now.Add(time.Minute))
	fixture.seedReviewerWithLoadAndTime("tenant-1", "reviewer-3", "user-3", []string{"go"}, 1, now)

	var got string
	err := fixture.store.WithTx(context.Background(), func(tx application.Tx) error {
		var err error
		got, err = fixture.alloc.Allocate(context.Background(), tx, "tenant-1", []string{"go"})
		return err
	})
	require.NoError(t, err)
	require.Equal(t, "reviewer-1", got)
}

func TestAllocatorRequiresAllCapabilities(t *testing.T) {
	fixture := newReviewFixture(t)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)
	fixture.seedReviewer("tenant-1", "reviewer-2", "user-2", []string{"go", "security"}, 0)

	var got string
	err := fixture.store.WithTx(context.Background(), func(tx application.Tx) error {
		var err error
		got, err = fixture.alloc.Allocate(context.Background(), tx, "tenant-1", []string{"go", "security"})
		return err
	})
	require.NoError(t, err)
	require.Equal(t, "reviewer-2", got)
}

func TestAllocatorReturnsErrorWhenNoReviewerMatches(t *testing.T) {
	fixture := newReviewFixture(t)
	fixture.seedReviewer("tenant-1", "reviewer-1", "user-1", []string{"go"}, 0)

	err := fixture.store.WithTx(context.Background(), func(tx application.Tx) error {
		_, err := fixture.alloc.Allocate(context.Background(), tx, "tenant-1", []string{"python"})
		return err
	})
	require.Equal(t, "no_reviewer_available", domain.CodeOf(err))
}

func TestNewServiceRequiresDependencies(t *testing.T) {
	store := newReviewMemoryStore(time.Now())
	_, err := reviewapp.NewService(store, nil, nil, reviewapp.Options{})
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
}

// --- fixtures ---

type reviewFixture struct {
	svc        *reviewapp.Service
	alloc      reviewapp.Allocator
	store      *reviewMemoryStore
	validation *fakeValidationProvider
}

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	store := newReviewMemoryStore(now)
	validation := &fakeValidationProvider{pass: true}
	diff := &fakeDiffProvider{data: []byte("diff")}
	svc, err := reviewapp.NewService(store, diff, validation, reviewapp.Options{
		NewID: sequenceIDs("review-1", "comment-1", "review-2"),
	})
	require.NoError(t, err)
	return &reviewFixture{svc: svc, alloc: reviewapp.Allocator{}, store: store, validation: validation}
}

func seedExecution(t *testing.T, f *reviewFixture, tenantID, executionID string, status domain.ExecutionStatus) {
	t.Helper()
	taskID := "task-" + executionID
	f.store.tasks[taskKey(tenantID, taskID)] = application.TaskRecord{
		ID:                      taskID,
		TenantID:                tenantID,
		PublisherAgentVersionID: "publisher-v1",
		Deadline:                f.store.now.Add(time.Hour),
		Status:                  domain.TaskInProgress,
	}
	f.store.executions[execKey(tenantID, executionID)] = &domain.Execution{
		ID:       executionID,
		TenantID: tenantID,
		TaskID:   taskID,
		AgentID:  "agent-v1",
		Status:   status,
		Lease: domain.Lease{
			Generation: 1,
			SoftExpiry: f.store.now.Add(time.Hour),
			HardExpiry: f.store.now.Add(time.Hour),
		},
	}
}

func seedPendingReview(t *testing.T, f *reviewFixture, tenantID, submissionID, reviewerID string) *reviewdomain.Review {
	t.Helper()
	seedExecution(t, f, tenantID, submissionID, domain.ExecutionReviewing)
	f.seedReviewer(tenantID, reviewerID, "user-"+reviewerID, []string{"go"}, 0)
	f.seedRubric(tenantID, "rubric-"+submissionID, 1)

	review, err := reviewdomain.NewReview("review-"+submissionID, tenantID, submissionID, reviewerID, "rubric-"+submissionID, f.store.now)
	require.NoError(t, err)
	reviewer := f.store.reviewers[reviewerKey(tenantID, reviewerID)]
	require.NoError(t, reviewer.Assign(f.store.now))
	f.store.reviewers[reviewerKey(tenantID, reviewerID)] = reviewer
	f.store.reviews[reviewKey(tenantID, review.ID)] = review
	return review
}

func (f *reviewFixture) seedReviewer(tenantID, reviewerID, userID string, caps []string, load int) {
	f.seedReviewerWithLoadAndTime(tenantID, reviewerID, userID, caps, load, f.store.now)
}

func (f *reviewFixture) seedReviewerWithLoad(tenantID, reviewerID, userID string, caps []string, load int) {
	f.seedReviewerWithLoadAndTime(tenantID, reviewerID, userID, caps, load, f.store.now)
}

func (f *reviewFixture) seedReviewerWithLoadAndTime(tenantID, reviewerID, userID string, caps []string, load int, createdAt time.Time) {
	profile, err := reviewdomain.NewReviewerProfile(reviewerID, tenantID, userID, caps, createdAt)
	if err != nil {
		panic(err)
	}
	if err := profile.SetLoad(load, createdAt); err != nil {
		panic(err)
	}
	f.store.reviewers[reviewerKey(tenantID, reviewerID)] = *profile
}

func (f *reviewFixture) seedRubric(tenantID, rubricID string, versionNumber int) {
	version, err := reviewdomain.NewRubricVersion(
		rubricID, tenantID, "Rubric "+rubricID, versionNumber,
		[]reviewdomain.RubricDimension{{ID: "quality", Name: "Quality"}},
		map[string]float64{"quality": 1.0},
		"v1", f.store.now,
	)
	if err != nil {
		panic(err)
	}
	f.store.rubrics[rubricKey(tenantID, rubricID)] = *version
}

func publisherPrincipal(tenantID, versionID string) auth.Principal {
	return auth.Principal{TenantID: tenantID, Type: auth.PrincipalTypeAgent, AgentID: "agent-1", AgentVersionID: versionID, Scopes: []string{"tasks:publish"}}
}

func reviewerPrincipal(tenantID, reviewerID string) auth.Principal {
	return auth.Principal{TenantID: tenantID, Type: auth.PrincipalTypeHuman, OwnerID: "user-" + reviewerID}
}

// --- memory store ---

type reviewMemoryStore struct {
	now        time.Time
	tasks      map[string]application.TaskRecord
	executions map[string]*domain.Execution
	reviewers  map[string]reviewdomain.ReviewerProfile
	rubrics    map[string]reviewdomain.RubricVersion
	reviews    map[string]*reviewdomain.Review
	comments   map[string]*reviewdomain.LineComment
	idem       map[application.IdempotencyKey]*application.IdempotencyRecord
}

func newReviewMemoryStore(now time.Time) *reviewMemoryStore {
	return &reviewMemoryStore{
		now:        now,
		tasks:      map[string]application.TaskRecord{},
		executions: map[string]*domain.Execution{},
		reviewers:  map[string]reviewdomain.ReviewerProfile{},
		rubrics:    map[string]reviewdomain.RubricVersion{},
		reviews:    map[string]*reviewdomain.Review{},
		comments:   map[string]*reviewdomain.LineComment{},
		idem:       map[application.IdempotencyKey]*application.IdempotencyRecord{},
	}
}

func (s *reviewMemoryStore) WithTx(ctx context.Context, fn func(application.Tx) error) error {
	return fn(&reviewMemoryTx{store: s})
}

type reviewMemoryTx struct {
	store *reviewMemoryStore
}

func (tx *reviewMemoryTx) Now(context.Context) (time.Time, error) { return tx.store.now, nil }

func (tx *reviewMemoryTx) InsertTask(context.Context, application.TaskRecord) error {
	panic("not implemented")
}
func (tx *reviewMemoryTx) GetTask(_ context.Context, tenantID, id string) (*application.TaskRecord, error) {
	r, ok := tx.store.tasks[taskKey(tenantID, id)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &r, nil
}
func (tx *reviewMemoryTx) ListTaskRecords(context.Context, application.TaskListQuery) ([]application.TaskRecord, error) {
	panic("not implemented")
}
func (tx *reviewMemoryTx) UpdateTask(context.Context, application.TaskRecord, int64, string) (bool, error) {
	panic("not implemented")
}
func (tx *reviewMemoryTx) ClaimTask(context.Context, string, string, int64, string) (bool, error) {
	panic("not implemented")
}

func (tx *reviewMemoryTx) InsertExecution(context.Context, *domain.Execution, []byte) error {
	panic("not implemented")
}
func (tx *reviewMemoryTx) GetExecution(_ context.Context, tenantID, id string) (*domain.Execution, int64, error) {
	e, ok := tx.store.executions[execKey(tenantID, id)]
	if !ok {
		return nil, 0, domain.ErrNotFound
	}
	copy := *e
	return &copy, 0, nil
}
func (tx *reviewMemoryTx) GetExecutionForUpdate(_ context.Context, tenantID, id string) (*domain.Execution, int64, error) {
	return tx.GetExecution(context.Background(), tenantID, id)
}
func (tx *reviewMemoryTx) ListActiveExecutions(context.Context, string, string) ([]application.ExecutionRecord, error) {
	panic("not implemented")
}
func (tx *reviewMemoryTx) UpdateExecution(_ context.Context, e *domain.Execution, _ int64) (bool, error) {
	key := execKey(e.TenantID, e.ID)
	if _, ok := tx.store.executions[key]; !ok {
		return false, nil
	}
	copy := *e
	tx.store.executions[key] = &copy
	return true, nil
}
func (tx *reviewMemoryTx) UpdateOwnedExecution(context.Context, *domain.Execution, int64, string, int64) (bool, error) {
	panic("not implemented")
}
func (tx *reviewMemoryTx) GetExecutionUsage(context.Context, string, string) (*application.UsageView, error) {
	panic("not implemented")
}

func (tx *reviewMemoryTx) AcquireIdempotency(_ context.Context, key application.IdempotencyKey, hash [32]byte, expires time.Time) (*application.IdempotencyRecord, error) {
	if r := tx.store.idem[key]; r != nil {
		if r.RequestHash != hash {
			return nil, &domain.Error{Code: "idempotency_mismatch", Message: "mismatch"}
		}
		copy := *r
		return &copy, nil
	}
	r := &application.IdempotencyRecord{Key: key, RequestHash: hash, ExpiresAt: expires, OwnerToken: "owner", Acquired: true}
	tx.store.idem[key] = r
	copy := *r
	return &copy, nil
}
func (tx *reviewMemoryTx) CompleteIdempotency(_ context.Context, key application.IdempotencyKey, _ string, code int, body []byte) error {
	r, ok := tx.store.idem[key]
	if !ok {
		return domain.ErrNotFound
	}
	r.ResponseCode = &code
	r.ResponseBody = body
	r.Completed = true
	return nil
}

func (tx *reviewMemoryTx) AppendTaskEvent(context.Context, application.TaskEvent) error {
	panic("not implemented")
}
func (tx *reviewMemoryTx) AppendOutboxEvent(context.Context, application.OutboxEvent) error {
	panic("not implemented")
}
func (tx *reviewMemoryTx) ListTaskEvents(context.Context, string, string, int64, int) ([]application.TaskEventSummary, error) {
	panic("not implemented")
}
func (tx *reviewMemoryTx) GetLatestExecutionEvent(context.Context, string, string) (application.TaskEventSummary, error) {
	panic("not implemented")
}

func (tx *reviewMemoryTx) Reviews() application.ReviewRepository {
	return reviewMemoryReviewRepository{store: tx.store}
}
func (tx *reviewMemoryTx) LineComments() application.LineCommentRepository {
	return reviewMemoryCommentRepository{store: tx.store}
}
func (tx *reviewMemoryTx) Rubrics() application.RubricRepository {
	return reviewMemoryRubricRepository{store: tx.store}
}
func (tx *reviewMemoryTx) Reviewers() application.ReviewerRepository {
	return reviewMemoryReviewerRepository{store: tx.store}
}

func (tx *reviewMemoryTx) UpsertReputationProjection(context.Context, reputationapp.ProjectionRecord) error {
	return nil
}

func (tx *reviewMemoryTx) ListReputationProjectionsByAgentVersion(context.Context, string, string) ([]reputationapp.ProjectionRecord, error) {
	return nil, nil
}

func (tx *reviewMemoryTx) RequireLiveAgent(context.Context, auth.Principal) error { return nil }

// --- memory repositories ---

type reviewMemoryReviewRepository struct{ store *reviewMemoryStore }

func (r reviewMemoryReviewRepository) Insert(_ context.Context, review *reviewdomain.Review) error {
	r.store.reviews[reviewKey(review.TenantID, review.ID)] = review
	return nil
}

func (r reviewMemoryReviewRepository) Update(_ context.Context, review *reviewdomain.Review) error {
	key := reviewKey(review.TenantID, review.ID)
	if _, ok := r.store.reviews[key]; !ok {
		return domain.ErrNotFound
	}
	r.store.reviews[key] = review
	return nil
}

func (r reviewMemoryReviewRepository) GetByID(_ context.Context, tenantID, id string) (*reviewdomain.Review, error) {
	review, ok := r.store.reviews[reviewKey(tenantID, id)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *review
	copy.RubricScores = slices.Clone(review.RubricScores)
	return &copy, nil
}

func (r reviewMemoryReviewRepository) ListBySubmission(context.Context, string, string) ([]reviewdomain.Review, error) {
	panic("not implemented")
}

func (r reviewMemoryReviewRepository) ListUnprojected(context.Context, int) ([]application.ReviewSignalRecord, error) {
	panic("not implemented")
}

func (r reviewMemoryReviewRepository) MarkProjected(context.Context, string, string) error {
	panic("not implemented")
}

type reviewMemoryCommentRepository struct{ store *reviewMemoryStore }

func (r reviewMemoryCommentRepository) Insert(_ context.Context, comment *reviewdomain.LineComment) error {
	r.store.comments[commentKey(comment.TenantID, comment.ID)] = comment
	return nil
}

func (r reviewMemoryCommentRepository) ListByReview(_ context.Context, tenantID, reviewID string) ([]reviewdomain.LineComment, error) {
	var out []reviewdomain.LineComment
	for _, c := range r.store.comments {
		if c.TenantID == tenantID && c.ReviewID == reviewID {
			out = append(out, *c)
		}
	}
	slices.SortFunc(out, func(a, b reviewdomain.LineComment) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

type reviewMemoryRubricRepository struct{ store *reviewMemoryStore }

func (r reviewMemoryRubricRepository) GetActive(_ context.Context, tenantID string) (*reviewdomain.RubricVersion, error) {
	for _, v := range r.store.rubrics {
		if v.TenantID == tenantID && v.IsActive {
			copy := cloneRubricVersion(v)
			return &copy, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r reviewMemoryRubricRepository) GetByID(_ context.Context, tenantID, id string) (*reviewdomain.RubricVersion, error) {
	v, ok := r.store.rubrics[rubricKey(tenantID, id)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := cloneRubricVersion(v)
	return &copy, nil
}

func (r reviewMemoryRubricRepository) ListVersions(context.Context, string) ([]reviewdomain.RubricVersion, error) {
	panic("not implemented")
}
func (r reviewMemoryRubricRepository) CreateVersion(context.Context, *reviewdomain.RubricVersion) error {
	panic("not implemented")
}

type reviewMemoryReviewerRepository struct{ store *reviewMemoryStore }

func (r reviewMemoryReviewerRepository) Insert(_ context.Context, profile *reviewdomain.ReviewerProfile) error {
	r.store.reviewers[reviewerKey(profile.TenantID, profile.ID)] = *profile
	return nil
}

func (r reviewMemoryReviewerRepository) GetByID(_ context.Context, tenantID, id string) (*reviewdomain.ReviewerProfile, error) {
	profile, ok := r.store.reviewers[reviewerKey(tenantID, id)]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := cloneReviewerProfile(profile)
	return &copy, nil
}

func (r reviewMemoryReviewerRepository) ListActive(_ context.Context, tenantID string, _ int) ([]reviewdomain.ReviewerProfile, error) {
	var out []reviewdomain.ReviewerProfile
	for _, profile := range r.store.reviewers {
		if profile.TenantID == tenantID && profile.IsActive {
			out = append(out, cloneReviewerProfile(profile))
		}
	}
	return out, nil
}

func (r reviewMemoryReviewerRepository) IncrementLoad(_ context.Context, tenantID, reviewerID string) error {
	key := reviewerKey(tenantID, reviewerID)
	profile, ok := r.store.reviewers[key]
	if !ok {
		return domain.ErrNotFound
	}
	if !profile.IsActive {
		return domain.ErrStateConflict
	}
	profile.CurrentLoad++
	profile.UpdatedAt = r.store.now
	r.store.reviewers[key] = profile
	return nil
}

func (r reviewMemoryReviewerRepository) DecrementLoad(_ context.Context, tenantID, reviewerID string) error {
	key := reviewerKey(tenantID, reviewerID)
	profile, ok := r.store.reviewers[key]
	if !ok {
		return domain.ErrNotFound
	}
	if profile.CurrentLoad <= 0 {
		return domain.ErrStateConflict
	}
	profile.CurrentLoad--
	profile.UpdatedAt = r.store.now
	r.store.reviewers[key] = profile
	return nil
}

// --- helpers ---

func taskKey(tenantID, id string) string     { return tenantID + "/" + id }
func execKey(tenantID, id string) string     { return tenantID + "/" + id }
func reviewerKey(tenantID, id string) string { return tenantID + "/" + id }
func rubricKey(tenantID, id string) string   { return tenantID + "/" + id }
func reviewKey(tenantID, id string) string   { return tenantID + "/" + id }
func commentKey(tenantID, id string) string  { return tenantID + "/" + id }

func cloneReviewerProfile(p reviewdomain.ReviewerProfile) reviewdomain.ReviewerProfile {
	p.Capabilities = slices.Clone(p.Capabilities)
	return p
}

func cloneRubricVersion(v reviewdomain.RubricVersion) reviewdomain.RubricVersion {
	v.Dimensions = slices.Clone(v.Dimensions)
	v.Weights = make(map[string]float64, len(v.Weights))
	for k, val := range v.Weights {
		v.Weights[k] = val
	}
	return v
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

// --- providers ---

type fakeDiffProvider struct {
	data []byte
	err  error
}

func (f *fakeDiffProvider) GetDiff(context.Context, string) ([]byte, error) {
	return f.data, f.err
}

type fakeValidationProvider struct {
	pass bool
}

func (f *fakeValidationProvider) GetValidationStatus(context.Context, string) (reviewapp.ValidationStatus, error) {
	return fakeValidationStatus{pass: f.pass}, nil
}

type fakeValidationStatus struct {
	pass bool
}

func (f fakeValidationStatus) AllHardGatesPassed() bool { return f.pass }

package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

func mustNewReview(t *testing.T, id, submissionID, reviewerID, rubricVersionID string) *reviewdomain.Review {
	t.Helper()
	review, err := reviewdomain.NewReview(id, "tenant-1", submissionID, reviewerID, rubricVersionID, time.Now())
	require.NoError(t, err)
	return review
}

func decisionAccepted() reviewdomain.Decision {
	return reviewdomain.DecisionAccepted
}

func decisionRejected() reviewdomain.Decision {
	return reviewdomain.DecisionRejected
}

func assertInvalidArgument(t *testing.T, err error, field string) {
	t.Helper()
	require.ErrorIs(t, err, appdomain.Error{Code: "invalid_argument"})
	var domainErr *appdomain.Error
	require.True(t, errors.As(err, &domainErr))
	require.Equal(t, field, domainErr.Field)
}

func TestReviewSubmitRequiresPending(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	if err := review.Submit(decisionAccepted(), nil, time.Now()); err == nil {
		t.Fatal("expected error when scores are empty")
	}
}

func TestReviewConstructorRejectsMissingFields(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name            string
		id              string
		tenantID        string
		submissionID    string
		reviewerID      string
		rubricVersionID string
		field           string
	}{
		{name: "empty id", tenantID: "t", submissionID: "s", reviewerID: "r", rubricVersionID: "rv", field: "id"},
		{name: "empty tenant_id", id: "id", submissionID: "s", reviewerID: "r", rubricVersionID: "rv", field: "tenant_id"},
		{name: "empty submission_id", id: "id", tenantID: "t", reviewerID: "r", rubricVersionID: "rv", field: "submission_id"},
		{name: "empty reviewer_id", id: "id", tenantID: "t", submissionID: "s", rubricVersionID: "rv", field: "reviewer_id"},
		{name: "empty rubric_version_id", id: "id", tenantID: "t", submissionID: "s", reviewerID: "r", field: "rubric_version_id"},
		{name: "zero created_at", id: "id", tenantID: "t", submissionID: "s", reviewerID: "r", rubricVersionID: "rv", field: "created_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var nowArg time.Time
			if tc.field != "created_at" {
				nowArg = now
			}
			review, err := reviewdomain.NewReview(tc.id, tc.tenantID, tc.submissionID, tc.reviewerID, tc.rubricVersionID, nowArg)
			require.Nil(t, review)
			assertInvalidArgument(t, err, tc.field)
		})
	}
}

func TestReviewStateMachine(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		decision   reviewdomain.Decision
		scores     []reviewdomain.RubricScore
		wantStatus reviewdomain.ReviewStatus
		wantErr    error
	}{
		{
			name:       "accepted with scores",
			decision:   reviewdomain.DecisionAccepted,
			scores:     []reviewdomain.RubricScore{{Dimension: "correctness", Score: 80}},
			wantStatus: reviewdomain.ReviewSubmitted,
		},
		{
			name:       "accepted without scores fails",
			decision:   reviewdomain.DecisionAccepted,
			scores:     nil,
			wantErr:    appdomain.Error{Code: "invalid_argument"},
		},
		{
			name:       "rejected without scores",
			decision:   reviewdomain.DecisionRejected,
			scores:     nil,
			wantStatus: reviewdomain.ReviewSubmitted,
		},
		{
			name:       "revision requested without scores",
			decision:   reviewdomain.DecisionRevisionRequested,
			scores:     nil,
			wantStatus: reviewdomain.ReviewSubmitted,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
			err := review.Submit(tc.decision, tc.scores, now)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, review.Status)
			require.Equal(t, tc.decision, review.FinalDecision)
			require.True(t, review.SubmittedAt.Equal(now))
		})
	}
}

func TestReviewCannotSubmitTwice(t *testing.T) {
	now := time.Now()
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	require.NoError(t, review.Submit(decisionAccepted(), []reviewdomain.RubricScore{{Dimension: "correctness", Score: 80}}, now))
	require.ErrorIs(t, review.Submit(decisionRejected(), nil, now.Add(time.Minute)), appdomain.ErrStateConflict)
}

func TestReviewAcceptedRequiresValidRubricScores(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	now := time.Now()

	err := review.Submit(decisionAccepted(), []reviewdomain.RubricScore{{Dimension: "", Score: 80}}, now)
	assertInvalidArgument(t, err, "rubric_scores")

	err = review.Submit(decisionAccepted(), []reviewdomain.RubricScore{{Dimension: "correctness", Score: -1}}, now)
	assertInvalidArgument(t, err, "rubric_scores")

	err = review.Submit(decisionAccepted(), []reviewdomain.RubricScore{{Dimension: "correctness", Score: 101}}, now)
	assertInvalidArgument(t, err, "rubric_scores")
}

func TestReviewSubmitRejectsInvalidDecision(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	now := time.Now()

	err := review.Submit("", nil, now)
	assertInvalidArgument(t, err, "final_decision")

	err = review.Submit("unknown", nil, now)
	assertInvalidArgument(t, err, "final_decision")
}

func TestReviewSubmitRejectsZeroNow(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	err := review.Submit(decisionRejected(), nil, time.Time{})
	assertInvalidArgument(t, err, "submitted_at")
}

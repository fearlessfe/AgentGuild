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
	review, err := reviewdomain.NewReview(id, "tenant-1", submissionID, reviewerID, rubricVersionID, "code-review", time.Now())
	require.NoError(t, err)
	return review
}

func mustNewRubricVersionForReview(t *testing.T, id string, dimensions []reviewdomain.RubricDimension) *reviewdomain.RubricVersion {
	t.Helper()
	weights := make(map[string]float64, len(dimensions))
	for _, d := range dimensions {
		weights[d.ID] = 1.0
	}
	rv, err := reviewdomain.NewRubricVersion(id, "tenant-1", "Rubric "+id, 1, dimensions, weights, "v1", time.Now())
	require.NoError(t, err)
	return rv
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
	rubric := mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{{ID: "quality", Name: "Quality"}})
	if err := review.Submit(decisionAccepted(), nil, rubric, time.Now()); err == nil {
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
		capability      string
		field           string
	}{
		{name: "empty id", tenantID: "t", submissionID: "s", reviewerID: "r", rubricVersionID: "rv", capability: "code-review", field: "id"},
		{name: "empty tenant_id", id: "id", submissionID: "s", reviewerID: "r", rubricVersionID: "rv", capability: "code-review", field: "tenant_id"},
		{name: "empty submission_id", id: "id", tenantID: "t", reviewerID: "r", rubricVersionID: "rv", capability: "code-review", field: "submission_id"},
		{name: "empty reviewer_id", id: "id", tenantID: "t", submissionID: "s", rubricVersionID: "rv", capability: "code-review", field: "reviewer_id"},
		{name: "empty rubric_version_id", id: "id", tenantID: "t", submissionID: "s", reviewerID: "r", capability: "code-review", field: "rubric_version_id"},
		{name: "empty capability", id: "id", tenantID: "t", submissionID: "s", reviewerID: "r", rubricVersionID: "rv", field: "capability"},
		{name: "zero created_at", id: "id", tenantID: "t", submissionID: "s", reviewerID: "r", rubricVersionID: "rv", capability: "code-review", field: "created_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var nowArg time.Time
			if tc.field != "created_at" {
				nowArg = now
			}
			review, err := reviewdomain.NewReview(tc.id, tc.tenantID, tc.submissionID, tc.reviewerID, tc.rubricVersionID, tc.capability, nowArg)
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
		rubric     *reviewdomain.RubricVersion
		wantStatus reviewdomain.ReviewStatus
		wantErr    error
	}{
		{
			name:       "accepted with complete scores",
			decision:   reviewdomain.DecisionAccepted,
			scores:     []reviewdomain.RubricScore{{Dimension: "correctness", Score: 80}},
			rubric:     mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}}),
			wantStatus: reviewdomain.ReviewSubmitted,
		},
		{
			name:     "accepted without scores fails",
			decision: reviewdomain.DecisionAccepted,
			scores:   nil,
			rubric:   mustNewRubricVersionForReview(t, "rubric-2", []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}}),
			wantErr:  appdomain.Error{Code: "invalid_argument"},
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
			err := review.Submit(tc.decision, tc.scores, tc.rubric, now)
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
	rubric := mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}})
	require.NoError(t, review.Submit(decisionAccepted(), []reviewdomain.RubricScore{{Dimension: "correctness", Score: 80}}, rubric, now))
	require.ErrorIs(t, review.Submit(decisionRejected(), nil, nil, now.Add(time.Minute)), appdomain.ErrStateConflict)
}

func TestReviewAcceptedRequiresCompleteRubricScores(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	now := time.Now()

	cases := []struct {
		name   string
		rubric *reviewdomain.RubricVersion
		scores []reviewdomain.RubricScore
	}{
		{
			name:   "empty dimension",
			rubric: mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}}),
			scores: []reviewdomain.RubricScore{{Dimension: "", Score: 80}},
		},
		{
			name:   "score below range",
			rubric: mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}}),
			scores: []reviewdomain.RubricScore{{Dimension: "correctness", Score: -1}},
		},
		{
			name:   "score above range",
			rubric: mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}}),
			scores: []reviewdomain.RubricScore{{Dimension: "correctness", Score: 101}},
		},
		{
			name: "missing dimension",
			rubric: mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{
				{ID: "correctness", Name: "Correctness"},
				{ID: "readability", Name: "Readability"},
			}),
			scores: []reviewdomain.RubricScore{{Dimension: "correctness", Score: 80}},
		},
		{
			name:   "duplicate dimension",
			rubric: mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{{ID: "correctness", Name: "Correctness"}}),
			scores: []reviewdomain.RubricScore{
				{Dimension: "correctness", Score: 80},
				{Dimension: "correctness", Score: 90},
			},
		},
		{
			name:   "nil rubric",
			rubric: nil,
			scores: []reviewdomain.RubricScore{{Dimension: "correctness", Score: 80}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := review.Submit(decisionAccepted(), tc.scores, tc.rubric, now)
			assertInvalidArgument(t, err, "rubric_scores")
		})
	}
}

func TestReviewAcceptedRequiresAllDimensions(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	rubric := mustNewRubricVersionForReview(t, "rubric-1", []reviewdomain.RubricDimension{
		{ID: "correctness", Name: "Correctness"},
		{ID: "readability", Name: "Readability"},
	})
	now := time.Now()

	err := review.Submit(decisionAccepted(), []reviewdomain.RubricScore{
		{Dimension: "correctness", Score: 80},
		{Dimension: "readability", Score: 70},
	}, rubric, now)
	require.NoError(t, err)
	require.Equal(t, reviewdomain.ReviewSubmitted, review.Status)
}

func TestReviewSubmitRejectsInvalidDecision(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	now := time.Now()

	err := review.Submit("", nil, nil, now)
	assertInvalidArgument(t, err, "final_decision")

	err = review.Submit("unknown", nil, nil, now)
	assertInvalidArgument(t, err, "final_decision")
}

func TestReviewSubmitRejectsZeroNow(t *testing.T) {
	review := mustNewReview(t, "rev-1", "sub-1", "revi-1", "rubric-1")
	err := review.Submit(decisionRejected(), nil, nil, time.Time{})
	assertInvalidArgument(t, err, "submitted_at")
}

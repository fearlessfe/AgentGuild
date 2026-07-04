package domain

import (
	"time"

	appdomain "agentguild.dev/agentguild/backend/internal/domain"
)

type ReviewStatus string

const (
	ReviewPending   ReviewStatus = "pending"
	ReviewSubmitted ReviewStatus = "submitted"
)

type Decision string

const (
	DecisionAccepted          Decision = "accepted"
	DecisionRejected          Decision = "rejected"
	DecisionRevisionRequested Decision = "revision_requested"
)

// RubricScore records the score given for a single rubric dimension.
type RubricScore struct {
	Dimension string
	Score     int
}

// Review represents a review of a submission by a reviewer against a rubric version.
type Review struct {
	ID              string
	TenantID        string
	SubmissionID    string
	ReviewerID      string
	RubricVersionID string
	RubricScores    []RubricScore
	Summary         string
	Status          ReviewStatus
	FinalDecision   Decision
	SubmittedAt     time.Time
	CreatedAt       time.Time
}

// NewReview creates a new review in the pending state.
func NewReview(id, tenantID, submissionID, reviewerID, rubricVersionID string, now time.Time) (*Review, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if submissionID == "" {
		return nil, invalidArgument("submission_id")
	}
	if reviewerID == "" {
		return nil, invalidArgument("reviewer_id")
	}
	if rubricVersionID == "" {
		return nil, invalidArgument("rubric_version_id")
	}
	if now.IsZero() {
		return nil, invalidArgument("created_at")
	}
	return &Review{
		ID:              id,
		TenantID:        tenantID,
		SubmissionID:    submissionID,
		ReviewerID:      reviewerID,
		RubricVersionID: rubricVersionID,
		Status:          ReviewPending,
		CreatedAt:       now,
	}, nil
}

// Submit transitions the review from pending to submitted.
// Accepted decisions require non-empty, valid rubric scores.
func (r *Review) Submit(decision Decision, scores []RubricScore, now time.Time) error {
	if r.Status != ReviewPending {
		return appdomain.ErrStateConflict
	}
	if !decisionValid(decision) {
		return invalidArgument("final_decision")
	}
	if now.IsZero() {
		return invalidArgument("submitted_at")
	}
	if decision == DecisionAccepted && !rubricComplete(scores, r.RubricVersionID) {
		return invalidArgument("rubric_scores")
	}
	r.FinalDecision = decision
	r.RubricScores = scores
	r.Status = ReviewSubmitted
	r.SubmittedAt = now
	return nil
}

func decisionValid(d Decision) bool {
	switch d {
	case DecisionAccepted, DecisionRejected, DecisionRevisionRequested:
		return true
	}
	return false
}

func rubricComplete(scores []RubricScore, rubricVersionID string) bool {
	if rubricVersionID == "" {
		return false
	}
	if len(scores) == 0 {
		return false
	}
	for _, s := range scores {
		if s.Dimension == "" || s.Score < 0 || s.Score > 100 {
			return false
		}
	}
	return true
}

func invalidArgument(field string) error {
	return &appdomain.Error{
		Code:    "invalid_argument",
		Message: field + " is invalid",
		Field:   field,
	}
}

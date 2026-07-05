package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

// Policy enforces authorization rules for the review application service.
type Policy struct{}

// ReviewRecord is the subset of review data needed for authorization.
type ReviewRecord struct {
	TenantID       string
	ID             string
	SubmissionID   string
	ReviewerID     string
	ReviewerUserID string
	Status         string
}

// CanViewReview decides whether the principal may read a review.
// Allowed roles: admin, the publisher agent of the task, the agent that owns
// the execution, or the assigned reviewer.
//
// Note: human owner resolution for the executing agent would require identity
// context; the current model checks the executing agent itself using the
// agent version ID recorded on the execution.
func (Policy) CanViewReview(_ context.Context, principal auth.Principal, review ReviewRecord, task application.TaskSummary) error {
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	if review.TenantID != principal.TenantID || task.TenantID != principal.TenantID {
		return domain.ErrForbidden
	}
	if principal.IsAdmin {
		return nil
	}
	if principal.AgentID != "" && principal.AgentVersionID != "" && principal.AgentVersionID == task.PublisherAgentVersionID {
		return nil
	}
	if principal.AgentID != "" && principal.AgentVersionID != "" && principal.AgentVersionID == task.ExecutionAgentVersionID {
		return nil
	}
	if principal.OwnerID != "" && principal.OwnerID == review.ReviewerUserID {
		return nil
	}
	return domain.ErrForbidden
}

// CanSubmitDecision decides whether the principal may submit a review decision.
// Only the assigned reviewer (or an admin) may submit.
func (Policy) CanSubmitDecision(principal auth.Principal, review ReviewRecord) error {
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	if review.TenantID != principal.TenantID {
		return domain.ErrForbidden
	}
	if principal.IsAdmin {
		return nil
	}
	if principal.OwnerID != "" && principal.OwnerID == review.ReviewerUserID {
		return nil
	}
	return domain.ErrForbidden
}

// CanAddComment decides whether the principal may add a line comment.
// Only the assigned reviewer (or an admin) may comment.
func (Policy) CanAddComment(principal auth.Principal, review ReviewRecord) error {
	return Policy{}.CanSubmitDecision(principal, review)
}

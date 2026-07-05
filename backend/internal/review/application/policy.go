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
func (p Policy) CanViewReview(ctx context.Context, principal auth.Principal, review ReviewRecord, task application.TaskSummary) error {
	if err := requireScope(principal, "reviews:read"); err != nil {
		return err
	}
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
func (p Policy) CanSubmitDecision(principal auth.Principal, review ReviewRecord) error {
	if err := requireScope(principal, "reviews:write"); err != nil {
		return err
	}
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
func (p Policy) CanAddComment(principal auth.Principal, review ReviewRecord) error {
	return p.CanSubmitDecision(principal, review)
}

// CanViewSubmission decides whether the principal may view a submission's diff.
// Any caller with reviews:read scope in the same tenant is allowed.
func (p Policy) CanViewSubmission(principal auth.Principal) error {
	if err := requireScope(principal, "reviews:read"); err != nil {
		return err
	}
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	return nil
}

// CanSubmitForReview decides whether the principal may move an execution to
// the reviewing state. Allowed: admin, publisher agent, or executing agent.
func (p Policy) CanSubmitForReview(principal auth.Principal, task application.TaskSummary, execution *domain.Execution) error {
	if err := requireScope(principal, "tasks:execute"); err != nil {
		return err
	}
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	if task.TenantID != principal.TenantID {
		return domain.ErrForbidden
	}
	if principal.IsAdmin {
		return nil
	}
	if principal.AgentID != "" && principal.AgentVersionID != "" && principal.AgentVersionID == task.PublisherAgentVersionID {
		return nil
	}
	if principal.AgentID != "" && principal.AgentVersionID != "" && principal.AgentVersionID == execution.AgentID {
		return nil
	}
	return domain.ErrForbidden
}

func requireScope(principal auth.Principal, scope string) error {
	if principal.IsAdmin {
		return nil
	}
	if principal.Type == auth.PrincipalTypeHuman {
		if principal.TenantID == "" {
			return domain.ErrForbidden
		}
		for _, s := range principal.Scopes {
			if s == scope {
				return nil
			}
		}
		return domain.ErrForbidden
	}
	return (auth.ScopePolicy{}).Require(principal, scope)
}

package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
)

// FixedSubmissionStore is a stub SubmissionStore that returns a single
// accepted submission. It is intended for tests; production uses the
// PostgreSQL-backed store in this module's postgres package.
type FixedSubmissionStore struct {
	Submission *Submission
}

// NewFixedSubmissionStore creates a stub store that always returns the provided
// submission. If submission is nil, a default accepted submission is used.
func NewFixedSubmissionStore(submission *Submission) *FixedSubmissionStore {
	if submission == nil {
		submission = &Submission{
			ID:     "sub-default",
			TaskID: "task-default",
			Status: "accepted",
		}
	}
	return &FixedSubmissionStore{Submission: submission}
}

// GetAcceptedSubmission returns the fixed submission if IDs match and status is
// accepted; otherwise it returns a not-found error.
func (s *FixedSubmissionStore) GetAcceptedSubmission(_ context.Context, tenantID, submissionID string) (*Submission, error) {
	if s.Submission == nil || s.Submission.ID != submissionID || s.Submission.Status != "accepted" {
		return nil, domain.ErrNotFound
	}
	result := *s.Submission
	result.TenantID = tenantID
	return &result, nil
}

// FixedExecutionStore is a stub ExecutionStore that returns a single execution.
type FixedExecutionStore struct {
	Execution *Execution
}

// NewFixedExecutionStore creates a stub store that always returns the provided
// execution. If execution is nil, a default execution is used.
func NewFixedExecutionStore(execution *Execution) *FixedExecutionStore {
	if execution == nil {
		execution = &Execution{
			ID:             "exe-default",
			TaskID:         "task-default",
			AgentVersionID: "version-default",
			Capabilities:   []string{"code"},
		}
	}
	return &FixedExecutionStore{Execution: execution}
}

// GetExecution returns the fixed execution if IDs match; otherwise not-found.
func (s *FixedExecutionStore) GetExecution(_ context.Context, tenantID, executionID string) (*Execution, error) {
	if s.Execution == nil || s.Execution.ID != executionID {
		return nil, domain.ErrNotFound
	}
	result := *s.Execution
	result.TenantID = tenantID
	return &result, nil
}

var _ SubmissionStore = (*FixedSubmissionStore)(nil)
var _ ExecutionStore = (*FixedExecutionStore)(nil)

package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/agentexperience/application"
	"agentguild.dev/agentguild/backend/internal/agentexperience/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type submissionStore struct {
	q queryer
}

// NewSubmissionStore returns a SubmissionStore backed by pool. An accepted
// submission is a submission whose review has been submitted with a final
// decision of accepted; the owning agent is resolved through the execution's
// agent version.
func NewSubmissionStore(pool *pgxpool.Pool) application.SubmissionStore {
	return &submissionStore{q: pool}
}

func (s *submissionStore) GetAcceptedSubmission(ctx context.Context, tenantID, submissionID string) (*application.Submission, error) {
	var submission application.Submission
	err := s.q.QueryRow(ctx, `
		SELECT s.id, s.tenant_id, av.agent_id, s.task_id, r.id, s.execution_id
		FROM submissions s
		JOIN reviews r
		  ON r.tenant_id = s.tenant_id AND r.submission_id = s.id
		 AND r.status = 'submitted' AND r.final_decision = 'accepted'
		JOIN executions e
		  ON e.tenant_id = s.tenant_id AND e.id = s.execution_id
		JOIN agent_versions av
		  ON av.tenant_id = e.tenant_id AND av.id = e.agent_version_id
		WHERE s.tenant_id = $1 AND s.id = $2`,
		tenantID, submissionID,
	).Scan(
		&submission.ID, &submission.TenantID, &submission.AgentID,
		&submission.TaskID, &submission.ReviewID, &submission.ExecutionID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	submission.Status = "accepted"
	return &submission, nil
}

var _ application.SubmissionStore = (*submissionStore)(nil)

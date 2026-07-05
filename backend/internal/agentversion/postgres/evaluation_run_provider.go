package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/agentversion/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// evaluationRunProvider adapts the evaluation_runs table to the
// agentversion.application.EvaluationRunProvider interface so the version module
// can verify promotion eligibility without importing the evaluation module.
type evaluationRunProvider struct {
	q queryer
}

// NewEvaluationRunProvider returns an application.EvaluationRunProvider backed
// by the same pool used for agent version storage.
func NewEvaluationRunProvider(pool *pgxpool.Pool) application.EvaluationRunProvider {
	return &evaluationRunProvider{q: pool}
}

// GetLatestPassed returns the most recent passed evaluation run for the given
// agent version. It reads through the supplied transaction so the check is
// consistent with the promotion transaction.
func (p *evaluationRunProvider) GetLatestPassed(ctx context.Context, tx application.Tx, tenantID, versionID string) (*application.EvaluationRunInfo, error) {
	var id string
	err := tx.QueryRow(ctx, `
		SELECT id
		FROM evaluation_runs
		WHERE tenant_id=$1 AND agent_version_id=$2 AND status='passed'
		ORDER BY completed_at DESC NULLS LAST, started_at DESC
		LIMIT 1`,
		tenantID, versionID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &application.EvaluationRunInfo{
		ID:     id,
		Status: "passed",
	}, nil
}

var _ application.EvaluationRunProvider = (*evaluationRunProvider)(nil)

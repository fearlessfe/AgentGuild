package postgres

import (
	"context"

	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationpostgres "agentguild.dev/agentguild/backend/internal/reputation/postgres"
)

func (tx *Tx) UpsertReputationProjection(ctx context.Context, p reputationapp.ProjectionRecord) error {
	return reputationpostgres.UpsertProjection(ctx, tx.tx, tx.Now, p)
}

func (tx *Tx) ListReputationProjectionsByAgentVersion(ctx context.Context, tenantID, agentVersionID string) ([]reputationapp.ProjectionRecord, error) {
	return reputationpostgres.ListProjectionsByAgentVersion(ctx, tx.tx, tenantID, agentVersionID)
}

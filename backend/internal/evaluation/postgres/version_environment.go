package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/evaluation/application"
	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// versionEnvironmentProvider reads agent version environment digests for the
// auto evaluator. It lives in the evaluation module's own postgres layer
// (same precedent as evaluationTaskObserver): tenant-scoped and read-only.
type versionEnvironmentProvider struct {
	q queryer
}

// NewVersionEnvironmentProvider returns a VersionEnvironmentProvider backed by pool.
func NewVersionEnvironmentProvider(pool *pgxpool.Pool) application.VersionEnvironmentProvider {
	return &versionEnvironmentProvider{q: pool}
}

func (p *versionEnvironmentProvider) GetVersionEnvironmentDigest(ctx context.Context, tenantID, versionID string) (string, error) {
	var digest string
	err := p.q.QueryRow(ctx, `
		SELECT COALESCE(environment_digest, '')
		FROM agent_versions
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, versionID,
	).Scan(&digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return digest, nil
}

var _ application.VersionEnvironmentProvider = (*versionEnvironmentProvider)(nil)

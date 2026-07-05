package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
	evalapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// versionLifecycleAdapter adapts the agentversion postgres repository to the
// evaluation.application.VersionLifecyclePort interface so the evaluation module
// can transition agent versions without importing the agentversion application
// package.
type versionLifecycleAdapter struct {
	q queryer
}

// NewVersionLifecycleAdapter returns an evalapp.VersionLifecyclePort backed by
// the same pool used for agent version storage.
func NewVersionLifecycleAdapter(pool *pgxpool.Pool) evalapp.VersionLifecyclePort {
	return &versionLifecycleAdapter{q: pool}
}

func (a *versionLifecycleAdapter) GetByID(ctx context.Context, tenantID, versionID string) (*evalapp.VersionInfo, error) {
	var agentID, status string
	err := a.q.QueryRow(ctx, `
		SELECT agent_id, status
		FROM agent_versions
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, versionID,
	).Scan(&agentID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &evalapp.VersionInfo{
		ID:       versionID,
		TenantID: tenantID,
		AgentID:  agentID,
		Status:   status,
	}, nil
}

func (a *versionLifecycleAdapter) MarkEvaluating(ctx context.Context, tx evalapp.Tx, tenantID, agentID, versionID string) error {
	return a.transition(ctx, tx, tenantID, agentID, versionID, string(domain.StatusEvaluating), "", nil)
}

func (a *versionLifecycleAdapter) MarkEligible(ctx context.Context, tx evalapp.Tx, tenantID, agentID, versionID string) error {
	now, err := tx.Now(ctx)
	if err != nil {
		return err
	}
	return a.transition(ctx, tx, tenantID, agentID, versionID, string(domain.StatusEligible), "", &now)
}

func (a *versionLifecycleAdapter) MarkRejected(ctx context.Context, tx evalapp.Tx, tenantID, agentID, versionID, reason string) error {
	return a.transition(ctx, tx, tenantID, agentID, versionID, string(domain.StatusRejected), reason, nil)
}

func (a *versionLifecycleAdapter) transition(
	ctx context.Context,
	tx evalapp.Tx,
	tenantID, agentID, versionID, status, reason string,
	promotedAt *time.Time,
) error {
	var promoted any
	if promotedAt != nil {
		promoted = *promotedAt
	}
	tag, err := tx.Exec(ctx, `
		UPDATE agent_versions
		SET status=$5,
		    promoted_at=COALESCE($6, promoted_at),
		    rejected_reason=COALESCE($7, rejected_reason)
		WHERE tenant_id=$1 AND agent_id=$2 AND id=$3 AND status=$4`,
		tenantID, agentID, versionID, currentStatusFor(status), status,
		promoted, nullStringValue(reason),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrStateConflict
	}
	return nil
}

func currentStatusFor(target string) string {
	switch target {
	case string(domain.StatusEvaluating):
		return string(domain.StatusDraft)
	case string(domain.StatusEligible):
		return string(domain.StatusEvaluating)
	case string(domain.StatusRejected):
		return string(domain.StatusEvaluating)
	default:
		return ""
	}
}

// nullStringValue mirrors the local helper so the adapter can stay self-contained.
func nullStringValue(value string) any {
	if value == "" {
		return sql.NullString{}
	}
	return value
}

var _ evalapp.VersionLifecyclePort = (*versionLifecycleAdapter)(nil)

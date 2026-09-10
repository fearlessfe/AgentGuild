package postgres

import (
	"context"
	"encoding/json"

	"agentguild.dev/agentguild/backend/internal/reward/application"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

type disputeRepository struct{ q queryer }

const disputeColumns = `
	resource_tenant_id, id, lock_id, decision_hash, status, reason,
	opened_by, opened_at, resolution, resolved_by, resolved_at,
	resolved_agent_amount_minor`

func (r *disputeRepository) Insert(ctx context.Context, dispute *domain.Dispute) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO reward_disputes (`+disputeColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		dispute.ResourceTenantID, dispute.ID, dispute.LockID, dispute.DecisionHash,
		string(dispute.Status), dispute.Reason, dispute.OpenedBy, dispute.OpenedAt,
		resolutionValue(dispute.Resolution), dispute.ResolvedBy, dispute.ResolvedAt,
		dispute.ResolvedAgentAmountMinor,
	)
	return writeError(err)
}

func (r *disputeRepository) GetByLock(ctx context.Context, tenantID, lockID string) (*domain.Dispute, error) {
	return r.scanOne(ctx, `
		SELECT`+disputeColumns+`
		FROM reward_disputes
		WHERE resource_tenant_id=$1 AND lock_id=$2`, tenantID, lockID)
}

func (r *disputeRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Dispute, error) {
	return r.scanOne(ctx, `
		SELECT`+disputeColumns+`
		FROM reward_disputes
		WHERE resource_tenant_id=$1 AND id=$2`, tenantID, id)
}

// Save 只允许把 open 推进到 resolved：裁决一旦落库就不可改写。
func (r *disputeRepository) Save(ctx context.Context, dispute *domain.Dispute) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE reward_disputes
		SET status=$3, resolution=$4, resolved_by=$5, resolved_at=$6,
		    resolved_agent_amount_minor=$7
		WHERE resource_tenant_id=$1 AND id=$2 AND status='open'`,
		dispute.ResourceTenantID, dispute.ID, string(dispute.Status),
		resolutionValue(dispute.Resolution), dispute.ResolvedBy,
		dispute.ResolvedAt, dispute.ResolvedAgentAmountMinor,
	)
	if err != nil {
		return writeError(err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrStateConflict
	}
	return nil
}

// AppendEvent 幂等追加争议事件：(dispute_id, event_type) 唯一，重复投递
// 不会产生第二条记录。
func (r *disputeRepository) AppendEvent(ctx context.Context, event application.DisputeEvent) error {
	payload, err := json.Marshal(emptyIfNilPayload(event.Payload))
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO reward_dispute_events (
			dispute_id, event_type, actor_id, payload, occurred_at
		) VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (dispute_id, event_type) DO NOTHING`,
		event.DisputeID, event.EventType, event.ActorID, payload, event.OccurredAt,
	)
	return writeError(err)
}

func (r *disputeRepository) scanOne(ctx context.Context, query string, args ...any) (*domain.Dispute, error) {
	var dispute domain.Dispute
	var status string
	var resolution *string
	err := r.q.QueryRow(ctx, query, args...).Scan(
		&dispute.ResourceTenantID, &dispute.ID, &dispute.LockID, &dispute.DecisionHash,
		&status, &dispute.Reason, &dispute.OpenedBy, &dispute.OpenedAt,
		&resolution, &dispute.ResolvedBy, &dispute.ResolvedAt,
		&dispute.ResolvedAgentAmountMinor,
	)
	if notFound(err) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	dispute.Status = domain.DisputeStatus(status)
	if resolution != nil {
		value := domain.DisputeResolution(*resolution)
		dispute.Resolution = &value
	}
	return &dispute, nil
}

func resolutionValue(resolution *domain.DisputeResolution) *string {
	if resolution == nil {
		return nil
	}
	value := string(*resolution)
	return &value
}

func emptyIfNilPayload(payload map[string]string) map[string]string {
	if payload == nil {
		return map[string]string{}
	}
	return payload
}

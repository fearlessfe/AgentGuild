package postgres

import (
	"context"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
	"github.com/jackc/pgx/v5"
)

type lockRepository struct{ q queryer }

const lockColumns = `
	resource_tenant_id, id, policy_id, task_id, execution_id, agent_id,
	agent_version_id, policy_hash, policy_snapshot, currency,
	locked_amount_minor, challenge_period_seconds, status, locked_at,
	expires_at, releasable_at, released_at, refunded_at, disputed_at, expired_at`

func (r *lockRepository) Insert(ctx context.Context, lock *domain.Lock) error {
	snapshot, err := json.Marshal(lock.PolicySnapshot)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO reward_locks (`+lockColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		lock.ResourceTenantID, lock.ID, lock.PolicyID, lock.TaskID, lock.ExecutionID,
		lock.AgentID, lock.AgentVersionID, lock.PolicyHash, snapshot,
		string(lock.Currency), lock.LockedAmountMinor,
		int64(lock.ChallengePeriod/time.Second), string(lock.Status),
		lock.LockedAt, lock.ExpiresAt, lock.ReleasableAt, lock.ReleasedAt,
		lock.RefundedAt, lock.DisputedAt, lock.ExpiredAt,
	)
	return writeError(err)
}

func (r *lockRepository) GetByID(ctx context.Context, tenantID, id string) (*domain.Lock, error) {
	return r.scanOne(ctx, `
		SELECT`+lockColumns+`
		FROM reward_locks
		WHERE resource_tenant_id=$1 AND id=$2`, tenantID, id)
}

func (r *lockRepository) GetByExecution(ctx context.Context, tenantID, executionID string) (*domain.Lock, error) {
	return r.scanOne(ctx, `
		SELECT`+lockColumns+`
		FROM reward_locks
		WHERE resource_tenant_id=$1 AND execution_id=$2`, tenantID, executionID)
}

func (r *lockRepository) Save(ctx context.Context, lock *domain.Lock) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE reward_locks
		SET status=$3, releasable_at=$4, released_at=$5, refunded_at=$6,
		    disputed_at=$7, expired_at=$8
		WHERE resource_tenant_id=$1 AND id=$2`,
		lock.ResourceTenantID, lock.ID, string(lock.Status), lock.ReleasableAt,
		lock.ReleasedAt, lock.RefundedAt, lock.DisputedAt, lock.ExpiredAt,
	)
	if err != nil {
		return writeError(err)
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *lockRepository) ListByStatus(ctx context.Context, status domain.LockStatus, limit int) ([]domain.Lock, error) {
	rows, err := r.q.Query(ctx, `
		SELECT`+lockColumns+`
		FROM reward_locks
		WHERE status=$1
		ORDER BY locked_at, id
		LIMIT $2`, string(status), limit)
	if err != nil {
		return nil, err
	}
	return scanLocks(rows)
}

// ListExpired 返回超过 expires_at 且仍占用资金的锁。争议中的锁被排除：
// 裁决优先于自动过期，否则争议一方可以靠拖延让资金被自动作废。
func (r *lockRepository) ListExpired(ctx context.Context, now time.Time, limit int) ([]domain.Lock, error) {
	rows, err := r.q.Query(ctx, `
		SELECT`+lockColumns+`
		FROM reward_locks
		WHERE status IN ('locked','releasable') AND expires_at <= $1
		ORDER BY expires_at, id
		LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	return scanLocks(rows)
}

func (r *lockRepository) scanOne(ctx context.Context, query string, args ...any) (*domain.Lock, error) {
	rows, err := r.q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	locks, err := scanLocks(rows)
	if err != nil {
		return nil, err
	}
	if len(locks) == 0 {
		return nil, domain.ErrNotFound
	}
	return &locks[0], nil
}

func scanLocks(rows pgx.Rows) ([]domain.Lock, error) {
	defer rows.Close()
	var locks []domain.Lock
	for rows.Next() {
		var lock domain.Lock
		var snapshot []byte
		var currency, status string
		var challengeSeconds int64
		if err := rows.Scan(
			&lock.ResourceTenantID, &lock.ID, &lock.PolicyID, &lock.TaskID,
			&lock.ExecutionID, &lock.AgentID, &lock.AgentVersionID, &lock.PolicyHash,
			&snapshot, &currency, &lock.LockedAmountMinor, &challengeSeconds,
			&status, &lock.LockedAt, &lock.ExpiresAt, &lock.ReleasableAt,
			&lock.ReleasedAt, &lock.RefundedAt, &lock.DisputedAt, &lock.ExpiredAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(snapshot, &lock.PolicySnapshot); err != nil {
			return nil, err
		}
		lock.Currency = domain.Currency(currency)
		lock.Status = domain.LockStatus(status)
		lock.ChallengePeriod = time.Duration(challengeSeconds) * time.Second
		locks = append(locks, lock)
	}
	return locks, rows.Err()
}

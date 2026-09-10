package postgres

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

type escrowRepository struct{ q queryer }

func (r *escrowRepository) EnsureAccount(ctx context.Context, tenantID string, currency domain.Currency) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO sponsor_escrow_accounts (tenant_id, currency)
		VALUES ($1,$2)
		ON CONFLICT (tenant_id, currency) DO NOTHING`, tenantID, string(currency))
	return writeError(err)
}

func (r *escrowRepository) Get(ctx context.Context, tenantID string, currency domain.Currency) (*domain.EscrowAccount, error) {
	account := domain.EscrowAccount{TenantID: tenantID, Currency: currency}
	err := r.q.QueryRow(ctx, `
		SELECT available_minor, locked_minor, updated_at
		FROM sponsor_escrow_accounts
		WHERE tenant_id=$1 AND currency=$2`, tenantID, string(currency),
	).Scan(&account.AvailableMinor, &account.LockedMinor, &account.UpdatedAt)
	if notFound(err) {
		// 尚未充值的租户余额视为零，而不是"账户不存在"——调用方只关心
		// 能不能承诺这笔钱。
		return &domain.EscrowAccount{TenantID: tenantID, Currency: currency}, nil
	}
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// ApplyEntry 幂等记账：先以 idempotency_key 抢占分录，抢到才动余额。
//
// 返回 false 表示该分录此前已存在，余额未被二次改变。这是重复充值回调、
// 重复支付、重复退款共同的兜底（doc §10）。
func (r *escrowRepository) ApplyEntry(ctx context.Context, entry domain.EscrowEntry) (bool, error) {
	if err := r.EnsureAccount(ctx, entry.TenantID, entry.Currency); err != nil {
		return false, err
	}
	tag, err := r.q.Exec(ctx, `
		INSERT INTO sponsor_escrow_entries (
			tenant_id, currency, entry_type, amount_minor, available_delta,
			locked_delta, reference_kind, reference_id, idempotency_key, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (tenant_id, idempotency_key) DO NOTHING`,
		entry.TenantID, string(entry.Currency), string(entry.EntryType),
		entry.AmountMinor, entry.AvailableDelta, entry.LockedDelta,
		entry.ReferenceKind, entry.ReferenceID, entry.IdempotencyKey, entry.CreatedAt,
	)
	if err != nil {
		return false, writeError(err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := r.q.Exec(ctx, `
		UPDATE sponsor_escrow_accounts
		SET available_minor = available_minor + $3,
		    locked_minor = locked_minor + $4,
		    updated_at = $5
		WHERE tenant_id=$1 AND currency=$2`,
		entry.TenantID, string(entry.Currency),
		entry.AvailableDelta, entry.LockedDelta, entry.CreatedAt,
	); err != nil {
		return false, writeError(err)
	}
	return true, nil
}

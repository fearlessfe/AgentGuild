package postgres

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

type receiptRepository struct{ q queryer }

const receiptColumns = `
	resource_tenant_id, decision_hash, provider, provider_reference, kind,
	status, currency, amount_minor, recipient_ref, failure_reason, occurred_at`

// Append 幂等落回执：(provider, provider_reference, status) 唯一，
// 因此 provider 的重复回调返回 false 且不产生第二条事实（doc §10）。
func (r *receiptRepository) Append(ctx context.Context, receipt *domain.Receipt) (bool, error) {
	tag, err := r.q.Exec(ctx, `
		INSERT INTO payment_receipts (`+receiptColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (provider, provider_reference, status) DO NOTHING`,
		receipt.ResourceTenantID, receipt.DecisionHash, receipt.Provider,
		receipt.Reference, string(receipt.Kind), string(receipt.State),
		string(receipt.Currency), receipt.AmountMinor, receipt.RecipientRef,
		receipt.FailureReason, receipt.OccurredAt,
	)
	if err != nil {
		return false, writeError(err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *receiptRepository) ListByDecision(ctx context.Context, decisionHash string) ([]domain.Receipt, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id,`+receiptColumns+`, recorded_at
		FROM payment_receipts
		WHERE decision_hash=$1
		ORDER BY occurred_at, id`, decisionHash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var receipts []domain.Receipt
	for rows.Next() {
		var receipt domain.Receipt
		var kind, status, currency string
		if err := rows.Scan(
			&receipt.ID, &receipt.ResourceTenantID, &receipt.DecisionHash,
			&receipt.Provider, &receipt.Reference, &kind, &status, &currency,
			&receipt.AmountMinor, &receipt.RecipientRef, &receipt.FailureReason,
			&receipt.OccurredAt, &receipt.RecordedAt,
		); err != nil {
			return nil, err
		}
		receipt.Kind = domain.ReceiptKind(kind)
		receipt.State = domain.ReceiptState(status)
		receipt.Currency = domain.Currency(currency)
		receipts = append(receipts, receipt)
	}
	return receipts, rows.Err()
}

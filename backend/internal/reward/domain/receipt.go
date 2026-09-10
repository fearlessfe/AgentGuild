package domain

import (
	"strings"
	"time"
)

// ReceiptKind 与 ReceiptState 与 settlement 包的取值一一对应。这里重新声明
// 一份，是为了让领域层不反向依赖基础设施包。
type ReceiptKind string

const (
	ReceiptPayment ReceiptKind = "payment"
	ReceiptRefund  ReceiptKind = "refund"
)

type ReceiptState string

const (
	ReceiptPending ReceiptState = "pending"
	ReceiptSettled ReceiptState = "settled"
	ReceiptFailed  ReceiptState = "failed"
)

// Receipt 是 provider 回执在账本中的不可变事实。
//
// (provider, provider_reference, status) 唯一：同一笔支付的重复回调不会
// 产生第二条记录，也不会二次改变余额（doc §10）。
type Receipt struct {
	ID               int64
	ResourceTenantID string
	DecisionHash     string
	Provider         string
	Reference        string
	Kind             ReceiptKind
	State            ReceiptState
	Currency         Currency
	AmountMinor      int64
	RecipientRef     string
	FailureReason    string
	OccurredAt       time.Time
	RecordedAt       time.Time
}

type NewReceiptParams struct {
	ResourceTenantID string
	DecisionHash     string
	Provider         string
	Reference        string
	Kind             ReceiptKind
	State            ReceiptState
	Currency         Currency
	AmountMinor      int64
	RecipientRef     string
	FailureReason    string
	OccurredAt       time.Time
}

func NewReceipt(params NewReceiptParams) (*Receipt, error) {
	for _, field := range []struct{ name, value string }{
		{"resource_tenant_id", params.ResourceTenantID},
		{"provider", params.Provider},
		{"reference", params.Reference},
	} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	if !isHex64(params.DecisionHash) {
		return nil, invalid("decision_hash")
	}
	if !isHex64(params.RecipientRef) {
		return nil, invalid("recipient_ref")
	}
	if params.Kind != ReceiptPayment && params.Kind != ReceiptRefund {
		return nil, invalid("kind")
	}
	if params.State != ReceiptPending && params.State != ReceiptSettled && params.State != ReceiptFailed {
		return nil, invalid("state")
	}
	if !ValidCurrency(params.Currency) {
		return nil, invalid("currency")
	}
	if params.AmountMinor < 0 || params.AmountMinor > MaxAmountMinor {
		return nil, invalid("amount_minor")
	}
	// 只有 failed 回执才允许带失败原因，避免成功回执里混进误导性文本。
	if params.State != ReceiptFailed && params.FailureReason != "" {
		return nil, invalid("failure_reason")
	}
	if params.OccurredAt.IsZero() {
		return nil, invalid("occurred_at")
	}
	return &Receipt{
		ResourceTenantID: params.ResourceTenantID, DecisionHash: params.DecisionHash,
		Provider: params.Provider, Reference: params.Reference, Kind: params.Kind,
		State: params.State, Currency: params.Currency, AmountMinor: params.AmountMinor,
		RecipientRef: params.RecipientRef, FailureReason: params.FailureReason,
		OccurredAt: params.OccurredAt.UTC(),
	}, nil
}

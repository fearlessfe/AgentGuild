// Package settlement 定义链下结算提供方的抽象。
//
// 第一阶段不接真实支付、不写合约（doc §7），只保留接口与 fake 实现，让
// 奖励账本可以在没有资金通道的情况下端到端跑通。
//
// 全部操作以 decision_hash 为幂等键：同一个 RewardDecision 无论重放多少次
// Pay/Refund，都只产生一笔资金动作和一份回执（doc §10）。
package settlement

import (
	"context"
	"errors"
	"time"
)

// Kind 区分付款与退款两类资金动作。
type Kind string

const (
	KindPayment Kind = "payment"
	KindRefund  Kind = "refund"
)

// State 是回执的终态。pending 表示提供方已受理但尚未落地。
type State string

const (
	StatePending State = "pending"
	StateSettled State = "settled"
	StateFailed  State = "failed"
)

// Request 是一次资金动作的完整输入。它刻意只携带公开可验证的字段：
// decision_hash、金额、币种与 recipient_ref，不含钱包地址、Issue 或代码。
type Request struct {
	// DecisionHash 同时是业务标识和幂等键。
	DecisionHash string
	Kind         Kind
	Currency     string
	AmountMinor  int64
	// RecipientRef = sha256(chain|address)，证明付给了哪个目的地而不泄露地址。
	RecipientRef string
}

// Receipt 是提供方返回的回执事实。它会被原样落入 payment_receipts 账本。
type Receipt struct {
	DecisionHash  string
	Provider      string
	Reference     string
	Kind          Kind
	State         State
	Currency      string
	AmountMinor   int64
	RecipientRef  string
	FailureReason string
	OccurredAt    time.Time
}

// Provider 是结算提供方。三个方法全部幂等。
type Provider interface {
	// Name 是写入回执的提供方标识，必须与配置值一致。
	Name() string
	// Pay 付款。对同一 DecisionHash 的重复调用返回首次的回执。
	Pay(ctx context.Context, request Request) (Receipt, error)
	// Refund 退款。对同一 DecisionHash 的重复调用返回首次的回执。
	Refund(ctx context.Context, request Request) (Receipt, error)
	// Status 查询某个 decision 的最新回执；从未提交过时返回 ErrNotFound。
	Status(ctx context.Context, decisionHash string) (Receipt, error)
}

var (
	// ErrNotFound 表示提供方没有该 decision 的任何记录。
	ErrNotFound = errors.New("settlement: receipt not found")
	// ErrInvalidRequest 表示请求本身不合法，重试也不会成功。
	ErrInvalidRequest = errors.New("settlement: request is invalid")
	// ErrConflict 表示同一 decision 上出现了互相矛盾的资金动作
	// （例如已付款又要求以不同金额付款）。
	ErrConflict = errors.New("settlement: conflicting request for decision")
)

// Validate 做提供方无关的基本校验，避免每个实现各写一遍。
func (r Request) Validate() error {
	if r.DecisionHash == "" || r.RecipientRef == "" || r.Currency == "" {
		return ErrInvalidRequest
	}
	if r.Kind != KindPayment && r.Kind != KindRefund {
		return ErrInvalidRequest
	}
	if r.AmountMinor < 0 {
		return ErrInvalidRequest
	}
	return nil
}

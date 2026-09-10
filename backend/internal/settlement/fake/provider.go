// Package fake 提供内存版的结算提供方，供开发、演示与验收测试使用。
//
// 它刻意模拟真实支付通道的三种难点：重复回调、退款、以及"先失败再重试成功"。
// 这样奖励账本的幂等语义可以在没有任何外部依赖的情况下被测出来（doc §10）。
package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"agentguild.dev/agentguild/backend/internal/settlement"
)

// ProviderName 是 fake 提供方写入回执的标识。
const ProviderName = "fake"

// Options 控制失败注入。默认零值是一个总是成功的提供方。
type Options struct {
	// FailuresBeforeSuccess 让某个 decision 的前 N 次 Pay 返回 failed 回执，
	// 第 N+1 次才成功，用来验证"失败重试"路径同样幂等。
	FailuresBeforeSuccess map[string]int
	// Now 固定时钟，便于测试断言回执时间。
	Now func() time.Time
}

// Provider 是内存实现。它对每个 decision 只保留一条终态回执，
// 因此重复 Pay/Refund 天然是 no-op。
type Provider struct {
	mu       sync.Mutex
	receipts map[string]settlement.Receipt
	// attempts 记录每个 decision 的付款尝试次数，供失败注入使用。
	attempts map[string]int
	failures map[string]int
	sequence int64
	now      func() time.Time
}

func New(options Options) *Provider {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	failures := make(map[string]int, len(options.FailuresBeforeSuccess))
	for key, count := range options.FailuresBeforeSuccess {
		failures[key] = count
	}
	return &Provider{
		receipts: make(map[string]settlement.Receipt),
		attempts: make(map[string]int),
		failures: failures,
		now:      now,
	}
}

func (p *Provider) Name() string { return ProviderName }

func (p *Provider) Pay(ctx context.Context, request settlement.Request) (settlement.Receipt, error) {
	request.Kind = settlement.KindPayment
	return p.submit(ctx, request)
}

func (p *Provider) Refund(ctx context.Context, request settlement.Request) (settlement.Receipt, error) {
	request.Kind = settlement.KindRefund
	return p.submit(ctx, request)
}

func (p *Provider) Status(_ context.Context, decisionHash string) (settlement.Receipt, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	receipt, found := p.receipts[decisionHash]
	if !found {
		return settlement.Receipt{}, settlement.ErrNotFound
	}
	return receipt, nil
}

// submit 是 Pay/Refund 的共同实现：以 decision_hash 为幂等键，命中已有终态
// 回执时直接返回它，不产生第二笔资金动作。
func (p *Provider) submit(ctx context.Context, request settlement.Request) (settlement.Receipt, error) {
	if err := ctx.Err(); err != nil {
		return settlement.Receipt{}, err
	}
	if err := request.Validate(); err != nil {
		return settlement.Receipt{}, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, found := p.receipts[request.DecisionHash]; found && existing.State == settlement.StateSettled {
		// 已结算的 decision 只接受完全一致的重放；金额或方向不同说明调用方
		// 记账出了问题，这里必须显式冲突而不是静默覆盖。
		if existing.Kind != request.Kind || existing.AmountMinor != request.AmountMinor ||
			existing.Currency != request.Currency || existing.RecipientRef != request.RecipientRef {
			return settlement.Receipt{}, settlement.ErrConflict
		}
		return existing, nil
	}

	p.attempts[request.DecisionHash]++
	attempt := p.attempts[request.DecisionHash]
	p.sequence++

	receipt := settlement.Receipt{
		DecisionHash: request.DecisionHash,
		Provider:     ProviderName,
		Reference:    fmt.Sprintf("fake-%s-%d", request.DecisionHash[:8], p.sequence),
		Kind:         request.Kind,
		State:        settlement.StateSettled,
		Currency:     request.Currency,
		AmountMinor:  request.AmountMinor,
		RecipientRef: request.RecipientRef,
		OccurredAt:   p.now(),
	}
	if budget := p.failures[request.DecisionHash]; attempt <= budget {
		receipt.State = settlement.StateFailed
		receipt.FailureReason = "injected settlement failure"
	}
	p.receipts[request.DecisionHash] = receipt
	return receipt, nil
}

var _ settlement.Provider = (*Provider)(nil)

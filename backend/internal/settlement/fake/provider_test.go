package fake_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/settlement"
	"agentguild.dev/agentguild/backend/internal/settlement/fake"
	"github.com/stretchr/testify/require"
)

func request(hash string, amount int64) settlement.Request {
	return settlement.Request{
		DecisionHash: hash, Kind: settlement.KindPayment, Currency: "USDC",
		AmountMinor: amount, RecipientRef: hash,
	}
}

const decisionHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestFakeProviderIsIdempotentPerDecision(t *testing.T) {
	provider := fake.New(fake.Options{})
	ctx := context.Background()

	first, err := provider.Pay(ctx, request(decisionHash, 100))
	require.NoError(t, err)
	require.Equal(t, settlement.StateSettled, first.State)

	// 重复回调必须返回同一份回执，而不是第二笔付款。
	second, err := provider.Pay(ctx, request(decisionHash, 100))
	require.NoError(t, err)
	require.Equal(t, first, second)

	status, err := provider.Status(ctx, decisionHash)
	require.NoError(t, err)
	require.Equal(t, first, status)

	// 金额不一致的重放是冲突，不允许静默覆盖。
	_, err = provider.Pay(ctx, request(decisionHash, 101))
	require.ErrorIs(t, err, settlement.ErrConflict)

	_, err = provider.Status(ctx, "bbbb")
	require.ErrorIs(t, err, settlement.ErrNotFound)
}

func TestFakeProviderRetriesAfterInjectedFailure(t *testing.T) {
	provider := fake.New(fake.Options{
		FailuresBeforeSuccess: map[string]int{decisionHash: 1},
	})
	ctx := context.Background()

	failed, err := provider.Pay(ctx, request(decisionHash, 100))
	require.NoError(t, err)
	require.Equal(t, settlement.StateFailed, failed.State)
	require.NotEmpty(t, failed.FailureReason)

	// 失败的 decision 允许重试，重试成功后转为 settled。
	retried, err := provider.Pay(ctx, request(decisionHash, 100))
	require.NoError(t, err)
	require.Equal(t, settlement.StateSettled, retried.State)
	require.Empty(t, retried.FailureReason)
}

func TestFakeProviderRefundIsIdempotent(t *testing.T) {
	provider := fake.New(fake.Options{})
	ctx := context.Background()
	refundRequest := request(decisionHash, 100)
	refundRequest.Kind = settlement.KindRefund

	first, err := provider.Refund(ctx, refundRequest)
	require.NoError(t, err)
	require.Equal(t, settlement.KindRefund, first.Kind)
	second, err := provider.Refund(ctx, refundRequest)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestFakeProviderRejectsInvalidRequests(t *testing.T) {
	provider := fake.New(fake.Options{})
	ctx := context.Background()
	_, err := provider.Pay(ctx, settlement.Request{})
	require.ErrorIs(t, err, settlement.ErrInvalidRequest)
}

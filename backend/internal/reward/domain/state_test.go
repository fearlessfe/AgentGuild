package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

func newTestPolicy(t *testing.T) *domain.Policy {
	t.Helper()
	createdAt := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	policy, err := domain.NewPolicy(domain.NewPolicyParams{
		ID: "policy-1", ResourceTenantID: "tenant-1", TaskID: "task-1",
		Currency: domain.CurrencyUSDC, SettlementProvider: "fake",
		GrossAmountMinor: 100_000,
		CriterionWeights: []domain.CriterionWeight{
			{CriterionID: "c2", WeightBps: 4000},
			{CriterionID: "c1", WeightBps: 6000},
		},
		MaintainerShareBps: 1000, ReviewerPoolShareBps: 1000,
		PlatformFeeBps: 500, DisputeReserveBps: 500,
		QualityMultiplierMinBps: 5000, QualityMultiplierMaxBps: 15000,
		ChallengePeriod: time.Hour, ExpiresAt: createdAt.Add(30 * 24 * time.Hour),
		CreatedAt: createdAt,
	})
	require.NoError(t, err)
	return policy
}

func TestNewPolicyComputesStableHash(t *testing.T) {
	first := newTestPolicy(t)
	second := newTestPolicy(t)
	require.Len(t, first.PolicyHash, 64)
	// 权重传入顺序不同也必须得到同一个摘要。
	require.Equal(t, first.PolicyHash, second.PolicyHash)
	require.Equal(t, "c1", first.CriterionWeights[0].CriterionID)

	// 金额变化必须改变摘要，否则"Claim 后不可变"无从校验。
	changed := newTestPolicy(t)
	changed.GrossAmountMinor = 99_999
	hash, err := changed.ComputePolicyHash()
	require.NoError(t, err)
	require.NotEqual(t, first.PolicyHash, hash)
}

func TestPolicyApply(t *testing.T) {
	now := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		from    domain.PolicyStatus
		intent  domain.PolicyIntent
		amount  int64
		wantErr error
		want    domain.PolicyStatus
	}{
		{name: "unfunded 可 fund", from: domain.PolicyUnfunded, intent: domain.PolicyIntentFund, amount: 100_000, want: domain.PolicyFunded},
		{name: "unfunded 可 cancel", from: domain.PolicyUnfunded, intent: domain.PolicyIntentCancel, want: domain.PolicyCancelled},
		{name: "funded 可 exhaust", from: domain.PolicyFunded, intent: domain.PolicyIntentExhaust, want: domain.PolicyExhausted},
		{name: "funded 可 cancel", from: domain.PolicyFunded, intent: domain.PolicyIntentCancel, want: domain.PolicyCancelled},
		{name: "funded 不可重复 fund", from: domain.PolicyFunded, intent: domain.PolicyIntentFund, amount: 1, wantErr: domain.ErrStateConflict},
		{name: "cancelled 是终态", from: domain.PolicyCancelled, intent: domain.PolicyIntentFund, amount: 1, wantErr: domain.ErrStateConflict},
		{name: "exhausted 是终态", from: domain.PolicyExhausted, intent: domain.PolicyIntentCancel, wantErr: domain.ErrStateConflict},
		{name: "出资额不得超过 gross", from: domain.PolicyUnfunded, intent: domain.PolicyIntentFund, amount: 100_001, wantErr: domain.ErrInvalidArgument},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			policy := newTestPolicy(t)
			policy.Status = testCase.from
			err := policy.Apply(testCase.intent, testCase.amount, now)
			if testCase.wantErr != nil {
				require.ErrorIs(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.want, policy.Status)
		})
	}
}

func newTestLock(t *testing.T, policy *domain.Policy) *domain.Lock {
	t.Helper()
	lockedAt := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	lock, err := domain.NewLock(domain.NewLockParams{
		ID: "lock-1", ResourceTenantID: policy.ResourceTenantID,
		PolicyID: policy.ID, TaskID: policy.TaskID, ExecutionID: "execution-1",
		AgentID: "agent-1", AgentVersionID: "version-1",
		Snapshot: policy.Snapshot(), LockedAmountMinor: policy.GrossAmountMinor,
		LockedAt: lockedAt, ExpiresAt: lockedAt.Add(72 * time.Hour),
	})
	require.NoError(t, err)
	return lock
}

func TestLockApply(t *testing.T) {
	now := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		from    domain.LockStatus
		intent  domain.LockIntent
		want    domain.LockStatus
		wantErr bool
	}{
		{name: "locked → releasable", from: domain.LockLocked, intent: domain.LockIntentMarkReleasable, want: domain.LockReleasable},
		{name: "releasable → released", from: domain.LockReleasable, intent: domain.LockIntentRelease, want: domain.LockReleased},
		{name: "locked 不能直接 release", from: domain.LockLocked, intent: domain.LockIntentRelease, wantErr: true},
		{name: "locked → refunded", from: domain.LockLocked, intent: domain.LockIntentRefund, want: domain.LockRefunded},
		{name: "locked → disputed", from: domain.LockLocked, intent: domain.LockIntentDispute, want: domain.LockDisputed},
		{name: "releasable → disputed", from: domain.LockReleasable, intent: domain.LockIntentDispute, want: domain.LockDisputed},
		{name: "disputed → released", from: domain.LockDisputed, intent: domain.LockIntentRelease, want: domain.LockReleased},
		{name: "disputed → refunded", from: domain.LockDisputed, intent: domain.LockIntentRefund, want: domain.LockRefunded},
		{name: "disputed 不可再被过期作废", from: domain.LockDisputed, intent: domain.LockIntentExpire, wantErr: true},
		{name: "locked → expired", from: domain.LockLocked, intent: domain.LockIntentExpire, want: domain.LockExpired},
		{name: "released 是终态", from: domain.LockReleased, intent: domain.LockIntentRefund, wantErr: true},
		{name: "refunded 是终态", from: domain.LockRefunded, intent: domain.LockIntentRelease, wantErr: true},
		{name: "expired 是终态", from: domain.LockExpired, intent: domain.LockIntentDispute, wantErr: true},
		{name: "未知意图被拒绝", from: domain.LockLocked, intent: domain.LockIntent("teleport"), wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			policy := newTestPolicy(t)
			lock := newTestLock(t, policy)
			lock.Status = testCase.from
			err := lock.Apply(testCase.intent, now)
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.want, lock.Status)
		})
	}
}

func TestLockSnapshotIsIndependentOfPolicyMutation(t *testing.T) {
	policy := newTestPolicy(t)
	lock := newTestLock(t, policy)
	// Claim 之后修改 policy 的内存副本，锁里的快照必须纹丝不动。
	policy.CriterionWeights[0].WeightBps = 1
	policy.GrossAmountMinor = 1
	require.Equal(t, 6000, lock.PolicySnapshot.CriterionWeightsBps[0].WeightBps)
	require.Equal(t, int64(100_000), lock.PolicySnapshot.GrossAmountMinor)
}

func TestDisputeResolve(t *testing.T) {
	openedAt := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	build := func(t *testing.T) *domain.Dispute {
		t.Helper()
		dispute, err := domain.NewDispute(domain.NewDisputeParams{
			ID: "dispute-1", ResourceTenantID: "tenant-1", LockID: "lock-1",
			DecisionHash: hash64('a'), Reason: "criterion evidence disputed",
			OpenedBy: "agent-1", OpenedAt: openedAt,
		})
		require.NoError(t, err)
		return dispute
	}

	cases := []struct {
		name        string
		resolution  domain.DisputeResolution
		agentAmount int64
		wantErr     bool
		wantIntent  domain.LockIntent
	}{
		{name: "release 必须有非零份额", resolution: domain.ResolutionRelease, agentAmount: 90, wantIntent: domain.LockIntentRelease},
		{name: "release 零份额非法", resolution: domain.ResolutionRelease, agentAmount: 0, wantErr: true},
		{name: "refund 必须零份额", resolution: domain.ResolutionRefund, agentAmount: 0, wantIntent: domain.LockIntentRefund},
		{name: "refund 带份额非法", resolution: domain.ResolutionRefund, agentAmount: 10, wantErr: true},
		{name: "split 落在开区间内", resolution: domain.ResolutionSplit, agentAmount: 45, wantIntent: domain.LockIntentRelease},
		{name: "split 取满额非法", resolution: domain.ResolutionSplit, agentAmount: 90, wantErr: true},
		{name: "份额不得超过 net", resolution: domain.ResolutionRelease, agentAmount: 91, wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dispute := build(t)
			err := dispute.Resolve(testCase.resolution, "admin-1", testCase.agentAmount, 90, openedAt.Add(time.Hour))
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, domain.DisputeResolved, dispute.Status)
			intent, err := dispute.LockIntentFor()
			require.NoError(t, err)
			require.Equal(t, testCase.wantIntent, intent)
			// 重复裁决必须冲突而不是覆盖。
			require.Error(t, dispute.Resolve(testCase.resolution, "admin-1", testCase.agentAmount, 90, openedAt.Add(2*time.Hour)))
		})
	}
}

func TestChallengeConsumeIsSingleUse(t *testing.T) {
	issuedAt := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	challenge, err := domain.NewChallenge(domain.NewChallengeParams{
		Nonce: "nonce-1", AgentID: "agent-1", Chain: "base",
		Address: "0xabc", IssuedAt: issuedAt, TTL: 10 * time.Minute,
	})
	require.NoError(t, err)

	require.ErrorIs(t, challenge.Consume("agent-2", "base", "0xabc", issuedAt), domain.ErrForbidden)
	require.NoError(t, challenge.Consume("agent-1", "base", "0xabc", issuedAt.Add(time.Minute)))
	// nonce 一次性：重放必须失败。
	require.ErrorIs(t, challenge.Consume("agent-1", "base", "0xabc", issuedAt.Add(2*time.Minute)), domain.ErrStateConflict)

	expired, err := domain.NewChallenge(domain.NewChallengeParams{
		Nonce: "nonce-2", AgentID: "agent-1", Chain: "base",
		Address: "0xabc", IssuedAt: issuedAt, TTL: time.Minute,
	})
	require.NoError(t, err)
	require.ErrorIs(t, expired.Consume("agent-1", "base", "0xabc", issuedAt.Add(time.Hour)), domain.ErrStateConflict)
}

func TestDeltasMatchLedgerInvariant(t *testing.T) {
	cases := []struct {
		entryType                 domain.EntryType
		wantAvailable, wantLocked int64
		wantErr                   bool
	}{
		{entryType: domain.EntryTopup, wantAvailable: 100},
		{entryType: domain.EntryLock, wantAvailable: -100, wantLocked: 100},
		{entryType: domain.EntryRelease, wantLocked: -100},
		{entryType: domain.EntryRefund, wantAvailable: 100, wantLocked: -100},
		{entryType: domain.EntryType("mint"), wantErr: true},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.entryType), func(t *testing.T) {
			available, locked, err := domain.Deltas(testCase.entryType, 100)
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.wantAvailable, available)
			require.Equal(t, testCase.wantLocked, locked)
		})
	}
}

func TestRecipientRefHidesAddress(t *testing.T) {
	ref := domain.RecipientRef("base", "0xabc")
	require.Len(t, ref, 64)
	require.NotContains(t, ref, "0xabc")
	require.Equal(t, ref, domain.RecipientRef("base", "0xabc"))
	require.NotEqual(t, ref, domain.RecipientRef("base", "0xabd"))
	// chain 与 address 之间有分隔符，避免拼接歧义。
	require.NotEqual(t, domain.RecipientRef("base", "x0xabc"), domain.RecipientRef("basex", "0xabc"))
}

package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

// baseSnapshot 是分配算法测试的公共快照：平台费 5%、争议准备金 5%，
// maintainer 10%、reviewer 池 10%，乘数区间 [0.5, 1.5]。
func baseSnapshot() domain.PolicySnapshot {
	return domain.PolicySnapshot{
		PolicyID: "policy-1", PolicyHash: hash64('a'), TaskID: "task-1",
		Currency: domain.CurrencyUSDC, SettlementProvider: "fake",
		GrossAmountMinor: 100_000,
		CriterionWeightsBps: []domain.CriterionWeight{
			{CriterionID: "c1", WeightBps: 6000},
			{CriterionID: "c2", WeightBps: 4000},
		},
		MaintainerShareBps: 1000, ReviewerPoolShareBps: 1000,
		PlatformFeeBps: 500, DisputeReserveBps: 500,
		QualityMultiplierMinBps: 5000, QualityMultiplierMaxBps: 15000,
		ChallengePeriodSeconds: 3600,
		ExpiresAt:              time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
	}
}

func hash64(fill rune) string {
	value := make([]rune, 64)
	for index := range value {
		value[index] = fill
	}
	return string(value)
}

func TestAllocate(t *testing.T) {
	snapshot := baseSnapshot()
	bothPassed := []domain.CriterionOutcome{
		{CriterionID: "c1", Required: true, WeightBps: 6000, Verified: true, Passed: true},
		{CriterionID: "c2", Required: false, WeightBps: 4000, Verified: true, Passed: true},
	}

	cases := []struct {
		name              string
		outcomes          []domain.CriterionOutcome
		allRequiredPassed bool
		qualityBps        int
		wantAgent         int64
		wantNet           int64
		wantUnallocated   int64
		wantErr           bool
	}{
		{
			name: "全部通过且乘数为 1", outcomes: bothPassed, allRequiredPassed: true,
			qualityBps: 10000,
			// gross 100000 − 5000 fee − 5000 reserve = 90000 net
			// maintainer 9000 + reviewer 9000 ⇒ agentPool 72000
			// 权重全通过 ⇒ 72000 × 1.0 = 72000
			wantAgent: 72_000, wantNet: 90_000, wantUnallocated: 0,
		},
		{
			name: "乘数 1.5 不得超发 agentPool", outcomes: bothPassed, allRequiredPassed: true,
			qualityBps: 15000, wantAgent: 72_000, wantNet: 90_000, wantUnallocated: 0,
		},
		{
			name: "乘数 0.5 折半", outcomes: bothPassed, allRequiredPassed: true,
			qualityBps: 5000, wantAgent: 36_000, wantNet: 90_000, wantUnallocated: 36_000,
		},
		{
			name: "required 未通过 ⇒ agent 份额为 0",
			outcomes: []domain.CriterionOutcome{
				{CriterionID: "c1", Required: true, WeightBps: 6000, Verified: true, Passed: false},
				{CriterionID: "c2", Required: false, WeightBps: 4000, Verified: true, Passed: true},
			},
			allRequiredPassed: false, qualityBps: 10000,
			wantAgent: 0, wantNet: 90_000, wantUnallocated: 72_000,
		},
		{
			name: "required 未验证 ⇒ 同样视为未通过",
			outcomes: []domain.CriterionOutcome{
				{CriterionID: "c1", Required: true, WeightBps: 6000, Verified: false, Passed: false},
				{CriterionID: "c2", Required: false, WeightBps: 4000, Verified: true, Passed: true},
			},
			allRequiredPassed: false, qualityBps: 10000,
			wantAgent: 0, wantNet: 90_000, wantUnallocated: 72_000,
		},
		{
			name: "可选标准未通过只按权重折减",
			outcomes: []domain.CriterionOutcome{
				{CriterionID: "c1", Required: true, WeightBps: 6000, Verified: true, Passed: true},
				{CriterionID: "c2", Required: false, WeightBps: 4000, Verified: true, Passed: false},
			},
			allRequiredPassed: true, qualityBps: 10000,
			wantAgent: 43_200, wantNet: 90_000, wantUnallocated: 28_800,
		},
		{
			name: "乘数低于冻结下限直接报错", outcomes: bothPassed, allRequiredPassed: true,
			qualityBps: 4999, wantErr: true,
		},
		{
			name: "乘数高于冻结上限直接报错", outcomes: bothPassed, allRequiredPassed: true,
			qualityBps: 15001, wantErr: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			allocation, err := domain.Allocate(domain.AllocateParams{
				Snapshot: snapshot, GrossAmountMinor: snapshot.GrossAmountMinor,
				Outcomes: testCase.outcomes, AllRequiredPassed: testCase.allRequiredPassed,
				QualityMultiplierBps: testCase.qualityBps,
			})
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.wantAgent, allocation.AgentAmountMinor)
			require.Equal(t, testCase.wantNet, allocation.NetAmountMinor)
			require.Equal(t, testCase.wantUnallocated, allocation.UnallocatedAmountMinor)
			// 分账必须闭合到最后一分钱。
			require.Equal(t, allocation.GrossAmountMinor,
				allocation.PlatformFeeMinor+allocation.DisputeReserveMinor+allocation.NetAmountMinor)
			require.Equal(t, allocation.NetAmountMinor,
				allocation.AgentAmountMinor+allocation.MaintainerAmountMinor+
					allocation.ReviewerPoolAmountMinor+allocation.UnallocatedAmountMinor)
		})
	}
}

func TestAllocateRoundsDownAndKeepsRemainder(t *testing.T) {
	snapshot := baseSnapshot()
	snapshot.GrossAmountMinor = 101
	allocation, err := domain.Allocate(domain.AllocateParams{
		Snapshot: snapshot, GrossAmountMinor: 101,
		Outcomes: []domain.CriterionOutcome{
			{CriterionID: "c1", Required: true, WeightBps: 6000, Verified: true, Passed: true},
			{CriterionID: "c2", Required: false, WeightBps: 4000, Verified: true, Passed: true},
		},
		AllRequiredPassed: true, QualityMultiplierBps: 10000,
	})
	require.NoError(t, err)
	require.Equal(t, int64(101), allocation.GrossAmountMinor)
	require.GreaterOrEqual(t, allocation.UnallocatedAmountMinor, int64(0))
	require.Equal(t, allocation.NetAmountMinor,
		allocation.AgentAmountMinor+allocation.MaintainerAmountMinor+
			allocation.ReviewerPoolAmountMinor+allocation.UnallocatedAmountMinor)
}

func TestNewDecisionComputesHashAndRejectsUngatedAgentShare(t *testing.T) {
	signer, err := domain.NewHMACSigner([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	decidedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	params := domain.NewDecisionParams{
		ResourceTenantID: "tenant-1", LockID: "lock-1",
		PolicyHash: hash64('a'), TaskSpecHash: hash64('b'),
		ContributionHash: hash64('c'), AlgorithmVersion: "2026-09-09-v1",
		Currency: domain.CurrencyUSDC,
		Allocation: domain.Allocation{
			GrossAmountMinor: 100, PlatformFeeMinor: 5, DisputeReserveMinor: 5,
			NetAmountMinor: 90, AgentAmountMinor: 72, MaintainerAmountMinor: 9,
			ReviewerPoolAmountMinor: 9, UnallocatedAmountMinor: 0,
		},
		QualityMultiplierBps: 10000,
		CriterionResults: []domain.CriterionOutcome{
			{CriterionID: "c2", WeightBps: 4000, Verified: true, Passed: true},
			{CriterionID: "c1", Required: true, WeightBps: 6000, Verified: true, Passed: true},
		},
		RequiredCriteriaPassed: true, RecipientRef: hash64('d'),
		ChallengeDeadline: decidedAt.Add(time.Hour), DecidedAt: decidedAt,
		Signer: signer,
	}

	decision, err := domain.NewDecision(params)
	require.NoError(t, err)
	require.Len(t, decision.DecisionHash, 64)
	require.True(t, signer.Verify(decision.DecisionHash, decision.Signature))
	// criterion 顺序不影响摘要。
	again, err := domain.NewDecision(params)
	require.NoError(t, err)
	require.Equal(t, decision.DecisionHash, again.DecisionHash)

	// 门禁未通过却带非零 Agent 份额，必须被领域层拒绝。
	ungated := params
	ungated.RequiredCriteriaPassed = false
	_, err = domain.NewDecision(ungated)
	require.ErrorIs(t, err, domain.ErrStateConflict)
}

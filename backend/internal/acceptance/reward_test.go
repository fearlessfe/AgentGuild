package acceptance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/acceptance"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	reputationpostgres "agentguild.dev/agentguild/backend/internal/reputation/postgres"
	rewardapp "agentguild.dev/agentguild/backend/internal/reward/application"
	rewarddomain "agentguild.dev/agentguild/backend/internal/reward/domain"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 完整闭环
// ---------------------------------------------------------------------------

// TestSponsoredRewardHappyPathClosesTheLoop 走完一整条经济链路：
// sponsor 充值 → 建契约并出资 → Agent claim（锁定契约快照）→ 全部验收标准
// 通过 → 挑战期届满 → 经结算提供方释放 → 重复回调为 no-op。
func TestSponsoredRewardHappyPathClosesTheLoop(t *testing.T) {
	env := acceptance.StartRewards(t, 0)
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")

	env.TopUp("topup-1", 100_000)
	policy := env.FundedPolicy("task-1", 100_000)
	require.Equal(t, "funded", policy.Status)

	env.Claim("public-1", "agent-1", "", "execution-1")
	locked := env.Lock("execution-1")
	require.Equal(t, "locked", locked.Status)
	require.Equal(t, policy.PolicyHash, locked.PolicyHash)
	require.Equal(t, int64(100_000), env.Escrow().LockedMinor)
	require.Zero(t, env.Escrow().AvailableMinor)

	destination := env.BindDestination(acceptance.AgentPrincipal("agent-1", ""), "base", "0xagent1")
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c2", "job-2", false, true)

	decision := env.Decide("execution-1")
	require.True(t, decision.RequiredCriteriaPassed)
	require.Equal(t, destination.RecipientRef, decision.RecipientRef)
	require.Positive(t, decision.AgentAmountMinor)
	require.Equal(t, "releasable", env.Lock("execution-1").Status)

	released := env.Release(decision.LockID)
	require.Equal(t, "released", released.Status)
	require.Zero(t, env.Escrow().LockedMinor)
	require.Len(t, env.Receipts(decision.DecisionHash), 1)

	// 重复回调：状态、余额与回执都不再变化。
	before := env.Escrow()
	env.Release(decision.LockID)
	require.Equal(t, before, env.Escrow())
	require.Len(t, env.Receipts(decision.DecisionHash), 1)
}

// ---------------------------------------------------------------------------
// doc §10-1：同一 Contribution Event 重复投递不会重复增加声望或奖励
// ---------------------------------------------------------------------------

func TestDuplicateContributionEventDoesNotDoubleCountReward(t *testing.T) {
	env := acceptance.StartRewards(t, 0)
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")
	env.TopUp("topup-1", 100_000)
	// 相同 request_id 的重复充值不得二次加钱。
	env.TopUp("topup-1", 100_000)
	require.Equal(t, int64(100_000), env.Escrow().AvailableMinor)

	env.FundedPolicy("task-1", 100_000)
	env.Claim("public-1", "agent-1", "", "execution-1")
	env.BindDestination(acceptance.AgentPrincipal("agent-1", ""), "base", "0xagent1")

	// 同一条验证证据投递两次：幂等键相同，账本只保留一条。
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c2", "job-2", false, true)

	first := env.Decide("execution-1")
	// 重复决策同样幂等：同一个 decision_hash，不产生第二份分配。
	second := env.Decide("execution-1")
	require.Equal(t, first.DecisionHash, second.DecisionHash)
	require.Equal(t, first.AgentAmountMinor, second.AgentAmountMinor)

	env.Release(first.LockID)
	after := env.Escrow()
	env.Release(first.LockID)
	require.Equal(t, after, env.Escrow(), "重复释放不得改变余额")
	require.Len(t, env.Receipts(first.DecisionHash), 1)
}

// ---------------------------------------------------------------------------
// doc §10-2：声望可从事件账本重算；此处确认奖励侧不受影响
// ---------------------------------------------------------------------------

func TestReputationRebuildLeavesRewardLedgerUntouched(t *testing.T) {
	env := acceptance.StartRewards(t, 0)
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")
	env.TopUp("topup-1", 100_000)
	env.FundedPolicy("task-1", 100_000)
	env.Claim("public-1", "agent-1", "", "execution-1")
	env.BindDestination(acceptance.AgentPrincipal("agent-1", ""), "base", "0xagent1")
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c2", "job-2", false, true)
	decision := env.Decide("execution-1")
	env.Release(decision.LockID)

	before := env.Escrow()
	ctx := context.Background()

	// 删光声望投影并在固定时刻全量重算：声望是投影，奖励是账本，
	// 前者可以被丢弃重建，后者不能。
	params := reputationpostgres.NewParamsRepository(env.DB)
	require.NoError(t, params.EnsureVersion(ctx, reputationdomain.DefaultParams()))
	cards := reputationpostgres.NewScoreCardRepository(env.DB)
	rebuilder, err := reputationapp.NewRebuilder(reputationpostgres.NewFactSource(env.DB), params, cards)
	require.NoError(t, err)
	_, err = env.DB.Exec(ctx, `DELETE FROM agent_reputation_projections`)
	require.NoError(t, err)
	_, err = rebuilder.Rebuild(ctx, reputationdomain.DefaultAlgorithmVersionV2,
		time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	require.Equal(t, before, env.Escrow(), "重算声望不得改变任何资金状态")
	stored, err := env.Service.GetDecision(ctx, decision.DecisionHash)
	require.NoError(t, err)
	// 时刻字段跨存储往返只保证瞬时相等，不保证 time.Location 相同，
	// 因此逐字段比较而不是整体 require.Equal。
	require.Equal(t, decision.DecisionHash, stored.Data.DecisionHash)
	require.Equal(t, decision.AgentAmountMinor, stored.Data.AgentAmountMinor)
	require.Equal(t, decision.Signature, stored.Data.Signature)
}

// ---------------------------------------------------------------------------
// doc §10-3：新 Agent Version 不继承旧版本样本，lifetime 贡献不丢
// ---------------------------------------------------------------------------

func TestNewAgentVersionDoesNotRewriteRewardAttribution(t *testing.T) {
	env := acceptance.StartRewards(t, 0)
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")
	env.SeedTask("task-2", "public-2")
	env.TopUp("topup-1", 200_000)
	env.FundedPolicy("task-1", 100_000)
	env.FundedPolicy("task-2", 100_000)

	env.Claim("public-1", "agent-1", "agent-1-v1", "execution-1")
	first := env.Lock("execution-1")
	require.Equal(t, "agent-1-v1", first.AgentVersionID)

	// 建新版本后再领第二个任务：新锁归属新版本，旧锁一字不改。
	env.SeedAgentVersion("agent-1", "agent-1-v2", 2)
	env.Claim("public-2", "agent-1", "agent-1-v2", "execution-2")

	require.Equal(t, first, env.Lock("execution-1"),
		"新建 Agent Version 不得改写历史锁的版本归属")
	second := env.Lock("execution-2")
	require.Equal(t, "agent-1-v2", second.AgentVersionID)
	// lifetime 归属仍然是同一个 Agent：两条锁指向同一个 agent_id。
	require.Equal(t, first.AgentID, second.AgentID)
}

// ---------------------------------------------------------------------------
// doc §10-4：RewardPolicy 在 Claim 后不可变
// ---------------------------------------------------------------------------

// TestRewardPolicyIsImmutableAfterClaim 证明 Issue 更新改不动已锁定的经济契约。
//
// 快照连同 policy_hash 一起冻结在锁上，因此即便有人直接改数据库里的 policy
// 行（比 Issue 同步更粗暴的路径），决策仍按 Claim 时刻的金额、criterion 权重
// 与挑战期计算。
func TestRewardPolicyIsImmutableAfterClaim(t *testing.T) {
	env := acceptance.StartRewards(t, 0)
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")
	env.TopUp("topup-1", 100_000)
	policy := env.FundedPolicy("task-1", 100_000)

	env.Claim("public-1", "agent-1", "", "execution-1")
	locked := env.Lock("execution-1")
	require.Equal(t, policy.PolicyHash, locked.PolicyHash)
	require.Equal(t, int64(100_000), locked.LockedAmountMinor)

	// 应用层没有任何"改 policy"的入口：只有创建、出资与取消。
	// 而取消在存在活跃锁时必须被拒绝。
	_, err := env.Service.CancelRewardPolicy(context.Background(),
		acceptance.SponsorPrincipal(), policy.ID)
	require.ErrorIs(t, err, rewarddomain.ErrStateConflict)

	// 直接改库模拟"Issue 更新写回契约"，锁上的快照必须无动于衷。
	_, err = env.DB.Exec(context.Background(), `
		UPDATE reward_policies
		SET gross_amount_minor = 50, funded_amount_minor = 1,
		    challenge_period_seconds = 999999,
		    criterion_weights_bps = '{"c2":10000}'::jsonb
		WHERE resource_tenant_id=$1 AND id=$2`, acceptance.RewardSponsorTenant, policy.ID)
	require.NoError(t, err)

	env.BindDestination(acceptance.AgentPrincipal("agent-1", ""), "base", "0xagent1")
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c2", "job-2", false, true)
	decision := env.Decide("execution-1")

	require.Equal(t, policy.PolicyHash, decision.PolicyHash)
	require.Equal(t, int64(100_000), decision.GrossAmountMinor,
		"金额必须来自 Claim 时刻的快照，而不是被改写后的契约行")
	weights := map[string]int{}
	for _, outcome := range decision.CriterionResults {
		weights[outcome.CriterionID] = outcome.WeightBps
	}
	require.Equal(t, 6000, weights["c1"], "criterion 权重同样冻结在快照里")
	require.Equal(t, 4000, weights["c2"])
}

// ---------------------------------------------------------------------------
// doc §10-5：required criterion 未通过时不能释放 Agent 奖励
// ---------------------------------------------------------------------------

// TestRequiredCriterionNotPassedWithholdsAgentReward 覆盖两种"未通过"：
// 一条必需标准明确失败，以及一条必需标准从未被验证过。
// 后者是最容易写错的分支——未验证绝不等于通过。
func TestRequiredCriterionNotPassedWithholdsAgentReward(t *testing.T) {
	cases := []struct {
		name string
		// record 决定这次执行往 criterion 账本里写什么。
		record func(env *acceptance.RewardEnv, taskID, executionID string)
	}{
		{
			name: "required_criterion_failed",
			record: func(env *acceptance.RewardEnv, taskID, executionID string) {
				env.RecordCriterion(taskID, executionID, "c1", "job-1", true, false)
				env.RecordCriterion(taskID, executionID, "c2", "job-2", false, true)
			},
		},
		{
			name: "required_criterion_never_verified",
			record: func(env *acceptance.RewardEnv, taskID, executionID string) {
				// 只验证了可选标准，必需标准 c1 一条结果都没有。
				env.RecordCriterion(taskID, executionID, "c2", "job-2", false, true)
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			env := acceptance.StartRewards(t, 0)
			env.SeedAgent("agent-1")
			env.SeedTask("task-1", "public-1")
			env.TopUp("topup-1", 100_000)
			env.FundedPolicy("task-1", 100_000)
			env.Claim("public-1", "agent-1", "", "execution-1")
			env.BindDestination(acceptance.AgentPrincipal("agent-1", ""), "base", "0xagent1")
			testCase.record(env, "task-1", "execution-1")

			decision := env.Decide("execution-1")
			require.False(t, decision.RequiredCriteriaPassed)
			require.Zero(t, decision.AgentAmountMinor, "必需标准未通过时 Agent 份额必须为 0")
			require.Equal(t, rewarddomain.UnassignedRecipientRef, decision.RecipientRef,
				"没有可付金额时不得绑定任何收款目的地")

			env.Release(decision.LockID)
			// 未分配部分与争议准备金全额回到 sponsor。
			escrow := env.Escrow()
			require.Zero(t, escrow.LockedMinor)
			require.Equal(t, decision.UnallocatedAmountMinor+decision.DisputeReserveMinor,
				escrow.AvailableMinor)
		})
	}
}

// ---------------------------------------------------------------------------
// doc §10-6：RewardDecision 可被第三方验证且不泄露私有内容
// ---------------------------------------------------------------------------

func TestRewardDecisionIsThirdPartyVerifiableWithoutPrivateEvidence(t *testing.T) {
	env := acceptance.StartRewards(t, 0)
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")
	env.TopUp("topup-1", 100_000)
	env.FundedPolicy("task-1", 100_000)
	env.Claim("public-1", "agent-1", "", "execution-1")
	env.BindDestination(acceptance.AgentPrincipal("agent-1", ""), "base", "0xagent1")
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c2", "job-2", false, true)
	decision := env.Decide("execution-1")

	// 完全匿名地取回决策。
	status, body := env.GetJSON("/v1/rewards/decisions/"+decision.DecisionHash, "")
	require.Equal(t, http.StatusOK, status, body)

	var envelope struct {
		Data rewardapp.DecisionView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &envelope))
	require.Equal(t, decision.DecisionHash, envelope.Data.DecisionHash)
	require.Equal(t, decision.TaskSpecHash, envelope.Data.TaskSpecHash)
	require.Equal(t, decision.ContributionHash, envelope.Data.ContributionHash)
	require.Equal(t, decision.AgentAmountMinor, envelope.Data.AgentAmountMinor)
	require.Equal(t, decision.CriterionResults, envelope.Data.CriterionResults)
	require.Equal(t, decision.RecipientRef, envelope.Data.RecipientRef)

	// 第三方用公开摘要与签名独立验证，不需要访问任何私有数据。
	signer, err := rewarddomain.NewHMACSigner([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	require.True(t, signer.Verify(envelope.Data.DecisionHash, envelope.Data.Signature),
		"第三方必须能只用公开 hash 与签名完成验证")

	// 响应里没有 Issue 正文、分析结论或租户标识。
	for _, needle := range []string{"Diagnosis", "Solution", "Widget fails", acceptance.RewardSponsorTenant, "tenant_id"} {
		require.NotContains(t, body, needle, "决策响应泄露了私有内容：%s", needle)
	}
}

// ---------------------------------------------------------------------------
// doc §10-7：回调、重复支付、退款、争议与过期全部幂等
// ---------------------------------------------------------------------------

func TestSettlementRefundDisputeAndExpiryAreIdempotent(t *testing.T) {
	t.Run("duplicate_payment_callback", func(t *testing.T) {
		env := acceptance.StartRewards(t, 0)
		decision := seedReleasableDecision(t, env)
		env.Release(decision.LockID)
		settled := env.Escrow()

		// 模拟提供方重复回调：连打三次，账本与余额都不再变化。
		for range 3 {
			env.Release(decision.LockID)
		}
		require.Equal(t, settled, env.Escrow())
		require.Len(t, env.Receipts(decision.DecisionHash), 1)
		require.Equal(t, "released", env.Lock("execution-1").Status)
	})

	t.Run("refund", func(t *testing.T) {
		env := acceptance.StartRewards(t, 0)
		decision := seedReleasableDecision(t, env)

		_, err := env.Service.Refund(context.Background(), rewardapp.SettleReward{
			ResourceTenantID: acceptance.RewardSponsorTenant, LockID: decision.LockID,
		})
		require.NoError(t, err)
		refunded := env.Escrow()
		require.Zero(t, refunded.LockedMinor)
		require.Equal(t, int64(100_000), refunded.AvailableMinor, "退款必须把整笔锁定还给 sponsor")

		_, err = env.Service.Refund(context.Background(), rewardapp.SettleReward{
			ResourceTenantID: acceptance.RewardSponsorTenant, LockID: decision.LockID,
		})
		require.NoError(t, err)
		require.Equal(t, refunded, env.Escrow(), "重复退款是 no-op")
	})

	t.Run("dispute_resolution", func(t *testing.T) {
		// 挑战期非零，决策产生后仍可发起争议。
		env := acceptance.StartRewards(t, time.Hour)
		decision := seedReleasableDecision(t, env)

		opened, err := env.Access.OpenDispute(context.Background(),
			acceptance.AgentPrincipal("agent-1", ""), decision.LockID, "criterion evidence is wrong")
		require.NoError(t, err)
		require.Equal(t, "open", opened.Data.Status)
		require.Equal(t, "disputed", env.Lock("execution-1").Status)

		// 重复发起返回既有争议，不派生第二条裁决路径。
		again, err := env.Access.OpenDispute(context.Background(),
			acceptance.AgentPrincipal("agent-1", ""), decision.LockID, "again")
		require.NoError(t, err)
		require.Equal(t, opened.Data.ID, again.Data.ID)

		admin := acceptance.SponsorPrincipal()
		admin.IsAdmin = true
		resolved, err := env.Access.ResolveDispute(context.Background(), admin,
			opened.Data.ID, "refund", 0)
		require.NoError(t, err)
		require.Equal(t, "resolved", resolved.Data.Status)
		afterResolve := env.Escrow()
		require.Zero(t, afterResolve.LockedMinor)

		// 重复裁决返回既有结果，不二次动账。
		_, err = env.Access.ResolveDispute(context.Background(), admin, opened.Data.ID, "release", 999)
		require.NoError(t, err)
		require.Equal(t, afterResolve, env.Escrow())
	})

	t.Run("expiry", func(t *testing.T) {
		env := acceptance.StartRewards(t, 0)
		env.SeedAgent("agent-1")
		env.SeedTask("task-1", "public-1")
		env.TopUp("topup-1", 100_000)
		env.FundedPolicy("task-1", 100_000)
		env.Claim("public-1", "agent-1", "", "execution-1")

		lock := env.Lock("execution-1")
		env.ExpireLockDeadline(lock.ID)
		require.NoError(t, env.Service.ExpireLock(context.Background(),
			acceptance.RewardSponsorTenant, lock.ID))
		expired := env.Escrow()
		require.Zero(t, expired.LockedMinor)
		require.Equal(t, int64(100_000), expired.AvailableMinor)
		require.Equal(t, "expired", env.Lock("execution-1").Status)

		// 过期从未让资金离开托管，所以提供方那边不该有任何记录。
		require.NoError(t, env.Service.ExpireLock(context.Background(),
			acceptance.RewardSponsorTenant, lock.ID))
		require.Equal(t, expired, env.Escrow())
		// worker 再扫一轮同样是 no-op。
		env.RunWorker()
		require.Equal(t, expired, env.Escrow())
	})
}

// ---------------------------------------------------------------------------
// doc §10-8：钱包轮换不改变历史归属，旧钱包不能再次领取同一奖励
// ---------------------------------------------------------------------------

func TestWalletRotationPreservesHistoricalAttribution(t *testing.T) {
	env := acceptance.StartRewards(t, 0)
	agent := acceptance.AgentPrincipal("agent-1", "")
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")
	env.TopUp("topup-1", 100_000)
	env.FundedPolicy("task-1", 100_000)
	env.Claim("public-1", "agent-1", "", "execution-1")

	old := env.BindDestination(agent, "base", "0xold")
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c2", "job-2", false, true)
	decision := env.Decide("execution-1")
	require.Equal(t, old.RecipientRef, decision.RecipientRef)

	// 轮换钱包：旧目的地被撤销，新目的地生效。
	rotated := env.BindDestination(agent, "base", "0xnew")
	require.NotEqual(t, old.RecipientRef, rotated.RecipientRef)
	destinations, err := env.Service.ListPayoutDestinations(context.Background(), agent)
	require.NoError(t, err)
	statuses := map[string]string{}
	for _, item := range destinations.Data.Items {
		statuses[item.RecipientRef] = item.Status
	}
	require.Equal(t, "revoked", statuses[old.RecipientRef])
	require.Equal(t, "verified", statuses[rotated.RecipientRef])

	// 历史归属不变：决策里冻结的 recipient_ref 仍指向旧目的地。
	stored, err := env.Service.GetDecision(context.Background(), decision.DecisionHash)
	require.NoError(t, err)
	require.Equal(t, old.RecipientRef, stored.Data.RecipientRef)

	// 释放按冻结的 recipient_ref 付款，且只付一次。
	env.Release(decision.LockID)
	receipts := env.Receipts(decision.DecisionHash)
	require.Len(t, receipts, 1)
	require.Equal(t, old.RecipientRef, receipts[0].RecipientRef)

	// 旧钱包不能再次领取同一笔奖励：锁已是终态，重放没有任何效果。
	after := env.Escrow()
	env.Release(decision.LockID)
	require.Equal(t, after, env.Escrow())
	require.Len(t, env.Receipts(decision.DecisionHash), 1)
	providerReceipt, err := env.ProviderStatus(decision.DecisionHash)
	require.NoError(t, err)
	require.Equal(t, old.RecipientRef, providerReceipt.RecipientRef)
}

// seedReleasableDecision 建立一条"全部标准通过、已生成决策"的执行，
// 供幂等相关子用例复用。
func seedReleasableDecision(t *testing.T, env *acceptance.RewardEnv) rewardapp.DecisionView {
	t.Helper()
	env.SeedAgent("agent-1")
	env.SeedTask("task-1", "public-1")
	env.TopUp("topup-1", 100_000)
	env.FundedPolicy("task-1", 100_000)
	env.Claim("public-1", "agent-1", "", "execution-1")
	env.BindDestination(acceptance.AgentPrincipal("agent-1", ""), "base", "0xagent1")
	env.RecordCriterion("task-1", "execution-1", "c1", "job-1", true, true)
	env.RecordCriterion("task-1", "execution-1", "c2", "job-2", false, true)
	return env.Decide("execution-1")
}

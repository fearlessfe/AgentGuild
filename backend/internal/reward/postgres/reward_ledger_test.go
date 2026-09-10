package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	publictaskapp "agentguild.dev/agentguild/backend/internal/publictask/application"
	publictaskpostgres "agentguild.dev/agentguild/backend/internal/publictask/postgres"
	rewardapp "agentguild.dev/agentguild/backend/internal/reward/application"
	rewarddomain "agentguild.dev/agentguild/backend/internal/reward/domain"
	rewardpostgres "agentguild.dev/agentguild/backend/internal/reward/postgres"
	rewardworker "agentguild.dev/agentguild/backend/internal/reward/worker"
	settlementfake "agentguild.dev/agentguild/backend/internal/settlement/fake"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const sponsorTenant = "tenant-sponsor"

type harness struct {
	db      *pgxpool.Pool
	service *rewardapp.Service
	claims  *publictaskpostgres.Repository
}

func newHarness(t *testing.T) *harness {
	// 挑战期为 0，让大多数测试不必等待即可释放。
	return newHarnessWithChallengePeriod(t, 0)
}

func newHarnessWithChallengePeriod(t *testing.T, challengePeriod time.Duration) *harness {
	t.Helper()
	db := testdb.StartPostgres(t)
	store := rewardpostgres.NewStore(db)
	signer, err := rewarddomain.NewHMACSigner([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)

	var counter atomic.Int64
	newID := func() string {
		return "id-" + strconv.FormatInt(counter.Add(1), 10)
	}
	service, err := rewardapp.NewService(rewardapp.Options{
		Store: store, Provider: settlementfake.New(settlementfake.Options{}),
		Evidence: rewardpostgres.NewEvidenceSource(db), Signer: signer,
		AlgorithmVersion:       rewarddomain.DefaultAlgorithmVersion,
		DefaultChallengePeriod: challengePeriod, NewID: newID,
	})
	require.NoError(t, err)

	claims := publictaskpostgres.NewRepository(db,
		publictaskpostgres.WithClaimParticipants(
			rewardpostgres.NewClaimParticipant(rewardpostgres.ClaimParticipantOptions{NewID: newID}),
		),
	)
	return &harness{db: db, service: service, claims: claims}
}

func sponsorPrincipal() auth.Principal {
	return auth.Principal{Type: auth.PrincipalTypeHuman, TenantID: sponsorTenant, OwnerID: "owner-1"}
}

func agentPrincipal(agentID string) auth.Principal {
	return auth.Principal{Type: auth.PrincipalTypeAgent, AgentID: agentID, AgentVersionID: agentID + "-v"}
}

func seedAgent(t *testing.T, db *pgxpool.Pool, agentID string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES ($1,$1,$1,'active')`, agentID)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		INSERT INTO agent_identity_versions (
			id, agent_id, version_number, status, runtime, model
		) VALUES ($1,$2,1,'active','pi','gpt-5')`, agentID+"-v", agentID)
	require.NoError(t, err)
}

// seedTask 建立一个已发布、带两条验收标准（一必需一可选）的公共任务。
func seedTask(t *testing.T, db *pgxpool.Pool, taskID, publicTaskID string) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			deadline, status
		) VALUES ($1,$2,'publisher-version','bug','Fix widget','Widget fails',
			clock_timestamp() + interval '1 day','open')`, sponsorTenant, taskID)
	require.NoError(t, err)
	specHash := hex.EncodeToString(sha256Sum(taskID))
	_, err = db.Exec(ctx, `
		INSERT INTO public_task_projections (
			id, resource_tenant_id, task_id, task_specification_version_id,
			canonical_repository, source_issue_url, issue_revision, base_commit,
			title, summary, problem_diagnosis, impact, proposed_solution,
			acceptance_criteria, quality_level, difficulty_class, spec_hash,
			status, published_at
		) VALUES ($1,$2,$3,'spec-1','octo/widget','https://example.test/1','rev-1',
			'0123456789abcdef0123456789abcdef01234567','Fix widget','Summary',
			'Diagnosis','Impact','Solution',
			'[{"id":"c1","statement":"tests pass","critical":true,"verifier_kind":"command","expected_result":"green"},
			  {"id":"c2","statement":"docs updated","critical":false,"verifier_kind":"manual","expected_result":"ok"}]'::jsonb,
			'standard','standard',$4,'published',clock_timestamp())`,
		publicTaskID, sponsorTenant, taskID, specHash)
	require.NoError(t, err)
}

func sha256Sum(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func claimRequest(publicTaskID, agentID, suffix string) publictaskapp.PublicClaimRequest {
	return publictaskapp.PublicClaimRequest{
		PublicTaskID: publicTaskID, AgentID: agentID, AgentVersionID: agentID + "-v",
		RequestID:   "request-" + suffix,
		RequestHash: sha256.Sum256([]byte(publicTaskID + "|" + agentID)),
		ExecutionID: "execution-" + suffix, GrantID: "grant-" + suffix,
		OutboxEventID: "outbox-" + suffix, GrantTTL: time.Hour,
		GrantScopes: []participationdomain.Scope{participationdomain.ScopeTaskRead},
	}
}

// fundedPolicy 充值并创建一条已 funded 的契约。
func fundedPolicy(t *testing.T, h *harness, taskID string, gross int64) rewardapp.PolicyView {
	t.Helper()
	ctx := context.Background()
	_, err := h.service.TopUpEscrow(ctx, sponsorPrincipal(), rewardapp.TopUpEscrow{
		RequestID: "topup-" + taskID, Currency: "USDC", AmountMinor: gross,
	})
	require.NoError(t, err)
	created, err := h.service.CreateRewardPolicy(ctx, sponsorPrincipal(), rewardapp.CreateRewardPolicy{
		TaskID: taskID, Currency: "USDC", GrossAmountMinor: gross,
		CriterionWeights: []rewarddomain.CriterionWeight{
			{CriterionID: "c1", WeightBps: 6000},
			{CriterionID: "c2", WeightBps: 4000},
		},
		MaintainerShareBps: 1000, ReviewerPoolShareBps: 1000,
		PlatformFeeBps: 500, DisputeReserveBps: 500,
	})
	require.NoError(t, err)
	funded, err := h.service.FundRewardPolicy(ctx, sponsorPrincipal(), rewardapp.FundRewardPolicy{
		PolicyID: created.Data.ID,
	})
	require.NoError(t, err)
	require.Equal(t, "funded", funded.Data.Status)
	return funded.Data
}

func recordCriterion(t *testing.T, db *pgxpool.Pool, taskID, executionID, criterionID string, critical, passed bool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO execution_criterion_results (
			resource_tenant_id, task_id, execution_id, criterion_id, critical,
			verifier_kind, passed, source_kind, source_id, verified_by, observed_at
		) VALUES ($1,$2,$3,$4,$5,'command',$6,'validation_job','job-'||$4,'validator',
			clock_timestamp())`,
		sponsorTenant, taskID, executionID, criterionID, critical, passed)
	require.NoError(t, err)
}

func TestClaimLocksRewardInTheSameTransaction(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedAgent(t, h.db, "agent-1")
	seedTask(t, h.db, "task-1", "public-1")
	policy := fundedPolicy(t, h, "task-1", 100_000)

	_, err := h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)

	lock, err := h.service.GetExecutionReward(ctx, sponsorTenant, "execution-1")
	require.NoError(t, err)
	require.Equal(t, "locked", lock.Data.Status)
	require.Equal(t, int64(100_000), lock.Data.LockedAmountMinor)
	// 快照连同 policy_hash 一起冻结在锁上。
	require.Equal(t, policy.PolicyHash, lock.Data.PolicyHash)

	escrow, err := h.service.GetEscrow(ctx, sponsorPrincipal(), "USDC")
	require.NoError(t, err)
	require.Equal(t, int64(0), escrow.Data.AvailableMinor)
	require.Equal(t, int64(100_000), escrow.Data.LockedMinor)
}

func TestClaimFailsAndTaskStaysOpenWhenEscrowIsInsufficient(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedAgent(t, h.db, "agent-1")
	seedAgent(t, h.db, "agent-2")
	seedTask(t, h.db, "task-1", "public-1")
	seedTask(t, h.db, "task-2", "public-2")
	// 只充值一次，却给两个任务各建一条同额的 funded 契约：
	// 第二次 claim 必须因为余额不足而整体失败。
	fundedPolicy(t, h, "task-1", 100_000)
	_, err := h.service.CreateRewardPolicy(ctx, sponsorPrincipal(), rewardapp.CreateRewardPolicy{
		TaskID: "task-2", Currency: "USDC", GrossAmountMinor: 100_000,
		PlatformFeeBps: 500, DisputeReserveBps: 500,
	})
	require.NoError(t, err)
	var secondPolicyID string
	require.NoError(t, h.db.QueryRow(ctx, `
		SELECT id FROM reward_policies WHERE resource_tenant_id=$1 AND task_id='task-2'`,
		sponsorTenant).Scan(&secondPolicyID))
	// 直接把第二条契约置为 funded，绕过应用层的余额预检查，
	// 从而验证真正的兜底是数据库的非负余额 CHECK。
	_, err = h.db.Exec(ctx, `
		UPDATE reward_policies
		SET status='funded', funded_amount_minor=gross_amount_minor,
		    funded_at=clock_timestamp()
		WHERE id=$1`, secondPolicyID)
	require.NoError(t, err)

	_, err = h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)
	_, err = h.claims.Claim(ctx, claimRequest("public-2", "agent-2", "2"))
	require.ErrorIs(t, err, rewarddomain.ErrInsufficientEscrow)

	var status string
	require.NoError(t, h.db.QueryRow(ctx,
		`SELECT status FROM tasks WHERE tenant_id=$1 AND id='task-2'`, sponsorTenant).Scan(&status))
	require.Equal(t, "open", status, "无资金背书的 claim 必须整体回滚")
	var executions int
	require.NoError(t, h.db.QueryRow(ctx,
		`SELECT count(*) FROM executions WHERE tenant_id=$1 AND task_id='task-2'`, sponsorTenant).Scan(&executions))
	require.Zero(t, executions)
}

func TestDecisionWithholdsAgentShareWhenRequiredCriterionFails(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedAgent(t, h.db, "agent-1")
	seedTask(t, h.db, "task-1", "public-1")
	fundedPolicy(t, h, "task-1", 100_000)
	_, err := h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)

	// 必需标准失败、可选标准通过。
	recordCriterion(t, h.db, "task-1", "execution-1", "c1", true, false)
	recordCriterion(t, h.db, "task-1", "execution-1", "c2", false, true)

	decision, err := h.service.Decide(ctx, rewardapp.DecideReward{
		ResourceTenantID: sponsorTenant, ExecutionID: "execution-1",
	})
	require.NoError(t, err)
	require.False(t, decision.Data.RequiredCriteriaPassed)
	require.Zero(t, decision.Data.AgentAmountMinor)
	require.Equal(t, rewarddomain.UnassignedRecipientRef, decision.Data.RecipientRef)
	// 重复决策幂等：同一 decision_hash，不产生第二条分配。
	again, err := h.service.Decide(ctx, rewardapp.DecideReward{
		ResourceTenantID: sponsorTenant, ExecutionID: "execution-1",
	})
	require.NoError(t, err)
	require.Equal(t, decision.Data.DecisionHash, again.Data.DecisionHash)

	released, err := h.service.Release(ctx, rewardapp.SettleReward{
		ResourceTenantID: sponsorTenant, LockID: decision.Data.LockID,
	})
	require.NoError(t, err)
	require.Equal(t, "released", released.Data.Status)

	escrow, err := h.service.GetEscrow(ctx, sponsorPrincipal(), "USDC")
	require.NoError(t, err)
	require.Zero(t, escrow.Data.LockedMinor)
	// Agent 份额为 0，未分配部分与争议准备金全额退回 sponsor。
	require.Equal(t, decision.Data.UnallocatedAmountMinor+decision.Data.DisputeReserveMinor,
		escrow.Data.AvailableMinor)

	// 重复释放是 no-op：余额不再变化。
	_, err = h.service.Release(ctx, rewardapp.SettleReward{
		ResourceTenantID: sponsorTenant, LockID: decision.Data.LockID,
	})
	require.NoError(t, err)
	after, err := h.service.GetEscrow(ctx, sponsorPrincipal(), "USDC")
	require.NoError(t, err)
	require.Equal(t, escrow.Data.AvailableMinor, after.Data.AvailableMinor)
}

func TestDecisionPaysVerifiedDestinationAndSurvivesWalletRotation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedAgent(t, h.db, "agent-1")
	seedTask(t, h.db, "task-1", "public-1")
	fundedPolicy(t, h, "task-1", 100_000)
	_, err := h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)

	challenge, err := h.service.ChallengePayoutDestination(ctx, agentPrincipal("agent-1"),
		rewardapp.ChallengePayoutDestination{Chain: "base", Address: "0xold"})
	require.NoError(t, err)
	destination, err := h.service.VerifyPayoutDestination(ctx, agentPrincipal("agent-1"),
		rewardapp.VerifyPayoutDestination{
			Nonce: challenge.Data.Nonce, Chain: "base", Address: "0xold",
		})
	require.NoError(t, err)
	require.Equal(t, "verified", destination.Data.Status)
	// nonce 一次性：重放必须失败。
	_, err = h.service.VerifyPayoutDestination(ctx, agentPrincipal("agent-1"),
		rewardapp.VerifyPayoutDestination{
			Nonce: challenge.Data.Nonce, Chain: "base", Address: "0xold",
		})
	require.ErrorIs(t, err, rewarddomain.ErrStateConflict)

	recordCriterion(t, h.db, "task-1", "execution-1", "c1", true, true)
	recordCriterion(t, h.db, "task-1", "execution-1", "c2", false, true)
	decision, err := h.service.Decide(ctx, rewardapp.DecideReward{
		ResourceTenantID: sponsorTenant, ExecutionID: "execution-1",
	})
	require.NoError(t, err)
	require.True(t, decision.Data.RequiredCriteriaPassed)
	require.Equal(t, int64(72_000), decision.Data.AgentAmountMinor)
	require.Equal(t, destination.Data.RecipientRef, decision.Data.RecipientRef)

	// 钱包轮换：旧目的地被撤销，历史决策里冻结的 recipient_ref 不变。
	rotation, err := h.service.ChallengePayoutDestination(ctx, agentPrincipal("agent-1"),
		rewardapp.ChallengePayoutDestination{Chain: "base", Address: "0xnew"})
	require.NoError(t, err)
	rotated, err := h.service.VerifyPayoutDestination(ctx, agentPrincipal("agent-1"),
		rewardapp.VerifyPayoutDestination{
			Nonce: rotation.Data.Nonce, Chain: "base", Address: "0xnew",
		})
	require.NoError(t, err)
	require.NotEqual(t, destination.Data.RecipientRef, rotated.Data.RecipientRef)

	stored, err := h.service.GetDecision(ctx, decision.Data.DecisionHash)
	require.NoError(t, err)
	require.Equal(t, destination.Data.RecipientRef, stored.Data.RecipientRef)

	_, err = h.service.Release(ctx, rewardapp.SettleReward{
		ResourceTenantID: sponsorTenant, LockID: decision.Data.LockID,
	})
	require.NoError(t, err)
	receipts, err := h.service.ListReceipts(ctx, decision.Data.DecisionHash)
	require.NoError(t, err)
	require.Len(t, receipts, 1)
	require.Equal(t, "settled", receipts[0].State)
	require.Equal(t, destination.Data.RecipientRef, receipts[0].RecipientRef)

	// 重复释放只会拿到同一份回执，不会追加第二条。
	_, err = h.service.Release(ctx, rewardapp.SettleReward{
		ResourceTenantID: sponsorTenant, LockID: decision.Data.LockID,
	})
	require.NoError(t, err)
	receipts, err = h.service.ListReceipts(ctx, decision.Data.DecisionHash)
	require.NoError(t, err)
	require.Len(t, receipts, 1)
}

func TestRewardDecisionsAreAppendOnly(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	seedAgent(t, h.db, "agent-1")
	seedTask(t, h.db, "task-1", "public-1")
	fundedPolicy(t, h, "task-1", 100_000)
	_, err := h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)
	recordCriterion(t, h.db, "task-1", "execution-1", "c1", true, false)
	decision, err := h.service.Decide(ctx, rewardapp.DecideReward{
		ResourceTenantID: sponsorTenant, ExecutionID: "execution-1",
	})
	require.NoError(t, err)

	_, err = h.db.Exec(ctx, `
		UPDATE reward_decisions SET agent_amount_minor=1 WHERE decision_hash=$1`,
		decision.Data.DecisionHash)
	require.Error(t, err)
	_, err = h.db.Exec(ctx, `DELETE FROM reward_decisions WHERE decision_hash=$1`,
		decision.Data.DecisionHash)
	require.Error(t, err)
}

func TestWorkerDecidesReleasesAndExpiresLocks(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	store := rewardpostgres.NewStore(h.db)
	worker, err := rewardworker.NewWorker(h.service, store, rewardworker.Options{})
	require.NoError(t, err)

	seedAgent(t, h.db, "agent-1")
	seedTask(t, h.db, "task-1", "public-1")
	fundedPolicy(t, h, "task-1", 100_000)
	_, err = h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)
	recordCriterion(t, h.db, "task-1", "execution-1", "c1", true, false)

	// 挑战期为 0，因此一轮就能同时完成决策与释放。
	require.NoError(t, worker.RunOnce(ctx))
	lock, err := h.service.GetExecutionReward(ctx, sponsorTenant, "execution-1")
	require.NoError(t, err)
	require.Equal(t, "released", lock.Data.Status)

	// 再跑一轮必须是 no-op：终态的锁不会被二次结算。
	require.NoError(t, worker.RunOnce(ctx))
	receipts, err := h.service.ListReceipts(ctx, lock.Data.DecisionHash)
	require.NoError(t, err)
	require.Len(t, receipts, 1)
}

func TestWorkerExpiresLockWhenNoDecisionIsPossible(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	store := rewardpostgres.NewStore(h.db)
	worker, err := rewardworker.NewWorker(h.service, store, rewardworker.Options{})
	require.NoError(t, err)

	seedAgent(t, h.db, "agent-1")
	seedTask(t, h.db, "task-1", "public-1")
	fundedPolicy(t, h, "task-1", 100_000)
	_, err = h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)
	// 清空规格摘要，让决策无法生成（fail closed），再把锁的过期时刻推到
	// 过去，模拟"长期拿不到可验证证据"的锁。
	_, err = h.db.Exec(ctx, `
		UPDATE public_task_projections SET spec_hash='' WHERE resource_tenant_id=$1`, sponsorTenant)
	require.NoError(t, err)
	_, err = h.db.Exec(ctx, `
		UPDATE reward_locks SET expires_at = locked_at WHERE resource_tenant_id=$1`, sponsorTenant)
	require.NoError(t, err)

	require.NoError(t, worker.RunOnce(ctx))
	lock, err := h.service.GetExecutionReward(ctx, sponsorTenant, "execution-1")
	require.NoError(t, err)
	require.Equal(t, "expired", lock.Data.Status)

	escrow, err := h.service.GetEscrow(ctx, sponsorPrincipal(), "USDC")
	require.NoError(t, err)
	require.Zero(t, escrow.Data.LockedMinor)
	require.Equal(t, int64(100_000), escrow.Data.AvailableMinor)
}

func TestDisputeSuspendsReleaseAndResolvesIdempotently(t *testing.T) {
	h := newHarnessWithChallengePeriod(t, time.Hour)
	ctx := context.Background()
	store := rewardpostgres.NewStore(h.db)
	worker, err := rewardworker.NewWorker(h.service, store, rewardworker.Options{})
	require.NoError(t, err)

	seedAgent(t, h.db, "agent-1")
	seedTask(t, h.db, "task-1", "public-1")
	fundedPolicy(t, h, "task-1", 100_000)
	_, err = h.claims.Claim(ctx, claimRequest("public-1", "agent-1", "1"))
	require.NoError(t, err)
	recordCriterion(t, h.db, "task-1", "execution-1", "c1", true, false)

	decision, err := h.service.Decide(ctx, rewardapp.DecideReward{
		ResourceTenantID: sponsorTenant, ExecutionID: "execution-1",
	})
	require.NoError(t, err)

	dispute, err := h.service.OpenDispute(ctx, agentPrincipal("agent-1"), rewardapp.OpenDispute{
		ResourceTenantID: sponsorTenant, LockID: decision.Data.LockID,
		Reason: "criterion evidence is stale",
	})
	require.NoError(t, err)
	require.Equal(t, "open", dispute.Data.Status)
	// 重复发起返回同一条争议，不会派生第二条裁决路径。
	again, err := h.service.OpenDispute(ctx, agentPrincipal("agent-1"), rewardapp.OpenDispute{
		ResourceTenantID: sponsorTenant, LockID: decision.Data.LockID,
	})
	require.NoError(t, err)
	require.Equal(t, dispute.Data.ID, again.Data.ID)

	// 争议中的锁不会被 worker 自动释放。
	require.NoError(t, worker.RunOnce(ctx))
	lock, err := h.service.GetExecutionReward(ctx, sponsorTenant, "execution-1")
	require.NoError(t, err)
	require.Equal(t, "disputed", lock.Data.Status)

	admin := sponsorPrincipal()
	admin.IsAdmin = true
	resolved, err := h.service.ResolveDispute(ctx, admin, rewardapp.ResolveDispute{
		ResourceTenantID: sponsorTenant, DisputeID: dispute.Data.ID,
		Resolution: "refund", AgentAmountMinor: 0,
	})
	require.NoError(t, err)
	require.Equal(t, "resolved", resolved.Data.Status)

	lock, err = h.service.GetExecutionReward(ctx, sponsorTenant, "execution-1")
	require.NoError(t, err)
	require.Equal(t, "refunded", lock.Data.Status)
	escrow, err := h.service.GetEscrow(ctx, sponsorPrincipal(), "USDC")
	require.NoError(t, err)
	require.Zero(t, escrow.Data.LockedMinor)
	require.Equal(t, int64(100_000), escrow.Data.AvailableMinor)

	// 重复裁决是 no-op，余额不再变化。
	_, err = h.service.ResolveDispute(ctx, admin, rewardapp.ResolveDispute{
		ResourceTenantID: sponsorTenant, DisputeID: dispute.Data.ID,
		Resolution: "release", AgentAmountMinor: 50,
	})
	require.NoError(t, err)
	after, err := h.service.GetEscrow(ctx, sponsorPrincipal(), "USDC")
	require.NoError(t, err)
	require.Equal(t, escrow.Data.AvailableMinor, after.Data.AvailableMinor)
}

package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	"agentguild.dev/agentguild/backend/internal/rewardaccess"
	"agentguild.dev/agentguild/backend/internal/settlement"
	settlementfake "agentguild.dev/agentguild/backend/internal/settlement/fake"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"agentguild.dev/agentguild/backend/internal/transport/rest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// RewardSponsorTenant 是奖励验收测试里唯一的 sponsor 租户。
const RewardSponsorTenant = "tenant-sponsor"

// RewardEnv 是链下奖励账本的端到端验收 harness。
//
// 它刻意与 Env 分开：奖励闭环需要公共任务的 Claim 参与者、结算提供方与
// 奖励 worker 三者协同装配，塞进已经很大的 Env 只会让两条线互相牵连。
type RewardEnv struct {
	T        *testing.T
	DB       *pgxpool.Pool
	Service  *rewardapp.Service
	Access   *rewardaccess.Service
	Claims   *publictaskpostgres.Repository
	Worker   *rewardworker.Worker
	Provider *settlementfake.Provider
	REST     http.Handler

	seq atomic.Int64
}

// StartRewards 启动一个隔离的奖励验收环境。
//
// challengePeriod 为 0 时决策一产生就可释放，绝大多数用例不必等待；
// 需要验证挑战期语义的用例显式传入正数。
func StartRewards(t *testing.T, challengePeriod time.Duration) *RewardEnv {
	t.Helper()
	db := testdb.StartPostgres(t)

	var counter atomic.Int64
	newID := func() string { return "rw-" + strconv.FormatInt(counter.Add(1), 10) }

	provider := settlementfake.New(settlementfake.Options{})
	signer, err := rewarddomain.NewHMACSigner([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	store := rewardpostgres.NewStore(db)
	service, err := rewardapp.NewService(rewardapp.Options{
		Store: store, Provider: provider, Evidence: rewardpostgres.NewEvidenceSource(db),
		Signer: signer, AlgorithmVersion: rewarddomain.DefaultAlgorithmVersion,
		DefaultChallengePeriod: challengePeriod, NewID: newID,
	})
	require.NoError(t, err)

	access, err := rewardaccess.NewService(service, rewardpostgres.NewResolver(db), nil)
	require.NoError(t, err)
	worker, err := rewardworker.NewWorker(service, store, rewardworker.Options{})
	require.NoError(t, err)

	claims := publictaskpostgres.NewRepository(db,
		publictaskpostgres.WithClaimParticipants(
			rewardpostgres.NewClaimParticipant(rewardpostgres.ClaimParticipantOptions{NewID: newID}),
		),
	)

	handler := rest.NewServer(nil, rewardTokenVerifier{}, rest.WithRewardService(access)).Router()

	return &RewardEnv{
		T: t, DB: db, Service: service, Access: access, Claims: claims,
		Worker: worker, Provider: provider, REST: handler,
	}
}

// rewardTokenVerifier 把测试用 token 映射为主体。奖励验收只需要三种角色。
type rewardTokenVerifier struct{}

func (rewardTokenVerifier) Verify(_ context.Context, token string) (auth.Principal, error) {
	switch token {
	case "token-agent-1":
		return auth.Principal{
			Type: auth.PrincipalTypeAgent, TenantID: RewardSponsorTenant,
			AgentID: "agent-1", AgentVersionID: "agent-1-v1",
		}, nil
	case "token-agent-2":
		return auth.Principal{
			Type: auth.PrincipalTypeAgent, TenantID: RewardSponsorTenant,
			AgentID: "agent-2", AgentVersionID: "agent-2-v1",
		}, nil
	default:
		return auth.Principal{}, fmt.Errorf("unknown token")
	}
}

// SponsorPrincipal 是 sponsor 侧的人类主体。
func SponsorPrincipal() auth.Principal {
	return auth.Principal{Type: auth.PrincipalTypeHuman, TenantID: RewardSponsorTenant, OwnerID: "owner-1"}
}

// AgentPrincipal 是 Agent 主体；versionID 为空时默认取 <agentID>-v1。
func AgentPrincipal(agentID, versionID string) auth.Principal {
	if versionID == "" {
		versionID = agentID + "-v1"
	}
	return auth.Principal{
		Type: auth.PrincipalTypeAgent, TenantID: RewardSponsorTenant,
		AgentID: agentID, AgentVersionID: versionID,
	}
}

// SeedAgent 建立一个 Agent 身份及其首个版本。
func (env *RewardEnv) SeedAgent(agentID string) {
	env.T.Helper()
	ctx := context.Background()
	_, err := env.DB.Exec(ctx, `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES ($1,$1,$1,'active')`, agentID)
	require.NoError(env.T, err)
	env.SeedAgentVersion(agentID, agentID+"-v1", 1)
}

// SeedAgentVersion 追加一个新的 Agent 版本。奖励归属冻结在 Claim 时刻的版本上，
// 新建版本不会改写历史锁。
func (env *RewardEnv) SeedAgentVersion(agentID, versionID string, number int) {
	env.T.Helper()
	_, err := env.DB.Exec(context.Background(), `
		INSERT INTO agent_identity_versions (
			id, agent_id, version_number, status, runtime, model
		) VALUES ($1,$2,$3,'active','pi','gpt-5')`, versionID, agentID, number)
	require.NoError(env.T, err)
}

// SeedTask 建立一个已发布的公共任务：一条必需标准 c1、一条可选标准 c2。
func (env *RewardEnv) SeedTask(taskID, publicTaskID string) {
	env.T.Helper()
	ctx := context.Background()
	_, err := env.DB.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			deadline, status
		) VALUES ($1,$2,'publisher-version','bug','Fix widget','Widget fails',
			clock_timestamp() + interval '1 day','open')`, RewardSponsorTenant, taskID)
	require.NoError(env.T, err)
	sum := sha256.Sum256([]byte(taskID))
	_, err = env.DB.Exec(ctx, `
		INSERT INTO public_task_projections (
			id, resource_tenant_id, task_id, task_specification_version_id,
			canonical_repository, source_issue_url, issue_revision, base_commit,
			title, summary, problem_diagnosis, impact, proposed_solution,
			acceptance_criteria, quality_level, difficulty_class, spec_hash,
			status, published_at
		) VALUES ($1,$2,$3,'spec-1','octo/widget','https://example.test/'||$3,'rev-1',
			'0123456789abcdef0123456789abcdef01234567','Fix widget','Summary',
			'Diagnosis','Impact','Solution',
			'[{"id":"c1","statement":"tests pass","critical":true,"verifier_kind":"command","expected_result":"green"},
			  {"id":"c2","statement":"docs updated","critical":false,"verifier_kind":"manual","expected_result":"ok"}]'::jsonb,
			'standard','standard',$4,'published',clock_timestamp())`,
		publicTaskID, RewardSponsorTenant, taskID, hex.EncodeToString(sum[:]))
	require.NoError(env.T, err)
}

// TopUp 为 sponsor 托管账户充值。相同 requestID 的重复投递不会二次加钱。
func (env *RewardEnv) TopUp(requestID string, amountMinor int64) rewardapp.EscrowView {
	env.T.Helper()
	result, err := env.Service.TopUpEscrow(context.Background(), SponsorPrincipal(), rewardapp.TopUpEscrow{
		RequestID: requestID, Currency: "USDC", AmountMinor: amountMinor,
	})
	require.NoError(env.T, err)
	return result.Data
}

// Escrow 读取当前托管余额。
func (env *RewardEnv) Escrow() rewardapp.EscrowView {
	env.T.Helper()
	result, err := env.Service.GetEscrow(context.Background(), SponsorPrincipal(), "USDC")
	require.NoError(env.T, err)
	return result.Data
}

// FundedPolicy 创建并出资一条奖励契约：c1 占 60%、c2 占 40%。
func (env *RewardEnv) FundedPolicy(taskID string, gross int64) rewardapp.PolicyView {
	env.T.Helper()
	ctx := context.Background()
	created, err := env.Service.CreateRewardPolicy(ctx, SponsorPrincipal(), rewardapp.CreateRewardPolicy{
		TaskID: taskID, Currency: "USDC", GrossAmountMinor: gross,
		CriterionWeights: []rewarddomain.CriterionWeight{
			{CriterionID: "c1", WeightBps: 6000},
			{CriterionID: "c2", WeightBps: 4000},
		},
		MaintainerShareBps: 1000, ReviewerPoolShareBps: 1000,
		PlatformFeeBps: 500, DisputeReserveBps: 500,
	})
	require.NoError(env.T, err)
	funded, err := env.Service.FundRewardPolicy(ctx, SponsorPrincipal(), rewardapp.FundRewardPolicy{
		PolicyID: created.Data.ID,
	})
	require.NoError(env.T, err)
	return funded.Data
}

// Claim 以指定 Agent 版本领取公共任务，返回 execution id。
func (env *RewardEnv) Claim(publicTaskID, agentID, versionID, executionID string) string {
	env.T.Helper()
	if versionID == "" {
		versionID = agentID + "-v1"
	}
	suffix := strconv.FormatInt(env.seq.Add(1), 10)
	_, err := env.Claims.Claim(context.Background(), publictaskapp.PublicClaimRequest{
		PublicTaskID: publicTaskID, AgentID: agentID, AgentVersionID: versionID,
		RequestID: "claim-" + suffix, RequestHash: sha256.Sum256([]byte(executionID)),
		ExecutionID: executionID, GrantID: "grant-" + suffix, OutboxEventID: "outbox-" + suffix,
		GrantTTL: time.Hour,
		GrantScopes: []participationdomain.Scope{
			participationdomain.ScopeTaskRead, participationdomain.ScopeExecutionRead,
		},
	})
	require.NoError(env.T, err)
	return executionID
}

// RecordCriterion 向 Stage 1 的 criterion 账本追加一条验证结果。
//
// sourceID 是幂等键的一部分：同一条证据重复投递不会产生第二条结果。
func (env *RewardEnv) RecordCriterion(taskID, executionID, criterionID, sourceID string, critical, passed bool) {
	env.T.Helper()
	_, err := env.DB.Exec(context.Background(), `
		INSERT INTO execution_criterion_results (
			resource_tenant_id, task_id, execution_id, criterion_id, critical,
			verifier_kind, passed, source_kind, source_id, verified_by, observed_at
		) VALUES ($1,$2,$3,$4,$5,'command',$6,'validation_job',$7,'validator',
			clock_timestamp())
		ON CONFLICT DO NOTHING`,
		RewardSponsorTenant, taskID, executionID, criterionID, critical, passed, sourceID)
	require.NoError(env.T, err)
}

// BindDestination 走完 challenge + verify 两步绑定收款地址。
func (env *RewardEnv) BindDestination(principal auth.Principal, chain, address string) rewardapp.DestinationView {
	env.T.Helper()
	ctx := context.Background()
	challenge, err := env.Service.ChallengePayoutDestination(ctx, principal,
		rewardapp.ChallengePayoutDestination{Chain: chain, Address: address})
	require.NoError(env.T, err)
	verified, err := env.Service.VerifyPayoutDestination(ctx, principal, rewardapp.VerifyPayoutDestination{
		Nonce: challenge.Data.Nonce, Chain: chain, Address: address,
	})
	require.NoError(env.T, err)
	return verified.Data
}

// Decide 为某次执行生成奖励决策。
func (env *RewardEnv) Decide(executionID string) rewardapp.DecisionView {
	env.T.Helper()
	result, err := env.Service.Decide(context.Background(), rewardapp.DecideReward{
		ResourceTenantID: RewardSponsorTenant, ExecutionID: executionID,
	})
	require.NoError(env.T, err)
	return result.Data
}

// Release 经结算提供方释放奖励。
func (env *RewardEnv) Release(lockID string) rewardapp.LockView {
	env.T.Helper()
	result, err := env.Service.Release(context.Background(), rewardapp.SettleReward{
		ResourceTenantID: RewardSponsorTenant, LockID: lockID,
	})
	require.NoError(env.T, err)
	return result.Data
}

// Lock 读取某次执行的锁定状态。
func (env *RewardEnv) Lock(executionID string) rewardapp.LockView {
	env.T.Helper()
	result, err := env.Service.GetExecutionReward(context.Background(), RewardSponsorTenant, executionID)
	require.NoError(env.T, err)
	return result.Data
}

// Receipts 返回某条决策的全部回执。
func (env *RewardEnv) Receipts(decisionHash string) []rewardapp.ReceiptView {
	env.T.Helper()
	receipts, err := env.Service.ListReceipts(context.Background(), decisionHash)
	require.NoError(env.T, err)
	return receipts
}

// ProviderStatus 直接问结算提供方要回执，用来区分"账本以为付了"和"真的付了"。
func (env *RewardEnv) ProviderStatus(decisionHash string) (settlement.Receipt, error) {
	return env.Provider.Status(context.Background(), decisionHash)
}

// ExpireLockDeadline 把锁的过期时刻挪到过去，供 worker 回收。
func (env *RewardEnv) ExpireLockDeadline(lockID string) {
	env.T.Helper()
	_, err := env.DB.Exec(context.Background(), `
		UPDATE reward_locks SET expires_at = clock_timestamp() - interval '1 minute'
		WHERE resource_tenant_id=$1 AND id=$2`, RewardSponsorTenant, lockID)
	require.NoError(env.T, err)
}

// RunWorker 跑一轮奖励 worker（决策 → 释放 → 过期回收）。
func (env *RewardEnv) RunWorker() {
	env.T.Helper()
	require.NoError(env.T, env.Worker.RunOnce(context.Background()))
}

// GetJSON 以匿名或指定 token 打一次 REST GET，返回状态码与原始响应体。
func (env *RewardEnv) GetJSON(path, token string) (int, string) {
	env.T.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	env.REST.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

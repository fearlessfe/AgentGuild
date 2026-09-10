package rest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	"agentguild.dev/agentguild/backend/internal/rewardaccess"
	settlementfake "agentguild.dev/agentguild/backend/internal/settlement/fake"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// 这些串被刻意种进数据库，用来证明匿名响应里没有它们。
// 它们涵盖三类必须保密的信息：sponsor 租户、赞助方组织与 sponsor 账户标识。
const (
	leakTenantID     = "tenant-secret-sponsor-corp"
	leakOrganization = "SecretSponsorCorporation"
	leakSponsorOwner = "sponsor-account-9f13c7"
	leakIssueURL     = "https://example.test/issues/1"
)

// leakFixture 是一次完整的奖励闭环，用于覆盖全部匿名端点。
type leakFixture struct {
	db           *pgxpool.Pool
	server       http.Handler
	publicTaskID string
	decisionHash string
}

func newLeakFixture(t *testing.T) *leakFixture {
	t.Helper()
	ctx := context.Background()
	db := testdb.StartPostgres(t)

	var counter atomic.Int64
	newID := func() string { return "leak-" + strconv.FormatInt(counter.Add(1), 10) }

	signer, err := rewarddomain.NewHMACSigner([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	rewardService, err := rewardapp.NewService(rewardapp.Options{
		Store: rewardpostgres.NewStore(db), Provider: settlementfake.New(settlementfake.Options{}),
		Evidence: rewardpostgres.NewEvidenceSource(db), Signer: signer,
		AlgorithmVersion: rewarddomain.DefaultAlgorithmVersion, NewID: newID,
	})
	require.NoError(t, err)
	rewardAccess, err := rewardaccess.NewService(rewardService, rewardpostgres.NewResolver(db), nil)
	require.NoError(t, err)

	claims := publictaskpostgres.NewRepository(db,
		publictaskpostgres.WithClaimParticipants(
			rewardpostgres.NewClaimParticipant(rewardpostgres.ClaimParticipantOptions{NewID: newID}),
		),
	)
	publicTasks, err := publictaskapp.NewService(claims, publictaskapp.Options{
		CursorSecret: []byte("0123456789abcdef0123456789abcdef"),
	})
	require.NoError(t, err)

	seedLeakAgent(t, db)
	seedLeakTask(t, db)

	sponsor := auth.Principal{Type: auth.PrincipalTypeHuman, TenantID: leakTenantID, OwnerID: leakSponsorOwner}
	_, err = rewardService.TopUpEscrow(ctx, sponsor, rewardapp.TopUpEscrow{
		RequestID: "topup-1", Currency: "USDC", AmountMinor: 100_000,
	})
	require.NoError(t, err)
	created, err := rewardService.CreateRewardPolicy(ctx, sponsor, rewardapp.CreateRewardPolicy{
		TaskID: "leak-task", Currency: "USDC", GrossAmountMinor: 100_000,
		CriterionWeights: []rewarddomain.CriterionWeight{{CriterionID: "c1", WeightBps: 10_000}},
		PlatformFeeBps:   500, DisputeReserveBps: 500,
	})
	require.NoError(t, err)
	_, err = rewardService.FundRewardPolicy(ctx, sponsor, rewardapp.FundRewardPolicy{PolicyID: created.Data.ID})
	require.NoError(t, err)

	_, err = claims.Claim(ctx, publictaskapp.PublicClaimRequest{
		PublicTaskID: "leak-public", AgentID: "leak-agent", AgentVersionID: "leak-agent-v",
		RequestID: "claim-1", RequestHash: sha256.Sum256([]byte("claim-1")),
		ExecutionID: "leak-execution", GrantID: "leak-grant", OutboxEventID: "leak-outbox",
		GrantTTL: time.Hour, GrantScopes: []participationdomain.Scope{participationdomain.ScopeTaskRead},
	})
	require.NoError(t, err)
	recordLeakCriterion(t, db)
	// 通过全部必需标准的路径需要一个已验证的收款目的地，否则决策无处可付。
	agent := auth.Principal{Type: auth.PrincipalTypeAgent, AgentID: "leak-agent", AgentVersionID: "leak-agent-v"}
	challenge, err := rewardService.ChallengePayoutDestination(ctx, agent,
		rewardapp.ChallengePayoutDestination{Chain: "base", Address: "0xleak"})
	require.NoError(t, err)
	_, err = rewardService.VerifyPayoutDestination(ctx, agent, rewardapp.VerifyPayoutDestination{
		Nonce: challenge.Data.Nonce, Chain: "base", Address: "0xleak",
	})
	require.NoError(t, err)
	decision, err := rewardService.Decide(ctx, rewardapp.DecideReward{
		ResourceTenantID: leakTenantID, ExecutionID: "leak-execution",
	})
	require.NoError(t, err)

	server := newTestServer(&fakeApplication{},
		rest.WithPublicTaskService(publicTasks),
		rest.WithRewardService(rewardAccess),
	)
	return &leakFixture{db: db, server: server, publicTaskID: "leak-public", decisionHash: decision.Data.DecisionHash}
}

func seedLeakAgent(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO agent_identities (id, handle, display_name, status)
		VALUES ('leak-agent','leak-agent','leak-agent','active')`)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `
		INSERT INTO agent_identity_versions (id, agent_id, version_number, status, runtime, model)
		VALUES ('leak-agent-v','leak-agent',1,'active','pi','gpt-5')`)
	require.NoError(t, err)
}

func seedLeakTask(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			deadline, status
		) VALUES ($1,'leak-task','publisher-version','bug',$2,$3,
			clock_timestamp() + interval '1 day','open')`,
		leakTenantID, "Fix widget for "+leakOrganization, "Reported by "+leakSponsorOwner)
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("leak-task"))
	_, err = db.Exec(ctx, `
		INSERT INTO public_task_projections (
			id, resource_tenant_id, task_id, task_specification_version_id,
			canonical_repository, source_issue_url, issue_revision, base_commit,
			title, summary, problem_diagnosis, impact, proposed_solution,
			acceptance_criteria, quality_level, difficulty_class, spec_hash,
			status, published_at
		) VALUES ('leak-public',$1,'leak-task','spec-1','octo/widget',$2,'rev-1',
			'0123456789abcdef0123456789abcdef01234567','Fix widget','Summary',
			'Diagnosis','Impact','Solution',
			'[{"id":"c1","statement":"tests pass","critical":true,"verifier_kind":"command","expected_result":"green"}]'::jsonb,
			'standard','standard',$3,'published',clock_timestamp())`,
		leakTenantID, leakIssueURL, hex.EncodeToString(sum[:]))
	require.NoError(t, err)
}

func recordLeakCriterion(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO execution_criterion_results (
			resource_tenant_id, task_id, execution_id, criterion_id, critical,
			verifier_kind, passed, source_kind, source_id, verified_by, observed_at
		) VALUES ($1,'leak-task','leak-execution','c1',true,'command',true,
			'validation_job','job-1','validator',clock_timestamp())`, leakTenantID)
	require.NoError(t, err)
}

// TestAnonymousEndpointsNeverLeakSponsorTenant 是表驱动的泄露回归。
//
// 它断言的是原始响应字节，而不是解析后的结构：一旦有人给某个 view 加回
// resource_tenant_id 之类的字段，这个测试立刻失败，不需要有人记得去更新断言。
func TestAnonymousEndpointsNeverLeakSponsorTenant(t *testing.T) {
	fixture := newLeakFixture(t)
	poison := []string{leakTenantID, leakOrganization, leakSponsorOwner, "resource_tenant", "tenant_id"}

	cases := []struct {
		name string
		path string
	}{
		{"public_task_list", "/v1/public/tasks?limit=10"},
		{"public_task_detail", "/v1/public/tasks/" + fixture.publicTaskID},
		{"public_task_reward", "/v1/public/tasks/" + fixture.publicTaskID + "/reward"},
		{"reward_decision", "/v1/rewards/decisions/" + fixture.decisionHash},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// 刻意不带任何 Authorization：这些端点必须匿名可读。
			req := httptest.NewRequest(http.MethodGet, testCase.path, nil)
			rec := httptest.NewRecorder()
			fixture.server.ServeHTTP(rec, req)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			body := rec.Body.String()
			require.NotEmpty(t, body)
			for _, needle := range poison {
				require.NotContains(t, body, needle,
					"匿名响应泄露了 sponsor 侧标识：%s", needle)
			}
		})
	}
}

// TestAnonymousRewardDecisionCarriesNoPrivateEvidence 对应文档 §10：
// 决策必须可被第三方验证，但不得泄露私有 Issue、代码或评审内容。
func TestAnonymousRewardDecisionCarriesNoPrivateEvidence(t *testing.T) {
	fixture := newLeakFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/rewards/decisions/"+fixture.decisionHash, nil)
	rec := httptest.NewRecorder()
	fixture.server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	// 可验证：摘要与签名都在。
	for _, field := range []string{"decision_hash", "task_spec_hash", "contribution_hash", "signature", "algorithm_version"} {
		require.Contains(t, body, field)
	}
	// 不可泄露：Issue 链接与分析正文都不出现在决策里。
	// 这里断言的是种进数据库的具体串，而不是 "review" 这类会误伤
	// reviewer_pool_amount_minor 字段名的泛化关键词。
	for _, needle := range []string{leakIssueURL, "Diagnosis", "Solution", "Fix widget", leakOrganization} {
		require.False(t, strings.Contains(body, needle),
			"决策响应泄露了私有证据：%s", needle)
	}
}

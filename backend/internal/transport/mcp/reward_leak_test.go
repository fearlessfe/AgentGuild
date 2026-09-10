package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"agentguild.dev/agentguild/backend/internal/auth"
	rewardapp "agentguild.dev/agentguild/backend/internal/reward/application"
	rewarddomain "agentguild.dev/agentguild/backend/internal/reward/domain"
	rewardpostgres "agentguild.dev/agentguild/backend/internal/reward/postgres"
	"agentguild.dev/agentguild/backend/internal/rewardaccess"
	settlementfake "agentguild.dev/agentguild/backend/internal/settlement/fake"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// 与 REST 侧同一套毒串：sponsor 租户、赞助方组织与 sponsor 账户标识。
const (
	mcpLeakTenantID     = "tenant-secret-sponsor-corp"
	mcpLeakOrganization = "SecretSponsorCorporation"
	mcpLeakSponsorOwner = "sponsor-account-9f13c7"
)

func seedMCPLeakTask(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `
		INSERT INTO tasks (
			tenant_id, id, publisher_agent_version_id, type, title, problem,
			deadline, status
		) VALUES ($1,'leak-task','publisher-version','bug',$2,$3,
			clock_timestamp() + interval '1 day','open')`,
		mcpLeakTenantID, "Fix widget for "+mcpLeakOrganization, "Reported by "+mcpLeakSponsorOwner)
	require.NoError(t, err)
	sum := sha256.Sum256([]byte("leak-task"))
	_, err = db.Exec(ctx, `
		INSERT INTO public_task_projections (
			id, resource_tenant_id, task_id, task_specification_version_id,
			canonical_repository, source_issue_url, issue_revision, base_commit,
			title, summary, problem_diagnosis, impact, proposed_solution,
			acceptance_criteria, quality_level, difficulty_class, spec_hash,
			status, published_at
		) VALUES ('leak-public',$1,'leak-task','spec-1','octo/widget',
			'https://example.test/issues/1','rev-1',
			'0123456789abcdef0123456789abcdef01234567','Fix widget','Summary',
			'Diagnosis','Impact','Solution',
			'[{"id":"c1","statement":"tests pass","critical":true,"verifier_kind":"command","expected_result":"green"}]'::jsonb,
			'standard','standard',$2,'published',clock_timestamp())`,
		mcpLeakTenantID, hex.EncodeToString(sum[:]))
	require.NoError(t, err)
}

// newRewardLeakServer 组装一个真实数据库支撑的 MCP server，调用方刻意来自
// 另一个租户：跨租户 Agent 是最可能被泄露伤到的角色。
func newRewardLeakServer(t *testing.T) (*mcp.Server, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	seedMCPLeakTask(t, db)

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

	sponsor := auth.Principal{Type: auth.PrincipalTypeHuman, TenantID: mcpLeakTenantID, OwnerID: mcpLeakSponsorOwner}
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

	access, err := rewardaccess.NewService(rewardService, rewardpostgres.NewResolver(db), nil)
	require.NoError(t, err)

	// 调用方属于另一个租户，且不持有该 Execution 的任何 grant。
	principal := auth.Principal{
		Type: auth.PrincipalTypeAgent, TenantID: "tenant-other",
		AgentID: "other-agent", AgentVersionID: "other-agent-v",
	}
	verifier := &fakeVerifier{principal: principal}
	server := NewServer(&fakeApplication{}, verifier, WithRewardService(access))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
	return server.mcpServer(req), db
}

func callRewardTool(t *testing.T, server *mcp.Server, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return result
}

// TestMCPRewardGetNeverLeaksSponsorTenant 是 MCP 侧的泄露回归，
// 与 REST 的 public_leak_test.go 覆盖同一批毒串。
func TestMCPRewardGetNeverLeaksSponsorTenant(t *testing.T) {
	server, _ := newRewardLeakServer(t)

	result := callRewardTool(t, server, "reward_get", map[string]any{"public_task_id": "leak-public"})
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	body := result.Content[0].(*mcp.TextContent).Text

	require.Contains(t, body, "policy_hash")
	for _, needle := range []string{mcpLeakTenantID, mcpLeakOrganization, mcpLeakSponsorOwner, "resource_tenant", "tenant_id"} {
		require.NotContains(t, body, needle, "MCP 奖励响应泄露了 sponsor 侧标识：%s", needle)
	}
}

// TestMCPRewardGetRejectsAmbiguousSelector 证明两个互斥选择器不会被同时接受：
// 它们背后是两套不同的授权模型，含糊的调用必须直接失败。
func TestMCPRewardGetRejectsAmbiguousSelector(t *testing.T) {
	server, _ := newRewardLeakServer(t)

	for _, args := range []map[string]any{
		{},
		{"public_task_id": "leak-public", "execution_id": "leak-execution"},
	} {
		result := callRewardTool(t, server, "reward_get", args)
		require.True(t, result.IsError)
		require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "INVALID_ARGUMENT")
	}
}

// TestMCPRewardGetDeniesForeignExecution 证明另一个租户的 Agent 拿不到
// 别人的执行级奖励，且错误响应本身也不泄露资源是否存在。
func TestMCPRewardGetDeniesForeignExecution(t *testing.T) {
	server, _ := newRewardLeakServer(t)

	result := callRewardTool(t, server, "reward_get", map[string]any{"execution_id": "leak-execution"})
	require.True(t, result.IsError)
	body := result.Content[0].(*mcp.TextContent).Text
	require.Contains(t, body, "NOT_FOUND")
	require.NotContains(t, body, mcpLeakTenantID)
}

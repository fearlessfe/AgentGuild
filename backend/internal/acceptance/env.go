package acceptance

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	identitypostgres "agentguild.dev/agentguild/backend/internal/identity/postgres"
	"agentguild.dev/agentguild/backend/internal/postgres"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	reputationpostgres "agentguild.dev/agentguild/backend/internal/reputation/postgres"
	reputationworker "agentguild.dev/agentguild/backend/internal/reputation/worker"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
	reviewpostgres "agentguild.dev/agentguild/backend/internal/review/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	transportmcp "agentguild.dev/agentguild/backend/internal/transport/mcp"
	"agentguild.dev/agentguild/backend/internal/transport/rest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// Env 是端到端验收测试的共享 harness，包含真实 PostgreSQL、应用服务与两个 transport。
type Env struct {
	T          *testing.T
	DB         *pgxpool.Pool
	Service    *application.Service
	ReviewSvc  *reviewapp.Service
	Validation *acceptanceValidationProvider
	Worker     *reputationworker.Worker
	MCP        *MCPClient
	REST       *RESTClient
	Identity   *IdentityClient
	publishSeq int64
}

// Start 启动一个隔离的验收环境。
func Start(t *testing.T) *Env {
	t.Helper()

	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	identityStore := identitypostgres.NewStore(db)
	svc, err := application.NewService(store, application.Options{
		CursorSecret: []byte("0123456789abcdef0123456789abcdef"),
		CursorTTL:    15 * time.Minute,
	})
	require.NoError(t, err)

	validation := &acceptanceValidationProvider{pass: true}
	reviewSvc, err := reviewapp.NewService(store, acceptanceDiffProvider{}, validation, reviewapp.Options{})
	require.NoError(t, err)
	worker := reputationworker.NewWorker(store, time.Hour, 100, slog.Default())

	verifier, tokenIssuer := newAcceptanceIdentityRuntime(t)
	identitySvc, err := identityapp.NewIdentityService(identityStore, identityapp.IdentityOptions{
		NewID:       acceptanceSequenceIDs("agent-1", "version-1", "agent-2", "version-2", "agent-3", "version-3"),
		TokenIssuer: tokenIssuer,
	})
	require.NoError(t, err)
	seedFakeLifecycleAgents(t, db)
	seedAcceptanceRubric(t, db)
	seedAcceptanceReviewer(t, db)

	reputationSvc := application.NewReputationQueryService(store)

	mcpHandler := transportmcp.NewServer(svc, verifier,
		transportmcp.WithReviewService(reviewSvc),
		transportmcp.WithReputationService(reputationSvc),
	).Handler()
	restHandler := rest.NewServer(svc, verifier,
		rest.WithIdentityService(identitySvc),
		rest.WithReviewService(reviewSvc),
		rest.WithRubricService(reviewSvc),
		rest.WithReputationService(reputationSvc),
		rest.WithSession("acceptance-session-secret-0123456789abcdef", false),
	).Router()

	return &Env{
		T:          t,
		DB:         db,
		Service:    svc,
		ReviewSvc:  reviewSvc,
		Validation: validation,
		Worker:     worker,
		MCP:        &MCPClient{t: t, handler: mcpHandler, db: db, token: "token-agent"},
		REST:       &RESTClient{t: t, handler: restHandler, db: db, token: "token-agent"},
		Identity: &IdentityClient{
			t:             t,
			handler:       restHandler,
			sessionSecret: "acceptance-session-secret-0123456789abcdef",
		},
	}
}

// PublishTask 以发布者身份创建一个新任务，返回 task id。
func (env *Env) PublishTask(deadline time.Time) string {
	env.T.Helper()
	reqID := fmt.Sprintf("pub-%d", atomic.AddInt64(&env.publishSeq, 1))
	result, err := env.Service.PublishTask(context.Background(), publisherPrincipal(), application.PublishTask{
		RequestID:    reqID,
		Type:         "code",
		Title:        "Acceptance task",
		Problem:      "Verify lifecycle",
		Constraints:  []string{"fast"},
		Requirements: []string{"pass"},
		Deadline:     deadline,
	})
	require.NoError(env.T, err)
	return result.Data.ID
}

// CountAuditIntent 返回审计事件中指定 intent 的数量。
func (env *Env) CountAuditIntent(intent string) int {
	env.T.Helper()
	var n int
	err := env.DB.QueryRow(context.Background(), `SELECT count(*) FROM task_events WHERE intent=$1`, intent).Scan(&n)
	require.NoError(env.T, err)
	return n
}

// SetLeaseOffsets 用相对当前数据库 clock_timestamp() 的偏移量调整执行 lease 时间。
// 负偏移表示过去，用于模拟宽限期或硬过期。
func (env *Env) SetLeaseOffsets(executionID string, softOffset, hardOffset time.Duration) {
	env.T.Helper()
	_, err := env.DB.Exec(context.Background(), `
		UPDATE executions
		SET lease_soft_expires_at = clock_timestamp() + $1::interval,
		    lease_hard_expires_at = clock_timestamp() + $2::interval
		WHERE id=$3`, softOffset, hardOffset, executionID)
	require.NoError(env.T, err)
}

// RunReaper 运行一次过期回收调度器。
func (env *Env) RunReaper() int {
	env.T.Helper()
	reaper := postgres.NewReaper(env.DB)
	n, err := reaper.RunBatch(context.Background(), 100)
	require.NoError(env.T, err)
	return n
}

// SubmitForReview 将运行中的执行推进到 reviewing 状态。
// 当前为临时入口，待 git-delivery-and-validation 模块合入后替换。
func (env *Env) SubmitForReview(executionID string) {
	env.T.Helper()
	ctx := context.Background()
	var agentVersionID string
	err := env.DB.QueryRow(ctx, `SELECT agent_version_id FROM executions WHERE tenant_id=$1 AND id=$2`, "tenant-1", executionID).Scan(&agentVersionID)
	require.NoError(env.T, err)
	token := "token-agent"
	var i int
	if _, err := fmt.Sscanf(agentVersionID, "acceptance-agent-%d", &i); err == nil && i > 1 {
		token = fmt.Sprintf("token-agent-%d", i)
	}
	res := env.REST.As(token).SubmitForReview(executionID, executionID+"-submit-for-review")
	require.Empty(env.T, res.Code, "submit for review failed: %s", res.Code)
}

// CreateResubmissionExecution 模拟 revision requested 后 agent 重新提交产生的新 execution。
// 当前 git-delivery-and-validation 模块尚未实现，因此直接操作数据库生成新 execution。
func (env *Env) CreateResubmissionExecution(taskID, oldExecutionID, newExecutionID, agentVersionID string) string {
	env.T.Helper()
	ctx := context.Background()

	_, err := env.DB.Exec(ctx, `
		UPDATE executions
		SET status = 'expired', expired_at = clock_timestamp(), updated_at = clock_timestamp()
		WHERE tenant_id = $1 AND id = $2`, "tenant-1", oldExecutionID)
	require.NoError(env.T, err)

	_, err = env.DB.Exec(ctx, `
		UPDATE tasks
		SET active_execution_id = NULL, updated_at = clock_timestamp()
		WHERE tenant_id = $1 AND id = $2`, "tenant-1", taskID)
	require.NoError(env.T, err)

	secret := make([]byte, 32)
	_, err = rand.Read(secret)
	require.NoError(env.T, err)
	hash := sha256.Sum256(secret)
	_, err = env.DB.Exec(ctx, `
		INSERT INTO executions (
			tenant_id, id, task_id, agent_version_id, status, state_version,
			lease_secret_hash, lease_generation, claimed_at, started_at, submitted_at,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, 'reviewing', 0, $5, 1,
			clock_timestamp(), clock_timestamp(), clock_timestamp(),
			clock_timestamp(), clock_timestamp())`,
		"tenant-1", newExecutionID, taskID, agentVersionID, hash[:])
	require.NoError(env.T, err)

	_, err = env.DB.Exec(ctx, `
		UPDATE tasks
		SET active_execution_id = $1, updated_at = clock_timestamp()
		WHERE tenant_id = $2 AND id = $3`, newExecutionID, "tenant-1", taskID)
	require.NoError(env.T, err)
	return newExecutionID
}

// GetExecutionStatus 直接读取 execution 状态。
func (env *Env) GetExecutionStatus(tenantID, executionID string) domain.ExecutionStatus {
	env.T.Helper()
	var status string
	err := env.DB.QueryRow(context.Background(), `
		SELECT status FROM executions WHERE tenant_id=$1 AND id=$2`, tenantID, executionID).Scan(&status)
	require.NoError(env.T, err)
	return domain.ExecutionStatus(status)
}

// CreateReviewViaREST 以发布者身份为指定 submission 创建 review。
func (env *Env) CreateReviewViaREST(executionID string, capabilities []string, requestID string) reviewapp.ReviewView {
	env.T.Helper()
	res := env.REST.As("token-publisher").CreateReview(executionID, capabilities, requestID)
	require.Empty(env.T, res.Code, "create review failed: %s", res.Code)
	return res.Review
}

// SubmitDecisionViaREST 以指定 reviewer token 提交 review 决策。
func (env *Env) SubmitDecisionViaREST(reviewID, decision string, scores []reviewdomain.RubricScore, summary, token, requestID string) reviewapp.ReviewView {
	env.T.Helper()
	res := env.REST.As(token).SubmitDecision(reviewID, decision, scores, summary, requestID)
	require.Empty(env.T, res.Code, "submit decision failed: %s", res.Code)
	return res.Review
}

// SubmitDecisionViaMCP 以指定 reviewer token 通过 MCP 提交 review 决策。
func (env *Env) SubmitDecisionViaMCP(reviewID, decision string, scores []reviewdomain.RubricScore, summary, token, requestID string) reviewapp.ReviewView {
	env.T.Helper()
	res := env.MCP.As(token).ReviewSubmit(reviewID, decision, scores, summary, requestID)
	require.Empty(env.T, res.Code, "submit decision via MCP failed: %s", res.Code)
	return res.Review
}

// SubmitDecisionCodeViaMCP 返回 MCP 提交决策的错误码，空字符串表示成功。
func (env *Env) SubmitDecisionCodeViaMCP(reviewID, decision string, scores []reviewdomain.RubricScore, summary, token, requestID string) string {
	env.T.Helper()
	return env.MCP.As(token).ReviewSubmit(reviewID, decision, scores, summary, requestID).Code
}

// AddCommentViaREST 为 review 添加行级注释。
func (env *Env) AddCommentViaREST(reviewID, submissionID, text, token, requestID string) reviewapp.CommentView {
	env.T.Helper()
	res := env.REST.As(token).AddComment(reviewID, submissionID, text, requestID)
	require.Empty(env.T, res.Code, "add comment failed: %s", res.Code)
	return res.Comment
}

// ListComments 返回指定 review 下的所有行级注释。
func (env *Env) ListComments(tenantID, reviewID string) []reviewdomain.LineComment {
	env.T.Helper()
	repo := reviewpostgres.NewLineCommentRepository(env.DB)
	comments, err := repo.ListByReview(context.Background(), tenantID, reviewID)
	require.NoError(env.T, err)
	return comments
}

// WorkerTick 运行一次声望投影 worker。
func (env *Env) WorkerTick() {
	env.T.Helper()
	require.NoError(env.T, env.Worker.RunOnce(context.Background()))
}

// GetProjection 读取指定 key 的声望投影。
func (env *Env) GetProjection(tenantID, agentVersionID, capability, taskType string) *reputationdomain.Projection {
	env.T.Helper()
	repo := reputationpostgres.NewProjectionRepository(env.DB)
	proj, err := repo.GetByKey(context.Background(), tenantID, reputationdomain.ProjectionKey{
		AgentVersionID: agentVersionID,
		Capability:     capability,
		TaskType:       taskType,
	})
	require.NoError(env.T, err)
	return proj
}

// SetHardGatesPass 切换验收测试中的硬门禁验证结果。
func (env *Env) SetHardGatesPass(pass bool) {
	env.T.Helper()
	env.Validation.pass = pass
}

// AsAgent 返回使用指定 agent token 的 MCP 客户端。
func (env *Env) AsAgent(token string) *MCPClient {
	return env.MCP.As(token)
}

// --- fake verifier ---

type fakeVerifier struct {
	fallback auth.TokenVerifier
}

func newAcceptanceIdentityRuntime(t *testing.T) (fakeVerifier, acceptanceFixedIssuer) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	issuer, err := auth.NewRS256TokenIssuer(key, auth.TokenIssuerConfig{
		Issuer:   "agentguild-acceptance",
		Audience: "agentguild-agents",
		KeyID:    "acceptance",
	})
	require.NoError(t, err)
	return fakeVerifier{
		fallback: auth.NewRS256Verifier(&key.PublicKey, auth.TokenVerifierConfig{
			Issuer:   "agentguild-acceptance",
			Audience: "agentguild-agents",
		}),
	}, acceptanceFixedIssuer{issuer: issuer}
}

func (v fakeVerifier) Verify(ctx context.Context, rawToken string) (auth.Principal, error) {
	switch {
	case rawToken == "token-publisher":
		return publisherPrincipal(), nil
	case rawToken == "token-agent":
		return agentPrincipal(1), nil
	case rawToken == "token-admin":
		return adminPrincipal(), nil
	case rawToken == "token-reviewer":
		return reviewerPrincipal(1), nil
	case strings.HasPrefix(rawToken, "token-agent-"):
		var i int
		if _, err := fmt.Sscanf(rawToken, "token-agent-%d", &i); err == nil {
			return agentPrincipal(i), nil
		}
	case strings.HasPrefix(rawToken, "token-reviewer-"):
		var i int
		if _, err := fmt.Sscanf(rawToken, "token-reviewer-%d", &i); err == nil {
			return reviewerPrincipal(i), nil
		}
	}
	if v.fallback != nil {
		return v.fallback.Verify(ctx, rawToken)
	}
	return auth.Principal{}, fmt.Errorf("unknown token")
}

func publisherPrincipal() auth.Principal {
	return auth.Principal{
		TenantID:       "tenant-1",
		Type:           auth.PrincipalTypeAgent,
		AgentID:        "publisher",
		AgentVersionID: "publisher-v1",
		Scopes:         []string{"tasks:publish", "tasks:read", "tasks:cancel", "reviews:read", "reviews:write", "tasks:execute"},
	}
}

func reviewerPrincipal(i int) auth.Principal {
	return auth.Principal{
		TenantID: "tenant-1",
		Type:     auth.PrincipalTypeHuman,
		OwnerID:  fmt.Sprintf("reviewer-user-%d", i),
		Scopes:   []string{"reviews:read", "reviews:write", "reputation:read"},
	}
}

func agentPrincipal(i int) auth.Principal {
	id := fmt.Sprintf("acceptance-agent-%d", i)
	return auth.Principal{
		TenantID:       "tenant-1",
		Type:           auth.PrincipalTypeAgent,
		AgentID:        id,
		AgentVersionID: id,
		Scopes:         []string{"tasks:claim", "tasks:execute"},
	}
}

func seedFakeLifecycleAgents(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	seedFakeLifecycleAgent(t, ctx, db, publisherPrincipal())
	for i := 0; i < 100; i++ {
		seedFakeLifecycleAgent(t, ctx, db, agentPrincipal(i))
	}
}

func seedFakeLifecycleAgent(t *testing.T, ctx context.Context, db *pgxpool.Pool, principal auth.Principal) {
	t.Helper()
	_, err := db.Exec(ctx, `
		WITH inserted_agent AS (
			INSERT INTO agents (
				id, tenant_id, owner_id, owner_email, name, status, scopes
			)
			VALUES ($1, $2, 'acceptance-owner', 'acceptance@example.test', $1, $3, $4)
		),
		inserted_version AS (
			INSERT INTO agent_versions (
				id, tenant_id, agent_id, version_number, runtime, model
			)
			VALUES ($5, $2, $1, 1, 'acceptance', 'fake-token')
		)
		UPDATE agents
		SET current_version_id=$5
		WHERE tenant_id=$2 AND id=$1`,
		principal.AgentID,
		principal.TenantID,
		identitydomain.AgentActive,
		principal.Scopes,
		principal.AgentVersionID,
	)
	require.NoError(t, err)
}

func adminPrincipal() auth.Principal {
	return auth.Principal{
		TenantID:       "tenant-1",
		AgentID:        "admin",
		AgentVersionID: "admin-v1",
		Scopes:         []string{"admin:tasks"},
	}
}

// --- MCP client ---

// MCPClient 通过 Streamable HTTP MCP 调用工具。
type MCPClient struct {
	t        *testing.T
	handler  http.Handler
	db       *pgxpool.Pool
	token    string
	lastMeta application.Meta
}

func (c *MCPClient) As(token string) *MCPClient {
	return &MCPClient{t: c.t, handler: c.handler, db: c.db, token: token}
}

// LastMeta 返回最近一次成功调用的 envelope meta。
func (c *MCPClient) LastMeta() application.Meta { return c.lastMeta }

// TaskClaim 领取任务并返回 execution 视图。
func (c *MCPClient) TaskClaim(taskID, requestID string) application.ExecutionView {
	c.t.Helper()
	res := c.call("task_claim", map[string]any{"request_id": requestID, "task_id": taskID})
	require.Empty(c.t, res.Code, "task_claim failed: %s", res.Code)
	return res.Execution
}

// PublishTaskCode 返回 publish 的错误码，空字符串表示成功。
func (c *MCPClient) PublishTaskCode(deadline time.Time, requestID string) string {
	c.t.Helper()
	return c.call("task_publish", map[string]any{
		"request_id":   requestID,
		"type":         "code",
		"title":        "Acceptance task",
		"problem":      "Verify identity status enforcement",
		"constraints":  []string{"fast"},
		"requirements": []string{"pass"},
		"deadline":     deadline,
	}).Code
}

// Heartbeat 续租并返回 execution 视图。
func (c *MCPClient) Heartbeat(executionID string, generation int64, requestID string) application.ExecutionView {
	c.t.Helper()
	res := c.call("execution_heartbeat", map[string]any{
		"request_id":       requestID,
		"execution_id":     executionID,
		"lease_generation": generation,
	})
	require.Empty(c.t, res.Code, "execution_heartbeat failed: %s", res.Code)
	return res.Execution
}

// HeartbeatDropResponse 模拟响应包丢失：请求已被服务器处理，但客户端未收到响应。
// 返回服务端实际存储的 generation，用于验证重试幂等恢复。
func (c *MCPClient) HeartbeatDropResponse(executionID string, generation int64, requestID string) *ExecutionSnapshot {
	c.t.Helper()
	rec := c.callRaw("execution_heartbeat", map[string]any{
		"request_id":       requestID,
		"execution_id":     executionID,
		"lease_generation": generation,
	})
	require.Equal(c.t, http.StatusOK, rec.Code, "dropped heartbeat request should be processed")

	var stored int64
	err := c.db.QueryRow(context.Background(),
		`SELECT lease_generation FROM executions WHERE id=$1`, executionID).Scan(&stored)
	require.NoError(c.t, err)
	return &ExecutionSnapshot{StoredGeneration: stored}
}

// ExecutionSnapshot 是响应丢失后直接从数据库读取的状态。
type ExecutionSnapshot struct {
	StoredGeneration int64
}

// HeartbeatCode 返回 heartbeat 的错误码，空字符串表示成功。
func (c *MCPClient) HeartbeatCode(executionID string, generation int64, requestID string) string {
	c.t.Helper()
	return c.call("execution_heartbeat", map[string]any{
		"request_id":       requestID,
		"execution_id":     executionID,
		"lease_generation": generation,
	}).Code
}

// StartExecution 开始执行。
func (c *MCPClient) StartExecution(executionID string, generation int64, requestID string) application.ExecutionView {
	c.t.Helper()
	res := c.call("execution_start", map[string]any{
		"request_id":       requestID,
		"execution_id":     executionID,
		"lease_generation": generation,
	})
	require.Empty(c.t, res.Code, "execution_start failed: %s", res.Code)
	return res.Execution
}

// StartExecutionCode 返回 start 的错误码。
func (c *MCPClient) StartExecutionCode(executionID string, generation int64, requestID string) string {
	c.t.Helper()
	return c.call("execution_start", map[string]any{
		"request_id":       requestID,
		"execution_id":     executionID,
		"lease_generation": generation,
	}).Code
}

// GetExecutionCode 返回 get execution 的错误码，空字符串表示成功。
func (c *MCPClient) GetExecutionCode(executionID string) string {
	c.t.Helper()
	return c.call("execution_get", map[string]any{"execution_id": executionID}).Code
}

// TaskClaimCode 返回 claim 的错误码。
func (c *MCPClient) TaskClaimCode(taskID, requestID string) string {
	c.t.Helper()
	return c.call("task_claim", map[string]any{"request_id": requestID, "task_id": taskID}).Code
}

// ReviewSubmit 通过 MCP 提交 review 决策。
func (c *MCPClient) ReviewSubmit(reviewID, decision string, scores []reviewdomain.RubricScore, summary, requestID string) mcpReviewResult {
	c.t.Helper()
	inputs := make([]map[string]any, len(scores))
	for i, s := range scores {
		inputs[i] = map[string]any{"dimension": s.Dimension, "score": s.Score}
	}
	rec := c.callRaw("review_submit", map[string]any{
		"request_id": requestID,
		"review_id":  reviewID,
		"decision":   decision,
		"scores":     inputs,
		"summary":    summary,
	})
	return c.parseReviewResult(rec)
}

// ReviewGet 通过 MCP 查询 review。
func (c *MCPClient) ReviewGet(reviewID string) mcpReviewResult {
	c.t.Helper()
	rec := c.callRaw("review_get", map[string]any{"review_id": reviewID})
	return c.parseReviewResult(rec)
}

// ReputationGet 通过 MCP 查询声望投影。
func (c *MCPClient) ReputationGet(agentVersionID, capability, taskType string) mcpReputationResult {
	c.t.Helper()
	rec := c.callRaw("reputation_get", map[string]any{
		"agent_version_id": agentVersionID,
		"capability":       capability,
		"task_type":        taskType,
	})
	return c.parseReputationResult(rec)
}

type mcpReviewResult struct {
	Code   string
	Review reviewapp.ReviewView
	Meta   application.Meta
}

type mcpReputationResult struct {
	Code       string
	Projection reputationapp.ProjectionView
	Meta       application.Meta
}

type mcpResult struct {
	Code      string
	Execution application.ExecutionView
	Meta      application.Meta
}

func (c *MCPClient) call(tool string, args map[string]any) mcpResult {
	c.t.Helper()
	rec := c.callRaw(tool, args)
	return c.parseResult(rec)
}

func (c *MCPClient) parseReviewResult(rec *httptest.ResponseRecorder) mcpReviewResult {
	c.t.Helper()
	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &rpcResp))

	if rpcResp.Error.Code != 0 {
		return mcpReviewResult{Code: "MCP_ERROR"}
	}
	if rpcResp.Result.IsError {
		require.NotEmpty(c.t, rpcResp.Result.Content, "error result has no content")
		var mcpErr struct {
			Code string `json:"code"`
		}
		require.NoError(c.t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &mcpErr))
		return mcpReviewResult{Code: mcpErr.Code}
	}

	require.NotEmpty(c.t, rpcResp.Result.Content, "success result has no content")
	var envelope struct {
		Data reviewapp.ReviewView `json:"data"`
		Meta application.Meta     `json:"meta"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &envelope))
	c.lastMeta = envelope.Meta
	return mcpReviewResult{Review: envelope.Data, Meta: envelope.Meta}
}

func (c *MCPClient) parseReputationResult(rec *httptest.ResponseRecorder) mcpReputationResult {
	c.t.Helper()
	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &rpcResp))

	if rpcResp.Error.Code != 0 {
		return mcpReputationResult{Code: "MCP_ERROR"}
	}
	if rpcResp.Result.IsError {
		require.NotEmpty(c.t, rpcResp.Result.Content, "error result has no content")
		var mcpErr struct {
			Code string `json:"code"`
		}
		require.NoError(c.t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &mcpErr))
		return mcpReputationResult{Code: mcpErr.Code}
	}

	require.NotEmpty(c.t, rpcResp.Result.Content, "success result has no content")
	var envelope struct {
		Data reputationapp.ProjectionView `json:"data"`
		Meta application.Meta             `json:"meta"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &envelope))
	c.lastMeta = envelope.Meta
	return mcpReputationResult{Projection: envelope.Data, Meta: envelope.Meta}
}

func (c *MCPClient) callRaw(tool string, args map[string]any) *httptest.ResponseRecorder {
	c.t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": args,
		},
	})
	require.NoError(c.t, err)

	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	return rec
}

func (c *MCPClient) parseResult(rec *httptest.ResponseRecorder) mcpResult {
	c.t.Helper()
	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &rpcResp))

	if rpcResp.Error.Code != 0 {
		return mcpResult{Code: "MCP_ERROR"}
	}
	if rpcResp.Result.IsError {
		require.NotEmpty(c.t, rpcResp.Result.Content, "error result has no content")
		var mcpErr struct {
			Code string `json:"code"`
		}
		require.NoError(c.t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &mcpErr))
		return mcpResult{Code: mcpErr.Code}
	}

	require.NotEmpty(c.t, rpcResp.Result.Content, "success result has no content")
	var envelope struct {
		Data application.ExecutionView `json:"data"`
		Meta application.Meta          `json:"meta"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(rpcResp.Result.Content[0].Text), &envelope))
	c.lastMeta = envelope.Meta
	return mcpResult{Execution: envelope.Data, Meta: envelope.Meta}
}

func (c *MCPClient) DB() *pgxpool.Pool {
	return c.db
}

// --- REST client ---

// RESTClient 通过 REST API 调用。
type RESTClient struct {
	t        *testing.T
	handler  http.Handler
	db       *pgxpool.Pool
	token    string
	lastMeta application.Meta
}

func (c *RESTClient) As(token string) *RESTClient {
	return &RESTClient{t: c.t, handler: c.handler, db: c.db, token: token}
}

// LastMeta 返回最近一次成功调用的 envelope meta。
func (c *RESTClient) LastMeta() application.Meta { return c.lastMeta }

func (c *RESTClient) ClaimTask(taskID, requestID string) application.ExecutionView {
	c.t.Helper()
	res := c.post("/v1/tasks/"+taskID+":claim", requestID, map[string]any{"request_id": requestID})
	require.Empty(c.t, res.Code, "REST claim failed: %s", res.Code)
	return res.Execution
}

func (c *RESTClient) Heartbeat(executionID string, generation int64, requestID string) application.ExecutionView {
	c.t.Helper()
	res := c.post("/v1/executions/"+executionID+":heartbeat", requestID, map[string]any{
		"request_id":       requestID,
		"lease_generation": generation,
	})
	require.Empty(c.t, res.Code, "REST heartbeat failed: %s", res.Code)
	return res.Execution
}

func (c *RESTClient) HeartbeatCode(executionID string, generation int64, requestID string) string {
	c.t.Helper()
	return c.post("/v1/executions/"+executionID+":heartbeat", requestID, map[string]any{
		"request_id":       requestID,
		"lease_generation": generation,
	}).Code
}

// CreateReview 为指定 submission 创建 review。
func (c *RESTClient) CreateReview(submissionID string, capabilities []string, requestID string) restReviewResult {
	c.t.Helper()
	return c.postReview("/v1/submissions/"+submissionID+"/reviews", requestID, map[string]any{
		"request_id":   requestID,
		"capabilities": capabilities,
	})
}

// SubmitDecision 提交 review 决策。
func (c *RESTClient) SubmitDecision(reviewID, decision string, scores []reviewdomain.RubricScore, summary, requestID string) restReviewResult {
	c.t.Helper()
	return c.postReview("/v1/reviews/"+reviewID+"/decision", requestID, map[string]any{
		"request_id": requestID,
		"decision":   decision,
		"scores":     scores,
		"summary":    summary,
	})
}

// AddComment 为 review 添加行级注释。
func (c *RESTClient) AddComment(reviewID, submissionID, text, requestID string) restCommentResult {
	c.t.Helper()
	return c.postComment("/v1/reviews/"+reviewID+"/comments", requestID, map[string]any{
		"request_id":       requestID,
		"submission_id":    submissionID,
		"file_path":        "main.go",
		"side":             "right",
		"line_number":      42,
		"hunk_hash":        "h1",
		"diff_fingerprint": "d1",
		"text":             text,
	})
}

// SubmitForReview 将运行中的 execution 推进到 reviewing。
func (c *RESTClient) SubmitForReview(executionID, requestID string) restResult {
	c.t.Helper()
	return c.post("/v1/executions/"+executionID+":submit_for_review", requestID, map[string]any{
		"request_id": requestID,
	})
}

// GetReputation 查询指定 key 的声望投影。
func (c *RESTClient) GetReputation(agentVersionID, capability, taskType string) restReputationResult {
	c.t.Helper()
	path := fmt.Sprintf("/v1/reputation?agent_version_id=%s&capability=%s&task_type=%s", agentVersionID, capability, taskType)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return restReputationResult{Code: resp.Error.Code}
	}
	var envelope struct {
		Data reputationapp.ProjectionView `json:"data"`
		Meta application.Meta             `json:"meta"`
	}
	require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	c.lastMeta = envelope.Meta
	return restReputationResult{Projection: envelope.Data, Meta: envelope.Meta}
}

type restReputationResult struct {
	Code       string
	Projection reputationapp.ProjectionView
	Meta       application.Meta
}

type restReviewResult struct {
	Code   string
	Review reviewapp.ReviewView
	Meta   application.Meta
}

type restCommentResult struct {
	Code    string
	Comment reviewapp.CommentView
	Meta    application.Meta
}

func (c *RESTClient) postReview(path, requestID string, body map[string]any) restReviewResult {
	c.t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(c.t, err)

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Idempotency-Key", requestID)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return restReviewResult{Code: resp.Error.Code}
	}
	var envelope struct {
		Data reviewapp.ReviewView `json:"data"`
		Meta application.Meta     `json:"meta"`
	}
	require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	c.lastMeta = envelope.Meta
	return restReviewResult{Review: envelope.Data, Meta: envelope.Meta}
}

func (c *RESTClient) postComment(path, requestID string, body map[string]any) restCommentResult {
	c.t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(c.t, err)

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Idempotency-Key", requestID)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return restCommentResult{Code: resp.Error.Code}
	}
	var envelope struct {
		Data reviewapp.CommentView `json:"data"`
		Meta application.Meta      `json:"meta"`
	}
	require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	c.lastMeta = envelope.Meta
	return restCommentResult{Comment: envelope.Data, Meta: envelope.Meta}
}

type restResult struct {
	Code      string
	Execution application.ExecutionView
	Meta      application.Meta
}

func (c *RESTClient) post(path, requestID string, body map[string]any) restResult {
	c.t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(c.t, err)

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Idempotency-Key", requestID)
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		var resp struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &resp))
		return restResult{Code: resp.Error.Code}
	}
	var envelope struct {
		Data application.ExecutionView `json:"data"`
		Meta application.Meta          `json:"meta"`
	}
	require.NoError(c.t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	c.lastMeta = envelope.Meta
	return restResult{Execution: envelope.Data, Meta: envelope.Meta}
}

type RegisterAgentRequest struct {
	Name           string
	Description    string
	Team           string
	Scopes         []string
	RepoScope      []string
	BudgetCents    int64
	BudgetCurrency string
}

type ActivateAgentRequest struct {
	Runtime           string
	Model             string
	Capabilities      []string
	ConfigFingerprint string
}

type IdentityAuditEvent struct {
	Intent string `json:"intent"`
}

type ActivationCredentialRecord struct {
	Status             string
	Hash               []byte
	HashBase64         string
	HasPlaintextColumn bool
}

type IdentityClient struct {
	t             *testing.T
	handler       http.Handler
	sessionSecret string
	lastBody      string
}

func (c *IdentityClient) LastBody() string {
	return c.lastBody
}

func (c *IdentityClient) DecodeLastBody(dst any) {
	c.t.Helper()
	require.NoError(c.t, json.Unmarshal([]byte(c.lastBody), dst))
}

func (c *IdentityClient) RegisterAgent(cookie *http.Cookie, req RegisterAgentRequest) identityapp.RegisterAgentResponse {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents", map[string]any{
		"name":            req.Name,
		"description":     req.Description,
		"team":            req.Team,
		"scopes":          req.Scopes,
		"repo_scope":      req.RepoScope,
		"budget_cents":    req.BudgetCents,
		"budget_currency": req.BudgetCurrency,
	}, "", cookie)
	require.Equal(c.t, http.StatusCreated, res.Code, "register failed: %s", res.Body)
	var envelope struct {
		Data identityapp.RegisterAgentResponse `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) ListAgents(cookie *http.Cookie) identityapp.AgentPage {
	c.t.Helper()
	res := c.doJSON(http.MethodGet, "/v1/agents", nil, "", cookie)
	require.Equal(c.t, http.StatusOK, res.Code, "list agents failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AgentPage `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) GetAgentCode(cookie *http.Cookie, agentID string) string {
	c.t.Helper()
	res := c.doJSON(http.MethodGet, "/v1/agents/"+agentID, nil, "", cookie)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) ActivateAgent(token string, req ActivateAgentRequest) identityapp.AccessTokenView {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/me:activate", map[string]any{
		"activation_token":   token,
		"runtime":            req.Runtime,
		"model":              req.Model,
		"capabilities":       req.Capabilities,
		"config_fingerprint": req.ConfigFingerprint,
	}, "", nil)
	require.Equal(c.t, http.StatusOK, res.Code, "activate failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AccessTokenView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) ActivateAgentCode(token string, req ActivateAgentRequest) string {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/me:activate", map[string]any{
		"activation_token":   token,
		"runtime":            req.Runtime,
		"model":              req.Model,
		"capabilities":       req.Capabilities,
		"config_fingerprint": req.ConfigFingerprint,
	}, "", nil)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) RefreshToken(token string) identityapp.AccessTokenView {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/me:refresh", map[string]any{}, token, nil)
	require.Equal(c.t, http.StatusOK, res.Code, "refresh failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AccessTokenView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) RefreshTokenCode(token string) string {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/me:refresh", map[string]any{}, token, nil)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) Heartbeat(token string) identityapp.AgentView {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/me:heartbeat", map[string]any{}, token, nil)
	require.Equal(c.t, http.StatusOK, res.Code, "heartbeat failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AgentView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) HeartbeatCode(token string) string {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/me:heartbeat", map[string]any{}, token, nil)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) GetSelf(token string) identityapp.AgentView {
	c.t.Helper()
	res := c.doJSON(http.MethodGet, "/v1/agents/me", nil, token, nil)
	require.Equal(c.t, http.StatusOK, res.Code, "get self failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AgentView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) SuspendAgent(cookie *http.Cookie, agentID, reason string) identityapp.AgentView {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/"+agentID+":suspend", map[string]any{"reason": reason}, "", cookie)
	require.Equal(c.t, http.StatusOK, res.Code, "suspend failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AgentView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) SuspendAgentCode(cookie *http.Cookie, agentID, reason string) string {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/"+agentID+":suspend", map[string]any{"reason": reason}, "", cookie)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) ResumeAgent(cookie *http.Cookie, agentID string) identityapp.AgentView {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/"+agentID+":resume", map[string]any{}, "", cookie)
	require.Equal(c.t, http.StatusOK, res.Code, "resume failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AgentView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) RevokeAgent(cookie *http.Cookie, agentID, reason string) identityapp.AgentView {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/"+agentID+":revoke", map[string]any{"reason": reason}, "", cookie)
	require.Equal(c.t, http.StatusOK, res.Code, "revoke failed: %s", res.Body)
	var envelope struct {
		Data identityapp.AgentView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) RevokeAgentCode(cookie *http.Cookie, agentID, reason string) string {
	c.t.Helper()
	res := c.doJSON(http.MethodPost, "/v1/agents/"+agentID+":revoke", map[string]any{"reason": reason}, "", cookie)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) GetActivationStatus(cookie *http.Cookie, agentID string) identityapp.ActivationStatusView {
	c.t.Helper()
	res := c.doJSON(http.MethodGet, "/v1/agents/"+agentID+":token", nil, "", cookie)
	require.Equal(c.t, http.StatusOK, res.Code, "status failed: %s", res.Body)
	var envelope struct {
		Data identityapp.ActivationStatusView `json:"data"`
	}
	require.NoError(c.t, json.Unmarshal([]byte(res.Body), &envelope))
	return envelope.Data
}

func (c *IdentityClient) GetActivationStatusCode(cookie *http.Cookie, agentID string) string {
	c.t.Helper()
	res := c.doJSON(http.MethodGet, "/v1/agents/"+agentID+":token", nil, "", cookie)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) ListAgentsCode() string {
	c.t.Helper()
	res := c.doJSON(http.MethodGet, "/v1/agents", nil, "", nil)
	return errorCodeFromJSON(c.t, res.Code, res.Body)
}

func (c *IdentityClient) doJSON(method, path string, body map[string]any, bearer string, cookie *http.Cookie) httpResult {
	c.t.Helper()
	var reader *strings.Reader
	if body == nil {
		reader = strings.NewReader("")
	} else {
		raw, err := json.Marshal(body)
		require.NoError(c.t, err)
		reader = strings.NewReader(string(raw))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	c.lastBody = rec.Body.String()
	return httpResult{Code: rec.Code, Body: rec.Body.String()}
}

type httpResult struct {
	Code int
	Body string
}

func errorCodeFromJSON(t *testing.T, statusCode int, body string) string {
	t.Helper()
	require.NotEqual(t, http.StatusOK, statusCode)
	require.NotEqual(t, http.StatusCreated, statusCode)
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	return resp.Error.Code
}

func ownerSession() *http.Cookie {
	cookie, err := auth.NewSessionCookie(auth.Session{
		TenantID:   "tenant-1",
		OwnerID:    "owner-1",
		OwnerEmail: "owner-1@example.com",
		IsAdmin:    false,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, "acceptance-session-secret-0123456789abcdef", false)
	if err != nil {
		panic(err)
	}
	return cookie
}

func otherOwnerSession() *http.Cookie {
	return tenantOwnerSession("tenant-1", "owner-2")
}

func tenantOwnerSession(tenantID, ownerID string) *http.Cookie {
	cookie, err := auth.NewSessionCookie(auth.Session{
		TenantID:   tenantID,
		OwnerID:    ownerID,
		OwnerEmail: ownerID + "@example.com",
		IsAdmin:    false,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, "acceptance-session-secret-0123456789abcdef", false)
	if err != nil {
		panic(err)
	}
	return cookie
}

func (env *Env) IdentityAuditEvents(agentID string) ([]IdentityAuditEvent, error) {
	rows, err := env.DB.Query(context.Background(), `
		SELECT intent
		FROM identity_events
		WHERE tenant_id = $1 AND agent_id = $2
		ORDER BY created_at ASC, id ASC`, "tenant-1", agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []IdentityAuditEvent
	for rows.Next() {
		var event IdentityAuditEvent
		if err := rows.Scan(&event.Intent); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (env *Env) IdentityAuditEventsForTenant(tenantID, agentID string) ([]IdentityAuditEvent, error) {
	rows, err := env.DB.Query(context.Background(), `
		SELECT intent
		FROM identity_events
		WHERE tenant_id = $1 AND agent_id = $2
		ORDER BY created_at ASC, id ASC`, tenantID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []IdentityAuditEvent
	for rows.Next() {
		var event IdentityAuditEvent
		if err := rows.Scan(&event.Intent); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (env *Env) ActivationCredentialRecord(tenantID, agentID string) ActivationCredentialRecord {
	env.T.Helper()
	var record ActivationCredentialRecord
	err := env.DB.QueryRow(context.Background(), `
		SELECT hash, status
		FROM activation_credentials
		WHERE tenant_id = $1 AND agent_id = $2
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, tenantID, agentID).Scan(&record.Hash, &record.Status)
	require.NoError(env.T, err)
	record.HashBase64 = base64.RawURLEncoding.EncodeToString(record.Hash)

	err = env.DB.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'activation_credentials'
			  AND column_name IN ('token', 'plaintext', 'plaintext_token', 'activation_token')
		)`).Scan(&record.HasPlaintextColumn)
	require.NoError(env.T, err)
	return record
}

type acceptanceFixedIssuer struct{ issuer *auth.TokenIssuer }

func (i acceptanceFixedIssuer) IssueAccessToken(_ context.Context, agent *identitydomain.Agent, version *identitydomain.AgentVersion, now time.Time) (identityapp.AccessTokenView, error) {
	token, err := i.issuer.Issue(agent, version, now)
	if err != nil {
		return identityapp.AccessTokenView{}, err
	}
	return identityapp.AccessTokenView{
		Token:          token,
		TokenType:      "Bearer",
		ExpiresAt:      now.Add(15 * time.Minute),
		AgentID:        agent.ID,
		AgentVersionID: version.ID,
		Scopes:         append([]string(nil), agent.Scopes...),
		RepoScope:      append([]string(nil), agent.RepoScope...),
	}, nil
}

func acceptanceSequenceIDs(values ...string) func() string {
	index := 0
	return func() string {
		if index >= len(values) {
			return values[len(values)-1]
		}
		value := values[index]
		index++
		return value
	}
}

// --- review & reputation helpers ---

func seedAcceptanceRubric(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	repo := reviewpostgres.NewRubricRepository(db)
	version, err := reviewdomain.NewRubricVersion(
		"rubric-acceptance", "tenant-1", "Acceptance Rubric", 1,
		[]reviewdomain.RubricDimension{{ID: "quality", Name: "Quality"}},
		map[string]float64{"quality": 1.0},
		"2026-07-04-v1", time.Now(),
	)
	require.NoError(t, err)
	require.NoError(t, repo.CreateVersion(context.Background(), version))
}

func seedAcceptanceReviewer(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	repo := reviewpostgres.NewReviewerRepository(db)
	profile, err := reviewdomain.NewReviewerProfile(
		"reviewer-1", "tenant-1", "reviewer-user-1", []string{"go"}, time.Now(),
	)
	require.NoError(t, err)
	require.NoError(t, repo.Insert(context.Background(), profile))
}

// acceptanceDiffProvider 是 diff provider 的内存桩，返回固定结构化 diff 内容。
// TODO: replace with git-delivery-and-validation implementation
type acceptanceDiffProvider struct{}

func (acceptanceDiffProvider) GetDiff(context.Context, string) ([]reviewapp.FileDiff, error) {
	return []reviewapp.FileDiff{{
		Path: "main.go",
		Hunks: []reviewapp.Hunk{{
			OldStart: 1,
			OldLines: 0,
			NewStart: 1,
			NewLines: 1,
			HunkHash: "h1",
			Lines: []reviewapp.DiffLine{
				{Type: "add", Text: "+func Run() {}", NewLine: 1},
			},
		}},
	}}, nil
}

// acceptanceValidationProvider 是 validation provider 的内存桩，可控制硬 gate 结果。
// TODO: replace with git-delivery-and-validation implementation
type acceptanceValidationProvider struct {
	pass bool
}

func (a *acceptanceValidationProvider) GetValidationStatus(context.Context, string) (reviewapp.ValidationStatus, error) {
	return acceptanceValidationStatus{pass: a.pass}, nil
}

type acceptanceValidationStatus struct {
	pass bool
}

func (v acceptanceValidationStatus) AllHardGatesPassed() bool { return v.pass }

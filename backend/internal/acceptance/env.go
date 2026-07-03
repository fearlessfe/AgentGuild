package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	transportmcp "agentguild.dev/agentguild/backend/internal/transport/mcp"
	"agentguild.dev/agentguild/backend/internal/transport/rest"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// Env 是端到端验收测试的共享 harness，包含真实 PostgreSQL、应用服务与两个 transport。
type Env struct {
	T         *testing.T
	DB        *pgxpool.Pool
	Service   *application.Service
	MCP       *MCPClient
	REST      *RESTClient
	publishSeq int64
}

// Start 启动一个隔离的验收环境。
func Start(t *testing.T) *Env {
	t.Helper()

	db := testdb.StartPostgres(t)
	store := postgres.NewStore(db)
	svc, err := application.NewService(store, application.Options{
		CursorSecret: []byte("0123456789abcdef0123456789abcdef"),
		CursorTTL:    15 * time.Minute,
	})
	require.NoError(t, err)

	verifier := fakeVerifier{}
	mcpHandler := transportmcp.NewServer(svc, verifier).Handler()
	restHandler := rest.NewServer(svc, verifier).Router()

	return &Env{
		T:       t,
		DB:      db,
		Service: svc,
		MCP:     &MCPClient{t: t, handler: mcpHandler, db: db, token: "token-agent"},
		REST:    &RESTClient{t: t, handler: restHandler, db: db, token: "token-agent"},
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

// AsAgent 返回使用指定 agent token 的 MCP 客户端。
func (env *Env) AsAgent(token string) *MCPClient {
	return env.MCP.As(token)
}

// --- fake verifier ---

type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, rawToken string) (auth.Principal, error) {
	switch {
	case rawToken == "token-publisher":
		return publisherPrincipal(), nil
	case rawToken == "token-agent":
		return agentPrincipal(1), nil
	case rawToken == "token-admin":
		return adminPrincipal(), nil
	case strings.HasPrefix(rawToken, "token-agent-"):
		var i int
		if _, err := fmt.Sscanf(rawToken, "token-agent-%d", &i); err == nil {
			return agentPrincipal(i), nil
		}
	}
	return auth.Principal{}, fmt.Errorf("unknown token")
}

func publisherPrincipal() auth.Principal {
	return auth.Principal{
		TenantID:       "tenant-1",
		AgentID:        "publisher",
		AgentVersionID: "publisher-v1",
		Scopes:         []string{"tasks:publish", "tasks:read", "tasks:cancel"},
	}
}

func agentPrincipal(i int) auth.Principal {
	id := fmt.Sprintf("agent-%d", i)
	return auth.Principal{
		TenantID:       "tenant-1",
		AgentID:        id,
		AgentVersionID: id,
		Scopes:         []string{"tasks:claim", "tasks:execute"},
	}
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

// TaskClaimCode 返回 claim 的错误码。
func (c *MCPClient) TaskClaimCode(taskID, requestID string) string {
	c.t.Helper()
	return c.call("task_claim", map[string]any{"request_id": requestID, "task_id": taskID}).Code
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

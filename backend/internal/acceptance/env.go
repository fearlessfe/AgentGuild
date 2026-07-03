package acceptance

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
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
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	identitypostgres "agentguild.dev/agentguild/backend/internal/identity/postgres"
	"agentguild.dev/agentguild/backend/internal/postgres"
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
	svc, err := application.NewService(store, application.Options{
		CursorSecret: []byte("0123456789abcdef0123456789abcdef"),
		CursorTTL:    15 * time.Minute,
	})
	require.NoError(t, err)
	verifier, tokenIssuer := newAcceptanceIdentityRuntime(t)
	identitySvc, err := identityapp.NewIdentityService(identitypostgres.NewStore(db), identityapp.IdentityOptions{
		NewID:       acceptanceSequenceIDs("agent-1", "version-1", "agent-2", "version-2", "agent-3", "version-3"),
		TokenIssuer: tokenIssuer,
	})
	require.NoError(t, err)

	mcpHandler := transportmcp.NewServer(svc, verifier).Handler()
	restHandler := rest.NewServer(svc, verifier,
		rest.WithIdentityService(identitySvc),
		rest.WithSession("acceptance-session-secret-0123456789abcdef", false),
	).Router()

	return &Env{
		T:       t,
		DB:      db,
		Service: svc,
		MCP:     &MCPClient{t: t, handler: mcpHandler, db: db, token: "token-agent"},
		REST:    &RESTClient{t: t, handler: restHandler, db: db, token: "token-agent"},
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
	case strings.HasPrefix(rawToken, "token-agent-"):
		var i int
		if _, err := fmt.Sscanf(rawToken, "token-agent-%d", &i); err == nil {
			return agentPrincipal(i), nil
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

type IdentityClient struct {
	t             *testing.T
	handler       http.Handler
	sessionSecret string
	lastBody      string
}

func (c *IdentityClient) LastBody() string {
	return c.lastBody
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
	cookie, err := auth.NewSessionCookie(auth.Session{
		TenantID:   "tenant-1",
		OwnerID:    "owner-2",
		OwnerEmail: "owner-2@example.com",
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

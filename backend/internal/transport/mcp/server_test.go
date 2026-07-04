package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// fakeApplication 记录调用参数并按预置值返回，用于验证 MCP 到 Application Service 的映射。
type fakeApplication struct {
	calls []call

	publish         application.Envelope[application.TaskView]
	publishErr      error
	list            application.Envelope[application.TaskPage]
	listErr         error
	getTask         application.Envelope[application.TaskView]
	getTaskErr      error
	cancel          application.Envelope[application.TaskView]
	cancelErr       error
	claim           application.Envelope[application.ExecutionView]
	claimErr        error
	start           application.Envelope[application.ExecutionView]
	startErr        error
	heartbeat       application.Envelope[application.ExecutionView]
	heartbeatErr    error
	getExecution    application.Envelope[application.ExecutionView]
	getExecutionErr error
}

type call struct {
	method    string
	principal auth.Principal
	payload   any
}

func (f *fakeApplication) PublishTask(ctx context.Context, p auth.Principal, cmd application.PublishTask) (application.Envelope[application.TaskView], error) {
	f.calls = append(f.calls, call{method: "PublishTask", principal: p, payload: cmd})
	return f.publish, f.publishErr
}

func (f *fakeApplication) ListTasks(ctx context.Context, p auth.Principal, q application.ListTasks) (application.Envelope[application.TaskPage], error) {
	f.calls = append(f.calls, call{method: "ListTasks", principal: p, payload: q})
	return f.list, f.listErr
}

func (f *fakeApplication) GetTask(ctx context.Context, p auth.Principal, q application.GetTask) (application.Envelope[application.TaskView], error) {
	f.calls = append(f.calls, call{method: "GetTask", principal: p, payload: q})
	return f.getTask, f.getTaskErr
}

func (f *fakeApplication) CancelTask(ctx context.Context, p auth.Principal, cmd application.CancelTask) (application.Envelope[application.TaskView], error) {
	f.calls = append(f.calls, call{method: "CancelTask", principal: p, payload: cmd})
	return f.cancel, f.cancelErr
}

func (f *fakeApplication) ClaimTask(ctx context.Context, p auth.Principal, cmd application.ClaimTask) (application.Envelope[application.ExecutionView], error) {
	f.calls = append(f.calls, call{method: "ClaimTask", principal: p, payload: cmd})
	return f.claim, f.claimErr
}

func (f *fakeApplication) StartExecution(ctx context.Context, p auth.Principal, cmd application.StartExecution) (application.Envelope[application.ExecutionView], error) {
	f.calls = append(f.calls, call{method: "StartExecution", principal: p, payload: cmd})
	return f.start, f.startErr
}

func (f *fakeApplication) HeartbeatExecution(ctx context.Context, p auth.Principal, cmd application.HeartbeatExecution) (application.Envelope[application.ExecutionView], error) {
	f.calls = append(f.calls, call{method: "HeartbeatExecution", principal: p, payload: cmd})
	return f.heartbeat, f.heartbeatErr
}

func (f *fakeApplication) GetExecution(ctx context.Context, p auth.Principal, q application.GetExecution) (application.Envelope[application.ExecutionView], error) {
	f.calls = append(f.calls, call{method: "GetExecution", principal: p, payload: q})
	return f.getExecution, f.getExecutionErr
}

type fakeVerifier struct {
	principal auth.Principal
	err       error
}

func (v *fakeVerifier) Verify(ctx context.Context, rawToken string) (auth.Principal, error) {
	if v.err != nil {
		return auth.Principal{}, v.err
	}
	return v.principal, nil
}

func testPrincipal(scopes ...string) auth.Principal {
	return auth.Principal{
		TenantID:       "tenant-1",
		AgentID:        "agent-1",
		AgentVersionID: "agent-1-v1",
		Scopes:         scopes,
	}
}

func newMCPServer(t *testing.T) *mcp.Server {
	t.Helper()
	app := &fakeApplication{}
	verifier := &fakeVerifier{principal: testPrincipal("tasks:claim", "tasks:execute", "tasks:read", "tasks:publish", "tasks:cancel")}
	s := NewServer(app, verifier)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), verifier.principal))
	return s.mcpServer(req)
}

func toolSchema(t *testing.T, server *mcp.Server, name string) map[string]any {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() {
		_, _ = server.Connect(ctx, serverTransport, nil)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	for _, tool := range result.Tools {
		if tool.Name == name {
			schema, ok := tool.InputSchema.(map[string]any)
			require.Truef(t, ok, "tool %q input schema is not a map", name)
			return schema
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func TestToolCallsMapToApplicationService(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	app := &fakeApplication{
		claim: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", Status: domain.ExecutionLeased},
			Meta: application.Meta{ServerTime: now, ResourceVersion: 1},
		},
		start: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", Status: domain.ExecutionRunning, Stage: "planning", Progress: 0.25},
			Meta: application.Meta{ServerTime: now, ResourceVersion: 2},
		},
	}
	server := newMCPServerWithApp(t, app)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "task_claim",
		Arguments: map[string]any{
			"request_id": "req-1",
			"task_id":    "task-1",
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, app.calls, 1)
	require.Equal(t, "ClaimTask", app.calls[0].method)
	require.Equal(t, "req-1", app.calls[0].payload.(application.ClaimTask).RequestID)
	require.Equal(t, "task-1", app.calls[0].payload.(application.ClaimTask).TaskID)

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "execution_start",
		Arguments: map[string]any{
			"request_id":       "req-2",
			"execution_id":     "exe-1",
			"lease_generation": int64(1),
			"stage":            "planning",
			"progress":         0.25,
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, app.calls, 2)
	require.Equal(t, "StartExecution", app.calls[1].method)
	require.Equal(t, "exe-1", app.calls[1].payload.(application.StartExecution).ExecutionID)
	require.NotNil(t, app.calls[1].payload.(application.StartExecution).Stage)
	require.Equal(t, "planning", *app.calls[1].payload.(application.StartExecution).Stage)
	require.NotNil(t, app.calls[1].payload.(application.StartExecution).Progress)
	require.InDelta(t, 0.25, *app.calls[1].payload.(application.StartExecution).Progress, 0.001)
}

func TestDomainErrorsMapToStableMCPCodes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		admin      bool
		wantCode   string
		wantSubstr string
	}{
		{"state_conflict", &domain.Error{Code: "state_conflict", Message: "task is not claimable"}, false, "STATE_CONFLICT", "task is not claimable"},
		{"lease_expired", &domain.Error{Code: "lease_expired", Message: "lease is expired"}, false, "LEASE_EXPIRED", "lease is expired"},
		{"idempotency_mismatch", &domain.Error{Code: "idempotency_mismatch", Message: "idempotency mismatch"}, false, "IDEMPOTENCY_MISMATCH", "idempotency mismatch"},
		{"deadline_exceeded", &domain.Error{Code: "deadline_exceeded", Message: "deadline exceeded"}, false, "DEADLINE_EXCEEDED", "deadline exceeded"},
		{"token_revoked", identitydomain.ErrTokenRevoked, false, "TOKEN_REVOKED", "token has been revoked"},
		{"forbidden_non_admin", &domain.Error{Code: "forbidden", Message: "not allowed"}, false, "NOT_FOUND", "resource not found"},
		{"forbidden_admin", &domain.Error{Code: "forbidden", Message: "not allowed"}, true, "FORBIDDEN", "not allowed"},
		{"not_found_non_admin", &domain.Error{Code: "not_found", Message: "hidden"}, false, "NOT_FOUND", "resource not found"},
		{"not_found_admin", &domain.Error{Code: "not_found", Message: "hidden"}, true, "NOT_FOUND", "hidden"},
		{"invalid_argument", &domain.Error{Code: "invalid_argument", Message: "task_id is invalid", Field: "task_id"}, false, "INVALID_ARGUMENT", "task_id is invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := &fakeApplication{claimErr: tc.err}
			server := newMCPServerWithAppAndPrincipal(t, app, func() auth.Principal {
				p := testPrincipal("tasks:claim")
				if tc.admin {
					p.Scopes = append(p.Scopes, "admin:tasks")
				}
				return p
			}())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			serverTransport, clientTransport := mcp.NewInMemoryTransports()
			go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
			client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
			session, err := client.Connect(ctx, clientTransport, nil)
			require.NoError(t, err)
			defer session.Close()

			res, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "task_claim",
				Arguments: map[string]any{"request_id": "req-1", "task_id": "task-1"},
			})
			require.NoError(t, err)
			require.True(t, res.IsError)
			require.Len(t, res.Content, 1)
			text, ok := res.Content[0].(*mcp.TextContent)
			require.True(t, ok)
			var mcpErr MCPError
			require.NoError(t, json.Unmarshal([]byte(text.Text), &mcpErr))
			require.Equal(t, tc.wantCode, mcpErr.Code)
			require.Contains(t, mcpErr.Message, tc.wantSubstr)
		})
	}
}

func TestUnknownFieldsAreRejected(t *testing.T) {
	server := newMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "task_claim",
		Arguments: map[string]any{"request_id": "req-1", "task_id": "task-1", "unknown_field": "x"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestMissingAuthorizationReturns401(t *testing.T) {
	app := &fakeApplication{}
	verifier := &fakeVerifier{principal: testPrincipal("tasks:read")}
	s := NewServer(app, verifier)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	s.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestInvalidTokenReturns401(t *testing.T) {
	app := &fakeApplication{}
	verifier := &fakeVerifier{err: errors.New("invalid token")}
	s := NewServer(app, verifier)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer bad-token")
	s.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func newMCPServerWithApp(t *testing.T, app *fakeApplication) *mcp.Server {
	return newMCPServerWithAppAndPrincipal(t, app, testPrincipal("tasks:claim", "tasks:execute", "tasks:read", "tasks:publish", "tasks:cancel"))
}

func newMCPServerWithAppAndPrincipal(t *testing.T, app *fakeApplication, principal auth.Principal) *mcp.Server {
	t.Helper()
	verifier := &fakeVerifier{principal: principal}
	s := NewServer(app, verifier)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
	return s.mcpServer(req)
}

func TestOversizedRequestBodyIsRejected(t *testing.T) {
	app := &fakeApplication{}
	verifier := &fakeVerifier{principal: testPrincipal("tasks:claim")}
	s := NewServer(app, verifier)
	rec := httptest.NewRecorder()
	large := strings.Repeat("x", (1<<20)+1)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(large))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer token-agent-1")
	s.Handler().ServeHTTP(rec, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestMutationToolsRequireRequestID(t *testing.T) {
	for _, name := range []string{
		"task_publish",
		"task_claim",
		"task_cancel",
		"execution_start",
		"execution_heartbeat",
	} {
		t.Run(name, func(t *testing.T) {
			schema := toolSchema(t, newMCPServer(t), name)
			required, ok := schema["required"].([]any)
			require.True(t, ok, "schema required should be an array")
			var found bool
			for _, r := range required {
				if r == "request_id" {
					found = true
					break
				}
			}
			require.True(t, found, "%s schema should require request_id", name)
		})
	}
}

func TestHeartbeatSchemaExposesStageAndProgress(t *testing.T) {
	schema := toolSchema(t, newMCPServer(t), "execution_heartbeat")
	properties, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "schema properties should be a map")
	require.NotContains(t, properties, "lease_token", "execution_heartbeat schema should not expose lease_token")
	require.Contains(t, properties, "stage", "execution_heartbeat schema should expose stage")
	require.Contains(t, properties, "progress", "execution_heartbeat schema should expose progress")
}

func TestSchemaValidationReturnsInvalidArgument(t *testing.T) {
	server := newMCPServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "execution_heartbeat",
		Arguments: map[string]any{"execution_id": "exe-1", "lease_generation": 1},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)

	var mcpErr MCPError
	require.NoError(t, json.Unmarshal([]byte(text.Text), &mcpErr))
	require.Equal(t, "INVALID_ARGUMENT", mcpErr.Code)
	require.Contains(t, mcpErr.Message, "request_id")
}

func TestDeadlineExceededReturnsTemporarilyUnavailable(t *testing.T) {
	app := &fakeApplication{heartbeatErr: context.DeadlineExceeded}
	server := newMCPServerWithApp(t, app)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "execution_heartbeat",
		Arguments: map[string]any{"request_id": "req-1", "execution_id": "exe-1", "lease_generation": 1},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)

	var mcpErr MCPError
	require.NoError(t, json.Unmarshal([]byte(text.Text), &mcpErr))
	require.Equal(t, "TEMPORARILY_UNAVAILABLE", mcpErr.Code)
	require.Contains(t, mcpErr.Message, "timed out")
}

func TestMCPRateLimiterReturns429WhenLimited(t *testing.T) {
	app := &fakeApplication{}
	verifier := &fakeVerifier{principal: testPrincipal("tasks:read")}
	limiter := &fakeRateLimiter{allowed: false, retryAfter: 30}
	s := NewServer(app, verifier, WithRateLimiter(limiter))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer token-agent-1")
	s.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	require.Equal(t, "30", rec.Header().Get("Retry-After"))
	var body struct {
		Error MCPError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "RATE_LIMITED", body.Error.Code)
	require.Equal(t, 30, body.Error.RetryAfterSeconds)
}

func TestMCPApplicationRateLimitIncludesRetryAfterSeconds(t *testing.T) {
	result := mapDomainError(&domain.Error{Code: "rate_limited", Message: "rate limit exceeded", RetryAfter: 1500 * time.Millisecond}, testPrincipal())
	require.True(t, result.IsError)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	var got MCPError
	require.NoError(t, json.Unmarshal([]byte(text.Text), &got))
	require.Equal(t, "RATE_LIMITED", got.Code)
	require.Equal(t, 2, got.RetryAfterSeconds)
}

type fakeRateLimiter struct {
	allowed    bool
	retryAfter int
}

func (f *fakeRateLimiter) Allow(ctx context.Context, key string) (bool, int) {
	return f.allowed, f.retryAfter
}

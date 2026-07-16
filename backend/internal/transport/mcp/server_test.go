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
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	reputationapp "agentguild.dev/agentguild/backend/internal/reputation/application"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"

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

// fakeReviewService 记录代码评审应用服务调用并按预置值返回。
type fakeReviewService struct {
	calls                 []call
	submitResult          application.Envelope[reviewapp.ReviewView]
	submitErr             error
	getResult             application.Envelope[reviewapp.ReviewView]
	getErr                error
	submitForReviewResult application.Envelope[application.ExecutionView]
	submitForReviewErr    error
}

func (f *fakeReviewService) SubmitDecision(ctx context.Context, p auth.Principal, cmd reviewapp.SubmitDecision) (application.Envelope[reviewapp.ReviewView], error) {
	f.calls = append(f.calls, call{method: "SubmitDecision", principal: p, payload: cmd})
	return f.submitResult, f.submitErr
}

func (f *fakeReviewService) GetReview(ctx context.Context, p auth.Principal, query reviewapp.GetReview) (application.Envelope[reviewapp.ReviewView], error) {
	f.calls = append(f.calls, call{method: "GetReview", principal: p, payload: query})
	return f.getResult, f.getErr
}

func (f *fakeReviewService) SubmitForReview(ctx context.Context, p auth.Principal, cmd reviewapp.SubmitForReview) (application.Envelope[application.ExecutionView], error) {
	f.calls = append(f.calls, call{method: "SubmitForReview", principal: p, payload: cmd})
	return f.submitForReviewResult, f.submitForReviewErr
}

// fakeReputationService 是声望投影服务的占位实现。
type fakeReputationService struct {
	calls         []call
	projection    application.Envelope[reputationapp.ProjectionView]
	projectionErr error
}

func (f *fakeReputationService) GetProjection(ctx context.Context, p auth.Principal, query reputationapp.GetProjection) (application.Envelope[reputationapp.ProjectionView], error) {
	f.calls = append(f.calls, call{method: "GetProjection", principal: p, payload: query})
	return f.projection, f.projectionErr
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

func newMCPServerWithReview(t *testing.T, reviewSvc reviewService, reputationSvc ReputationService) *mcp.Server {
	t.Helper()
	app := &fakeApplication{}
	verifier := &fakeVerifier{principal: testPrincipal("tasks:read", "tasks:publish", "tasks:claim", "tasks:execute")}
	opts := []Option{}
	if reviewSvc != nil {
		opts = append(opts, WithReviewService(reviewSvc))
	}
	if reputationSvc != nil {
		opts = append(opts, WithReputationService(reputationSvc))
	}
	s := NewServer(app, verifier, opts...)
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), verifier.principal))
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

func TestReviewSubmitMapsToApplicationService(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	reviewSvc := &fakeReviewService{
		submitResult: application.Envelope[reviewapp.ReviewView]{
			Data: reviewapp.ReviewView{
				ID:            "review-1",
				SubmissionID:  "sub-1",
				ReviewerID:    "reviewer-1",
				Status:        string(reviewdomain.ReviewSubmitted),
				FinalDecision: string(reviewdomain.DecisionAccepted),
			},
			Meta: application.Meta{ServerTime: now, ResourceVersion: 1},
		},
	}
	server := newMCPServerWithReview(t, reviewSvc, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "review_submit",
		Arguments: map[string]any{
			"request_id": "req-1",
			"review_id":  "review-1",
			"decision":   "accepted",
			"scores": []map[string]any{
				{"dimension": "correctness", "score": 5},
			},
			"summary": "looks good",
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, reviewSvc.calls, 1)
	require.Equal(t, "SubmitDecision", reviewSvc.calls[0].method)
	payload := reviewSvc.calls[0].payload.(reviewapp.SubmitDecision)
	require.Equal(t, "req-1", payload.RequestID)
	require.Equal(t, "review-1", payload.ReviewID)
	require.Equal(t, reviewdomain.DecisionAccepted, payload.Decision)
	require.Len(t, payload.Scores, 1)
	require.Equal(t, "correctness", payload.Scores[0].Dimension)
	require.Equal(t, 5, payload.Scores[0].Score)
	require.Equal(t, "looks good", payload.Summary)
}

func TestReviewGetMapsToApplicationService(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	reviewSvc := &fakeReviewService{
		getResult: application.Envelope[reviewapp.ReviewView]{
			Data: reviewapp.ReviewView{
				ID:            "review-1",
				SubmissionID:  "sub-1",
				ReviewerID:    "reviewer-1",
				Status:        string(reviewdomain.ReviewPending),
				FinalDecision: "",
			},
			Meta: application.Meta{ServerTime: now, ResourceVersion: 1},
		},
	}
	server := newMCPServerWithReview(t, reviewSvc, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "review_get",
		Arguments: map[string]any{"review_id": "review-1"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, reviewSvc.calls, 1)
	require.Equal(t, "GetReview", reviewSvc.calls[0].method)
	require.Equal(t, "review-1", reviewSvc.calls[0].payload.(reviewapp.GetReview).ReviewID)
}

func TestReputationGetReturnsNotImplemented(t *testing.T) {
	server := newMCPServerWithReview(t, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "reputation_get",
		Arguments: map[string]any{
			"agent_version_id": "agent-1-v1",
			"capability":       "code-review",
			"task_type":        "refactor",
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	text, ok := res.Content[0].(*mcp.TextContent)
	require.True(t, ok)
	var mcpErr MCPError
	require.NoError(t, json.Unmarshal([]byte(text.Text), &mcpErr))
	require.Equal(t, "NOT_IMPLEMENTED", mcpErr.Code)
}

func TestReviewToolsRequireRequestID(t *testing.T) {
	server := newMCPServerWithReview(t, &fakeReviewService{}, nil)
	for _, name := range []string{"review_submit"} {
		t.Run(name, func(t *testing.T) {
			schema := toolSchema(t, server, name)
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

type fakeSubmissionService struct {
	calls []submissionCall

	create    gitapp.Envelope[gitapp.SubmissionView]
	createErr error
	get       gitapp.Envelope[gitapp.SubmissionView]
	getErr    error
}

type submissionCall struct {
	method    string
	principal gitapp.Principal
	payload   any
}

func (f *fakeSubmissionService) CreateSubmission(_ context.Context, p gitapp.Principal, cmd gitapp.CreateSubmission) (gitapp.Envelope[gitapp.SubmissionView], error) {
	f.calls = append(f.calls, submissionCall{method: "CreateSubmission", principal: p, payload: cmd})
	return f.create, f.createErr
}

func (f *fakeSubmissionService) GetSubmission(_ context.Context, p gitapp.Principal, q gitapp.GetSubmission) (gitapp.Envelope[gitapp.SubmissionView], error) {
	f.calls = append(f.calls, submissionCall{method: "GetSubmission", principal: p, payload: q})
	return f.get, f.getErr
}

func newMCPServerWithAppAndSubmissions(t *testing.T, app *fakeApplication, sub *fakeSubmissionService) *mcp.Server {
	t.Helper()
	verifier := &fakeVerifier{principal: testPrincipal("tasks:claim", "tasks:execute", "tasks:read", "tasks:publish", "tasks:cancel")}
	s := NewServer(app, verifier, WithSubmissionService(sub))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), verifier.principal))
	return s.mcpServer(req)
}

func TestSubmissionCreateToolRequiresRequestID(t *testing.T) {
	schema := toolSchema(t, newMCPServerWithAppAndSubmissions(t, &fakeApplication{}, &fakeSubmissionService{}), "submission_create")
	required, ok := schema["required"].([]any)
	require.True(t, ok, "schema required should be an array")
	var found bool
	for _, r := range required {
		if r == "request_id" {
			found = true
			break
		}
	}
	require.True(t, found, "submission_create schema should require request_id")
}

func TestSubmissionCreateToolMapsToService(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{
				ID:              "exe-1",
				TaskID:          "task-1",
				TenantID:        "tenant-1",
				AgentVersionID:  "agent-1-v1",
				TaskConstraints: []string{"path:allowed:src/*", "path:forbidden:src/*.env"},
			},
		},
	}
	sub := &fakeSubmissionService{
		create: gitapp.Envelope[gitapp.SubmissionView]{
			Data: gitapp.SubmissionView{
				ID: "sub-1", TenantID: "tenant-1", TaskID: "task-1", ExecutionID: "exe-1",
				Branch: "agentguild/exe-1", CommitSHA: "abc", BaseCommitSHA: "def",
				Summary: "fix parser", Status: gitdomain.SubmissionStatusPendingVerification,
				CreatedAt: now, UpdatedAt: now,
			},
			Meta: gitapp.Meta{ServerTime: now},
		},
	}
	server := newMCPServerWithAppAndSubmissions(t, app, sub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "submission_create",
		Arguments: map[string]any{
			"request_id":      "req-1",
			"execution_id":    "exe-1",
			"repo":            "owner/repo",
			"branch":          "agentguild/exe-1",
			"commit_sha":      "abc",
			"base_commit_sha": "def",
			"summary":         "fix parser",
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, app.calls, 1)
	require.Equal(t, "exe-1", app.calls[0].payload.(application.GetExecution).ExecutionID)
	require.Len(t, sub.calls, 1)
	cmd := sub.calls[0].payload.(gitapp.CreateSubmission)
	require.Equal(t, "req-1", cmd.RequestID)
	require.Equal(t, "exe-1", cmd.ExecutionID)
	require.Equal(t, "task-1", cmd.TaskID)
	require.Equal(t, "owner/repo", cmd.Repo)
	require.Equal(t, []string{"src/*"}, cmd.AllowedPaths)
	require.Equal(t, []string{"src/*.env"}, cmd.ForbiddenPaths)
}

func TestValidationGetToolMapsToService(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1-v1"},
		},
	}
	sub := &fakeSubmissionService{
		get: gitapp.Envelope[gitapp.SubmissionView]{
			Data: gitapp.SubmissionView{
				ID: "sub-1", TenantID: "tenant-1", ExecutionID: "exe-1",
				Status:    gitdomain.SubmissionStatusPendingVerification,
				CreatedAt: now, UpdatedAt: now,
			},
			Meta: gitapp.Meta{ServerTime: now},
		},
	}
	server := newMCPServerWithAppAndSubmissions(t, app, sub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validation_get",
		Arguments: map[string]any{"submission_id": "sub-1"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, sub.calls, 1)
	require.Equal(t, "sub-1", sub.calls[0].payload.(gitapp.GetSubmission).SubmissionID)
	require.Len(t, app.calls, 1)
	require.Equal(t, "exe-1", app.calls[0].payload.(application.GetExecution).ExecutionID)
}

func TestSubmissionCreateHidesNonOwnerExecution(t *testing.T) {
	app := &fakeApplication{getExecutionErr: &domain.Error{Code: "not_found", Message: "hidden"}}
	sub := &fakeSubmissionService{}
	server := newMCPServerWithAppAndSubmissions(t, app, sub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "submission_create",
		Arguments: map[string]any{
			"request_id":      "req-1",
			"execution_id":    "exe-1",
			"repo":            "owner/repo",
			"branch":          "agentguild/exe-1",
			"commit_sha":      "abc",
			"base_commit_sha": "def",
			"summary":         "fix",
		},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
	require.Len(t, sub.calls, 0)
}

func TestValidationGetHidesCrossOwnerExecution(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	app := &fakeApplication{getExecutionErr: &domain.Error{Code: "forbidden", Message: "not owner"}}
	sub := &fakeSubmissionService{
		get: gitapp.Envelope[gitapp.SubmissionView]{
			Data: gitapp.SubmissionView{ID: "sub-1", TenantID: "tenant-1", ExecutionID: "exe-1", CreatedAt: now, UpdatedAt: now},
		},
	}
	server := newMCPServerWithAppAndSubmissions(t, app, sub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validation_get",
		Arguments: map[string]any{"submission_id": "sub-1"},
	})
	require.NoError(t, err)
	require.True(t, res.IsError)
}

func TestSubmissionToolsMapGitPrincipal(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1-v1"},
		},
	}
	sub := &fakeSubmissionService{}
	server := newMCPServerWithAppAndSubmissions(t, app, sub)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	_, _ = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "submission_create",
		Arguments: map[string]any{
			"request_id":      "req-1",
			"execution_id":    "exe-1",
			"repo":            "owner/repo",
			"branch":          "agentguild/exe-1",
			"commit_sha":      "abc",
			"base_commit_sha": "def",
			"summary":         "fix",
		},
	})

	p := sub.calls[0].principal
	require.Equal(t, "tenant-1", p.TenantID)
	require.Equal(t, "agent-1", p.AgentID)
	require.Equal(t, "agent-1-v1", p.AgentVersionID)
	require.Equal(t, []string{"tasks:claim", "tasks:execute", "tasks:read", "tasks:publish", "tasks:cancel"}, p.Scopes)
}

type fakeCredentialService struct {
	calls []credentialCall

	issue     gitapp.Envelope[gitapp.IssueCredentialResponse]
	issueErr  error
	get       gitapp.Envelope[gitapp.CredentialView]
	getErr    error
	revoke    gitapp.Envelope[gitapp.CredentialView]
	revokeErr error
}

type credentialCall struct {
	method    string
	principal gitapp.Principal
	payload   any
}

func (f *fakeCredentialService) IssueCredential(_ context.Context, p gitapp.Principal, cmd gitapp.IssueCredential) (gitapp.Envelope[gitapp.IssueCredentialResponse], error) {
	f.calls = append(f.calls, credentialCall{method: "IssueCredential", principal: p, payload: cmd})
	return f.issue, f.issueErr
}

func (f *fakeCredentialService) GetCredential(_ context.Context, p gitapp.Principal, q gitapp.GetCredential) (gitapp.Envelope[gitapp.CredentialView], error) {
	f.calls = append(f.calls, credentialCall{method: "GetCredential", principal: p, payload: q})
	return f.get, f.getErr
}

func (f *fakeCredentialService) RevokeCredential(_ context.Context, p gitapp.Principal, cmd gitapp.RevokeCredential) (gitapp.Envelope[gitapp.CredentialView], error) {
	f.calls = append(f.calls, credentialCall{method: "RevokeCredential", principal: p, payload: cmd})
	return f.revoke, f.revokeErr
}

func newMCPServerWithAppAndCredentials(t *testing.T, app *fakeApplication, creds *fakeCredentialService) *mcp.Server {
	t.Helper()
	verifier := &fakeVerifier{principal: testPrincipal("tasks:claim", "tasks:execute", "tasks:read", "tasks:publish", "tasks:cancel")}
	s := NewServer(app, verifier, WithCredentialService(creds))
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), verifier.principal))
	return s.mcpServer(req)
}

func TestCredentialIssueToolMapsToService(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	creds := &fakeCredentialService{
		issue: gitapp.Envelope[gitapp.IssueCredentialResponse]{
			Data: gitapp.IssueCredentialResponse{
				Credential: gitapp.CredentialView{ID: "cred-1", ExecutionID: "exe-1"},
				Token:      "tok-1",
			},
			Meta: gitapp.Meta{ServerTime: now},
		},
	}
	server := newMCPServerWithAppAndCredentials(t, &fakeApplication{}, creds)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "credential_issue",
		Arguments: map[string]any{
			"request_id":   "cred-req-1",
			"execution_id": "exe-1",
			"repo":         "owner/repo",
			"base_commit":  "abc",
		},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, creds.calls, 1)
	cmd := creds.calls[0].payload.(gitapp.IssueCredential)
	require.Equal(t, "exe-1", cmd.ExecutionID)
	require.Equal(t, "owner/repo", cmd.Repo)
	require.Equal(t, "abc", cmd.BaseCommit)
	require.Equal(t, "cred-req-1", cmd.RequestID)
}

func TestCredentialGetToolMapsToService(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	creds := &fakeCredentialService{
		get: gitapp.Envelope[gitapp.CredentialView]{
			Data: gitapp.CredentialView{ID: "cred-1", ExecutionID: "exe-1"},
			Meta: gitapp.Meta{ServerTime: now},
		},
	}
	server := newMCPServerWithAppAndCredentials(t, &fakeApplication{}, creds)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "credential_get",
		Arguments: map[string]any{"execution_id": "exe-1"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, creds.calls, 1)
	require.Equal(t, "exe-1", creds.calls[0].payload.(gitapp.GetCredential).ExecutionID)
}

func TestCredentialRevokeToolMapsToService(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	creds := &fakeCredentialService{
		revoke: gitapp.Envelope[gitapp.CredentialView]{
			Data: gitapp.CredentialView{ID: "cred-1", ExecutionID: "exe-1"},
			Meta: gitapp.Meta{ServerTime: now},
		},
	}
	server := newMCPServerWithAppAndCredentials(t, &fakeApplication{}, creds)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	go func() { _, _ = server.Connect(ctx, serverTransport, nil) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "credential_revoke",
		Arguments: map[string]any{"execution_id": "exe-1"},
	})
	require.NoError(t, err)
	require.False(t, res.IsError)
	require.Len(t, creds.calls, 1)
	require.Equal(t, "exe-1", creds.calls[0].payload.(gitapp.RevokeCredential).ExecutionID)
}

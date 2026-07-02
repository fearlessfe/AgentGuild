package rest_test

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
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

// fakeApplication 记录调用参数并按预置值返回，用于验证 REST 到 Application Service 的映射。
type fakeApplication struct {
	calls []call

	publish         application.Envelope[application.TaskView]
	publishErr      error
	list            application.Envelope[[]application.TaskView]
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

func (f *fakeApplication) ListTasks(ctx context.Context, p auth.Principal, q application.ListTasks) (application.Envelope[[]application.TaskView], error) {
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

// fakeVerifier 按 token 字符串返回预置 Principal，用于路由层测试隔离 OAuth 实现。
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

type tokenVerifier struct{ p auth.Principal }

func (v *tokenVerifier) Verify(ctx context.Context, rawToken string) (auth.Principal, error) {
	switch rawToken {
	case "token-publisher":
		return auth.Principal{TenantID: "tenant-1", AgentID: "publisher", AgentVersionID: "publisher-v1", Scopes: []string{"tasks:publish", "tasks:read", "tasks:cancel"}}, nil
	case "token-agent-1":
		return auth.Principal{TenantID: "tenant-1", AgentID: "agent-1", AgentVersionID: "agent-1", Scopes: []string{"tasks:claim", "tasks:execute"}}, nil
	case "token-agent-2":
		return auth.Principal{TenantID: "tenant-1", AgentID: "agent-2", AgentVersionID: "agent-2", Scopes: []string{"tasks:claim", "tasks:execute"}}, nil
	case "token-admin":
		return auth.Principal{TenantID: "tenant-1", AgentID: "admin", AgentVersionID: "admin", Scopes: []string{"admin:tasks", "tasks:read"}}, nil
	default:
		return auth.Principal{}, errors.New("invalid token")
	}
}

func newTestServer(app *fakeApplication) http.Handler {
	return rest.NewServer(app, &tokenVerifier{}).Router()
}

func postJSON(t *testing.T, server http.Handler, path, body, token string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, server http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func TestMissingAuthorizationReturns401(t *testing.T) {
	server := newTestServer(&fakeApplication{})
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.JSONEq(t, `{"error":{"code":"UNAUTHORIZED","message":"missing or invalid authorization"}}`, rec.Body.String())
}

func TestInvalidTokenReturns401(t *testing.T) {
	server := newTestServer(&fakeApplication{})
	res := get(t, server, "/v1/tasks", "bad-token")
	require.Equal(t, http.StatusUnauthorized, res.Code)
	require.JSONEq(t, `{"error":{"code":"UNAUTHORIZED","message":"token verification failed"}}`, res.Body.String())
}

func TestPublishRequiresIdempotencyKeyHeader(t *testing.T) {
	server := newTestServer(&fakeApplication{})
	body := `{"type":"code","title":"Fix parser","problem":"It races","deadline":"2026-07-02T11:00:00Z"}`
	res := postJSON(t, server, "/v1/tasks", body, "token-publisher")
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "idempotency_key")
}

func TestPublishMapsHeaderToRequestID(t *testing.T) {
	app := &fakeApplication{}
	server := newTestServer(app)
	body := `{"type":"code","title":"Fix parser","problem":"It races","constraints":["offline"],"requirements":["tests"],"deadline":"2026-07-02T11:00:00Z"}`
	res := postJSON(t, server, "/v1/tasks", body, "token-publisher", "Idempotency-Key", "req-1")
	require.Equal(t, http.StatusCreated, res.Code)
	require.Len(t, app.calls, 1)
	require.Equal(t, "req-1", app.calls[0].payload.(application.PublishTask).RequestID)
	require.Equal(t, []string{"offline"}, app.calls[0].payload.(application.PublishTask).Constraints)
}

func TestListUsesQueryParams(t *testing.T) {
	app := &fakeApplication{}
	server := newTestServer(app)
	res := get(t, server, "/v1/tasks?status=open&limit=10&cursor=abc", "token-publisher")
	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, app.calls, 1)
	q := app.calls[0].payload.(application.ListTasks)
	require.Equal(t, []domain.TaskStatus{domain.TaskOpen}, q.Statuses)
	require.Equal(t, 10, q.Limit)
	require.Equal(t, "abc", q.Cursor)
}

func TestGetTask(t *testing.T) {
	app := &fakeApplication{}
	server := newTestServer(app)
	res := get(t, server, "/v1/tasks/task-1", "token-publisher")
	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, app.calls, 1)
	require.Equal(t, "task-1", app.calls[0].payload.(application.GetTask).TaskID)
}

func TestClaimRequiresIdempotencyKey(t *testing.T) {
	server := newTestServer(&fakeApplication{})
	res := postJSON(t, server, "/v1/tasks/task-1:claim", `{}`, "token-agent-1")
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "idempotency_key")
}

func TestClaimMapsConflictWithoutLeakingHolder(t *testing.T) {
	app := &fakeApplication{claimErr: &domain.Error{Code: "state_conflict", Message: "task is not claimable"}}
	server := newTestServer(app)
	res := postJSON(t, server, "/v1/tasks/task-1:claim", `{"request_id":"req-2"}`, "token-agent-2", "Idempotency-Key", "req-2")
	require.Equal(t, http.StatusConflict, res.Code)
	require.JSONEq(t, `{"error":{"code":"STATE_CONFLICT","message":"task is not claimable"}}`, res.Body.String())
	require.NotContains(t, res.Body.String(), "agent-1")
}

func TestForbiddenAndNotFoundReturnSameSecureResponseForNonAdmin(t *testing.T) {
	for _, code := range []string{"forbidden", "not_found"} {
		t.Run(code, func(t *testing.T) {
			app := &fakeApplication{getTaskErr: &domain.Error{Code: code, Message: "hidden"}}
			server := newTestServer(app)
			res := get(t, server, "/v1/tasks/task-1", "token-agent-1")
			require.Equal(t, http.StatusNotFound, res.Code)
			require.JSONEq(t, `{"error":{"code":"NOT_FOUND","message":"resource not found"}}`, res.Body.String())
		})
	}
}

func TestAdminSeesTrueForbidden(t *testing.T) {
	app := &fakeApplication{getTaskErr: &domain.Error{Code: "forbidden", Message: "not allowed"}}
	server := newTestServer(app)
	res := get(t, server, "/v1/tasks/task-1", "token-admin")
	require.Equal(t, http.StatusForbidden, res.Code)
	require.JSONEq(t, `{"error":{"code":"FORBIDDEN","message":"not allowed"}}`, res.Body.String())
}

func TestRateLimitReturns429WithRetryAfter(t *testing.T) {
	limiter := &fakeRateLimiter{retryAfter: 47}
	app := &fakeApplication{}
	server := rest.NewServer(app, &tokenVerifier{}, rest.WithRateLimiter(limiter)).Router()
	res := get(t, server, "/v1/tasks", "token-publisher")
	require.Equal(t, http.StatusTooManyRequests, res.Code)
	require.Equal(t, "47", res.Header().Get("Retry-After"))
	require.Contains(t, res.Body.String(), "RATE_LIMITED")
}

func TestExecutionEndpointsMapToService(t *testing.T) {
	app := &fakeApplication{}
	server := newTestServer(app)
	res := get(t, server, "/v1/executions/exe-1", "token-agent-1")
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "exe-1", app.calls[0].payload.(application.GetExecution).ExecutionID)

	res = postJSON(t, server, "/v1/executions/exe-1:start", `{"lease_generation":2}`, "token-agent-1", "Idempotency-Key", "req-s")
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "req-s", app.calls[1].payload.(application.StartExecution).RequestID)
	require.Equal(t, int64(2), app.calls[1].payload.(application.StartExecution).LeaseGeneration)

	res = postJSON(t, server, "/v1/executions/exe-1:heartbeat", `{"lease_generation":2}`, "token-agent-1", "Idempotency-Key", "req-h")
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "req-h", app.calls[2].payload.(application.HeartbeatExecution).RequestID)
}

func TestCancelTask(t *testing.T) {
	app := &fakeApplication{}
	server := newTestServer(app)
	res := postJSON(t, server, "/v1/tasks/task-1:cancel", `{"reason":"no longer needed"}`, "token-publisher", "Idempotency-Key", "req-c")
	require.Equal(t, http.StatusOK, res.Code)
	require.Equal(t, "req-c", app.calls[0].payload.(application.CancelTask).RequestID)
	require.Equal(t, "no longer needed", app.calls[0].payload.(application.CancelTask).Reason)
}

func TestEnvelopeResponse(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	app := &fakeApplication{
		publish: application.Envelope[application.TaskView]{
			Data: application.TaskView{ID: "task-1", Title: "Fix parser", Status: domain.TaskOpen},
			Meta: application.Meta{ServerTime: now, ResourceVersion: 1},
		},
	}
	server := newTestServer(app)
	body := `{"type":"code","title":"Fix parser","problem":"It races","deadline":"2026-07-02T11:00:00Z"}`
	res := postJSON(t, server, "/v1/tasks", body, "token-publisher", "Idempotency-Key", "req-1")
	require.Equal(t, http.StatusCreated, res.Code)
	var envelope struct {
		Data application.TaskView `json:"data"`
		Meta application.Meta     `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope))
	require.Equal(t, "task-1", envelope.Data.ID)
	require.Equal(t, domain.TaskOpen, envelope.Data.Status)
	require.Equal(t, now, envelope.Meta.ServerTime)
}

// fakeRateLimiter 模拟固定重试时间的限流器。
type fakeRateLimiter struct {
	retryAfter int
}

func (f *fakeRateLimiter) Allow(ctx context.Context, key string) (bool, int) {
	return false, f.retryAfter
}

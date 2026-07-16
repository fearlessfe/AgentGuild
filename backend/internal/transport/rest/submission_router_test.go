package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

// fakeSubmissionService records calls and returns pre-set values for REST
// submission route tests.
type fakeSubmissionService struct {
	calls []submissionCall

	create    gitapp.Envelope[gitapp.SubmissionView]
	createErr error
	get       gitapp.Envelope[gitapp.SubmissionView]
	getErr    error
	list      gitapp.Envelope[[]gitapp.SubmissionView]
	listErr   error
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

func (f *fakeSubmissionService) ListSubmissions(_ context.Context, p gitapp.Principal, q gitapp.ListSubmissions) (gitapp.Envelope[[]gitapp.SubmissionView], error) {
	f.calls = append(f.calls, submissionCall{method: "ListSubmissions", principal: p, payload: q})
	return f.list, f.listErr
}

func TestListSubmissionsAuthorizesExecutionFirst(t *testing.T) {
	app := &fakeApplication{getExecution: application.Envelope[application.ExecutionView]{Data: application.ExecutionView{ID: "exe-1", TenantID: "tenant-1"}}}
	sub := &fakeSubmissionService{list: gitapp.Envelope[[]gitapp.SubmissionView]{Data: []gitapp.SubmissionView{{ID: "sub-1", ExecutionID: "exe-1"}}}}
	server := newTestServerWithSubmissions(app, sub)

	res := get(t, server, "/v1/executions/exe-1/submissions", "token-agent-1")
	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, sub.calls, 1)
	require.Equal(t, "exe-1", sub.calls[0].payload.(gitapp.ListSubmissions).ExecutionID)
	require.Len(t, app.calls, 1)
	require.Equal(t, "GetExecution", app.calls[0].method)
}

func newTestServerWithSubmissions(app *fakeApplication, sub *fakeSubmissionService) http.Handler {
	return rest.NewServer(app, &tokenVerifier{}, rest.WithSubmissionService(sub)).Router()
}

func TestCreateSubmissionRequiresIdempotencyKey(t *testing.T) {
	app := &fakeApplication{}
	sub := &fakeSubmissionService{}
	server := newTestServerWithSubmissions(app, sub)

	body := `{"repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix"}`
	res := postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-1")
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "idempotency_key")
	require.Len(t, sub.calls, 0)
}

func TestCreateSubmissionMapsToService(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{
				ID:              "exe-1",
				TaskID:          "task-1",
				TenantID:        "tenant-1",
				AgentVersionID:  "agent-1",
				TaskConstraints: []string{"path:allowed:src/*", "path:forbidden:src/*.env"},
			},
		},
	}
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
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
	server := newTestServerWithSubmissions(app, sub)

	body := `{"repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix parser"}`
	res := postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-1", "Idempotency-Key", "req-1")

	require.Equal(t, http.StatusCreated, res.Code)
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

	var envelope struct {
		Data gitapp.SubmissionView `json:"data"`
		Meta gitapp.Meta           `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope))
	require.Equal(t, "sub-1", envelope.Data.ID)
	require.Equal(t, gitdomain.SubmissionStatusPendingVerification, envelope.Data.Status)
}

func TestCreateSubmissionMapsBodyRequestID(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1"},
		},
	}
	sub := &fakeSubmissionService{}
	server := newTestServerWithSubmissions(app, sub)

	body := `{"request_id":"req-2","repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix"}`
	res := postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-1")
	require.Equal(t, http.StatusCreated, res.Code)
	require.Equal(t, "req-2", sub.calls[0].payload.(gitapp.CreateSubmission).RequestID)
}

func TestCreateSubmissionHidesNonOwnerExecution(t *testing.T) {
	app := &fakeApplication{getExecutionErr: &domain.Error{Code: "not_found", Message: "hidden"}}
	sub := &fakeSubmissionService{}
	server := newTestServerWithSubmissions(app, sub)

	body := `{"request_id":"req-1","repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix"}`
	res := postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-2", "Idempotency-Key", "req-1")
	require.Equal(t, http.StatusNotFound, res.Code)
	require.Len(t, sub.calls, 0)
}

func TestCreateSubmissionMapsDomainErrors(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1"},
		},
	}
	sub := &fakeSubmissionService{createErr: &domain.Error{Code: "state_conflict", Message: "credential revoked"}}
	server := newTestServerWithSubmissions(app, sub)

	body := `{"request_id":"req-1","repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix"}`
	res := postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-1", "Idempotency-Key", "req-1")
	require.Equal(t, http.StatusConflict, res.Code)
	require.JSONEq(t, `{"error":{"code":"STATE_CONFLICT","message":"credential revoked"}}`, res.Body.String())
}

func TestGetSubmissionMapsToServiceAndChecksExecution(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1"},
		},
	}
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
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
	server := newTestServerWithSubmissions(app, sub)

	res := get(t, server, "/v1/submissions/sub-1", "token-agent-1")
	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, sub.calls, 1)
	require.Equal(t, "sub-1", sub.calls[0].payload.(gitapp.GetSubmission).SubmissionID)
	require.Len(t, app.calls, 1)
	require.Equal(t, "exe-1", app.calls[0].payload.(application.GetExecution).ExecutionID)
}

func TestGetSubmissionHidesCrossOwnerExecution(t *testing.T) {
	app := &fakeApplication{getExecutionErr: &domain.Error{Code: "forbidden", Message: "not owner"}}
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	sub := &fakeSubmissionService{
		get: gitapp.Envelope[gitapp.SubmissionView]{
			Data: gitapp.SubmissionView{ID: "sub-1", TenantID: "tenant-1", ExecutionID: "exe-1", CreatedAt: now, UpdatedAt: now},
		},
	}
	server := newTestServerWithSubmissions(app, sub)

	res := get(t, server, "/v1/submissions/sub-1", "token-agent-2")
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestGetSubmissionMapsNotFound(t *testing.T) {
	app := &fakeApplication{}
	sub := &fakeSubmissionService{getErr: &domain.Error{Code: "not_found", Message: "submission not found"}}
	server := newTestServerWithSubmissions(app, sub)

	res := get(t, server, "/v1/submissions/sub-1", "token-agent-1")
	require.Equal(t, http.StatusNotFound, res.Code)
}

func TestCreateSubmissionMapsGitPrincipal(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1"},
		},
	}
	sub := &fakeSubmissionService{}
	server := newTestServerWithSubmissions(app, sub)

	body := `{"request_id":"req-1","repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix"}`
	postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-1", "Idempotency-Key", "req-1")

	p := sub.calls[0].principal
	require.Equal(t, "tenant-1", p.TenantID)
	require.Equal(t, "agent-1", p.AgentID)
	require.Equal(t, "agent-1", p.AgentVersionID)
	require.Equal(t, []string{"tasks:claim", "tasks:execute"}, p.Scopes)
}

func TestSubmissionRoutesRequireAuthentication(t *testing.T) {
	app := &fakeApplication{}
	sub := &fakeSubmissionService{}
	server := newTestServerWithSubmissions(app, sub)

	req := httptest.NewRequest(http.MethodPost, "/v1/executions/exe-1/submissions", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

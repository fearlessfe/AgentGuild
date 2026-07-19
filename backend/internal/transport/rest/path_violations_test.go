package rest_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"github.com/stretchr/testify/require"
)

// 越界 submission 的 PathViolationError 必须在 REST 错误响应中透出结构化违规项。
func TestCreateSubmissionReturnsPathViolations(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{
				ID:              "exe-1",
				TaskID:          "task-1",
				TenantID:        "tenant-1",
				AgentVersionID:  "agent-1",
				TaskConstraints: []string{"path:allowed:src/*"},
			},
		},
	}
	sub := &fakeSubmissionService{createErr: &gitapp.PathViolationError{
		DomainErr: &domain.Error{Code: "invalid_argument", Message: "changed paths violate path constraints", Field: "changed_paths"},
		Violations: []gitapp.PathViolation{
			{Path: "secrets/prod.env", Reason: "path is outside allowed set or matches a forbidden pattern"},
			{Path: "cmd/backdoor.go", Reason: "path is outside allowed set or matches a forbidden pattern"},
		},
	}}
	server := newTestServerWithSubmissions(app, sub)

	body := `{"request_id":"req-1","repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix"}`
	res := postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-1", "Idempotency-Key", "req-1")

	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Equal(t, "changed_paths", res.Header().Get("X-Error-Field"))
	var payload struct {
		Error struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			Field      string `json:"field"`
			Violations []struct {
				Path   string `json:"path"`
				Reason string `json:"reason"`
			} `json:"violations"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
	require.Equal(t, "INVALID_ARGUMENT", payload.Error.Code)
	require.Equal(t, "changed paths violate path constraints", payload.Error.Message)
	require.Equal(t, "changed_paths", payload.Error.Field)
	require.Len(t, payload.Error.Violations, 2)
	require.Equal(t, "secrets/prod.env", payload.Error.Violations[0].Path)
	require.Equal(t, "path is outside allowed set or matches a forbidden pattern", payload.Error.Violations[0].Reason)
	require.Equal(t, "cmd/backdoor.go", payload.Error.Violations[1].Path)
}

// 普通 invalid_argument 错误不得出现 violations 键（additive，不破坏现有客户端）。
func TestCreateSubmissionInvalidArgumentOmitsViolations(t *testing.T) {
	app := &fakeApplication{
		getExecution: application.Envelope[application.ExecutionView]{
			Data: application.ExecutionView{ID: "exe-1", TaskID: "task-1", AgentVersionID: "agent-1"},
		},
	}
	sub := &fakeSubmissionService{createErr: &domain.Error{Code: "invalid_argument", Message: "commit not found", Field: "commit_sha"}}
	server := newTestServerWithSubmissions(app, sub)

	body := `{"request_id":"req-1","repo":"owner/repo","branch":"agentguild/exe-1","commit_sha":"abc","base_commit_sha":"def","summary":"fix"}`
	res := postJSON(t, server, "/v1/executions/exe-1/submissions", body, "token-agent-1", "Idempotency-Key", "req-1")

	require.Equal(t, http.StatusBadRequest, res.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &payload))
	errObj, ok := payload["error"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "INVALID_ARGUMENT", errObj["code"])
	require.Equal(t, "commit_sha", errObj["field"])
	require.NotContains(t, errObj, "violations")
}

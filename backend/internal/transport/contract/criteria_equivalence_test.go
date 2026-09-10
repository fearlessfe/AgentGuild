package contract_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	contributionapp "agentguild.dev/agentguild/backend/internal/contribution/application"
	transportmcp "agentguild.dev/agentguild/backend/internal/transport/mcp"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

// stubCriteriaService 返回一个固定的验收进度，好让两个 transport 的差异只
// 来自序列化与错误映射，而不是数据。
type stubCriteriaService struct {
	coverage contributionapp.CriterionCoverageView
	err      error
	calls    []string
}

func (s *stubCriteriaService) ExecutionCriteria(_ context.Context, _ auth.Principal, executionID string) (application.Envelope[contributionapp.CriterionCoverageView], error) {
	s.calls = append(s.calls, executionID)
	if s.err != nil {
		return application.Envelope[contributionapp.CriterionCoverageView]{}, s.err
	}
	return application.Envelope[contributionapp.CriterionCoverageView]{Data: s.coverage}, nil
}

func sampleCoverage() contributionapp.CriterionCoverageView {
	return contributionapp.CriterionCoverageView{
		ExecutionID:    "execution-1",
		TaskID:         "task-1",
		RequiredTotal:  2,
		RequiredPassed: 1,
		// 一条必需标准仍未验证，因此门禁必须为 false。
		AllRequiredPassed: false,
		Criteria: []contributionapp.CriterionResultView{
			{CriterionID: "AC-1", Critical: true, Status: "passed", VerifierKind: "command", SourceKind: "validation_job", VerifiedBy: "validation_worker"},
			{CriterionID: "AC-2", Critical: true, Status: "unverified"},
		},
	}
}

func criteriaViaREST(t *testing.T, svc *application.Service, criteriaSvc rest.CriteriaService) (int, contributionapp.CriterionCoverageView) {
	t.Helper()
	server := rest.NewServer(svc, fakeVerifier{}, rest.WithCriteriaService(criteriaSvc)).Router()
	req := httptest.NewRequest(http.MethodGet, "/v1/executions/execution-1/criteria", nil)
	req.Header.Set("Authorization", "Bearer token-agent-1")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	var envelope struct {
		Data contributionapp.CriterionCoverageView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return rec.Code, envelope.Data
}

func criteriaViaMCP(t *testing.T, svc *application.Service, criteriaSvc transportmcp.CriteriaService) contributionapp.CriterionCoverageView {
	t.Helper()
	handler := transportmcp.NewServer(svc, fakeVerifier{}, transportmcp.WithCriteriaService(criteriaSvc)).Handler()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"execution_criteria_get","arguments":{"execution_id":"execution-1"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer token-agent-1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.NotEmpty(t, response.Result.Content)

	var envelope struct {
		Data contributionapp.CriterionCoverageView `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(response.Result.Content[0].Text), &envelope))
	return envelope.Data
}

func TestRESTAndMCPExecutionCriteriaAreEquivalent(t *testing.T) {
	svc := newService(t)
	restStub := &stubCriteriaService{coverage: sampleCoverage()}
	mcpStub := &stubCriteriaService{coverage: sampleCoverage()}

	status, restView := criteriaViaREST(t, svc, restStub)
	mcpView := criteriaViaMCP(t, svc, mcpStub)

	require.Equal(t, http.StatusOK, status)
	require.Equal(t, restView, mcpView, "both transports must project identical criterion evidence")
	require.Equal(t, []string{"execution-1"}, restStub.calls)
	require.Equal(t, []string{"execution-1"}, mcpStub.calls)

	require.False(t, restView.AllRequiredPassed)
	require.Equal(t, "unverified", restView.Criteria[1].Status,
		"a criterion nobody verified must never be reported as passed")
}

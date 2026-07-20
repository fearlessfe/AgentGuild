package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	evaluationapp "agentguild.dev/agentguild/backend/internal/evaluation/application"
	evaldomain "agentguild.dev/agentguild/backend/internal/evaluation/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

type fakeEvaluationService struct {
	listDetail      []evaluationapp.EvaluationRunDetail
	listDetailErr   error
	listDetailCalls []struct {
		tenantID       string
		agentVersionID string
	}

	detail      *evaluationapp.EvaluationRunDetail
	detailErr   error
	detailCalls []struct {
		tenantID string
		id       string
	}

	createCmd evaluationapp.CreateBenchmarkSet
}

func (f *fakeEvaluationService) ListBenchmarkSetSummaries(ctx context.Context, principal identityapp.Principal, tenantID string) ([]evaluationapp.BenchmarkSetSummary, error) {
	return nil, nil
}

func (f *fakeEvaluationService) GetBenchmarkSetSummary(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*evaluationapp.BenchmarkSetSummary, error) {
	return nil, nil
}

func (f *fakeEvaluationService) CreateBenchmarkSet(ctx context.Context, cmd evaluationapp.CreateBenchmarkSet) (*evaluationapp.CreateBenchmarkSetResponse, error) {
	f.createCmd = cmd
	bs, err := evaldomain.NewBenchmarkSet("bs-1", cmd.TenantID, cmd.CreatedBy, 1)
	if err != nil {
		return nil, err
	}
	return &evaluationapp.CreateBenchmarkSetResponse{BenchmarkSet: bs}, nil
}

func (f *fakeEvaluationService) ListEvaluationRunDetails(ctx context.Context, principal identityapp.Principal, tenantID, agentVersionID string) ([]evaluationapp.EvaluationRunDetail, error) {
	f.listDetailCalls = append(f.listDetailCalls, struct {
		tenantID       string
		agentVersionID string
	}{tenantID: tenantID, agentVersionID: agentVersionID})
	return f.listDetail, f.listDetailErr
}

func (f *fakeEvaluationService) GetEvaluationRunDetail(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*evaluationapp.EvaluationRunDetail, error) {
	f.detailCalls = append(f.detailCalls, struct {
		tenantID string
		id       string
	}{tenantID: tenantID, id: id})
	return f.detail, f.detailErr
}

func (f *fakeEvaluationService) StartEvaluationRun(ctx context.Context, cmd evaluationapp.StartEvaluationRun) (*evaluationapp.StartEvaluationRunResponse, error) {
	return nil, nil
}

func TestListEvaluationsReturnsSnakeCaseFields(t *testing.T) {
	started := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	completed := started.Add(time.Hour)
	evals := &fakeEvaluationService{
		listDetail: []evaluationapp.EvaluationRunDetail{
			{
				ID:                 "run-1",
				TenantID:           "tenant-1",
				AgentVersionID:     "av-1",
				BenchmarkSetID:     "bs-1",
				Status:             "passed",
				EnvironmentDigest:  "env-1",
				ScoringRuleVersion: "v1",
				Summary: evaldomain.EvaluationSummary{
					PassRate: 1.0,
				},
				StartedAt:   started,
				CompletedAt: &completed,
			},
		},
	}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithSession(testSessionSecret, false),
		rest.WithEvaluationService(evals),
	).Router()

	res := getWithSession(t, server, "/v1/evaluations?agent_version_id=av-1", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, evals.listDetailCalls, 1)
	require.Equal(t, "tenant-1", evals.listDetailCalls[0].tenantID)
	require.Equal(t, "av-1", evals.listDetailCalls[0].agentVersionID)

	body := res.Body.String()
	require.Contains(t, body, `"data":`)
	require.Contains(t, body, `"items":`)
	require.Contains(t, body, `"agent_version_id":`)
	require.Contains(t, body, `"benchmark_set_id":`)
	require.Contains(t, body, `"environment_digest":`)
	require.Contains(t, body, `"scoring_rule_version":`)
	require.Contains(t, body, `"started_at":`)
	require.Contains(t, body, `"completed_at":`)
	require.Contains(t, body, `"summary":`)
	require.Contains(t, body, `"pass_rate":`)
	require.NotContains(t, body, `"AgentVersionID"`)
	require.NotContains(t, body, `"BenchmarkSetID"`)
	require.NotContains(t, body, `"EnvironmentDigest"`)
	require.NotContains(t, body, `"ScoringRuleVersion"`)
	require.NotContains(t, body, `"StartedAt"`)
	require.NotContains(t, body, `"CompletedAt"`)

	var envelope struct {
		Data struct {
			Items []evaluationapp.EvaluationRunDetail `json:"items"`
		} `json:"data"`
		Meta struct {
			ServerTime time.Time `json:"server_time"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope))
	require.Len(t, envelope.Data.Items, 1)
	require.Equal(t, "run-1", envelope.Data.Items[0].ID)
	require.Equal(t, 1.0, envelope.Data.Items[0].Summary.PassRate)
	require.NotZero(t, envelope.Meta.ServerTime)
}

func TestGetEvaluationReturnsDetailWithThresholdsAndSummary(t *testing.T) {
	started := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	completed := started.Add(time.Hour)
	evals := &fakeEvaluationService{
		detail: &evaluationapp.EvaluationRunDetail{
			ID:                 "run-1",
			TenantID:           "tenant-1",
			AgentVersionID:     "av-1",
			BenchmarkSetID:     "bs-1",
			Status:             "passed",
			EnvironmentDigest:  "env-1",
			ScoringRuleVersion: "v1",
			ThresholdResults: []evaldomain.ThresholdResult{
				{Name: "security", Passed: true, Evidence: map[string]any{"tool": "trivy"}},
			},
			TaskResults: []evaldomain.EvaluationRunResult{
				{EvaluationRunID: "run-1", TenantID: "tenant-1", TaskRef: "task-1", Score: 1.0, Passed: true, Details: map[string]any{"resolution": "validated"}},
				{EvaluationRunID: "run-1", TenantID: "tenant-1", TaskRef: "task-2", Score: 0.0, Passed: false},
			},
			Summary: evaldomain.EvaluationSummary{
				PassRate:       1.0,
				AvgLatencyMs:   100.0,
				CostCents:      50,
				SecurityPassed: true,
				Extra:          map[string]any{"coverage": 0.9},
			},
			StartedAt:   started,
			CompletedAt: &completed,
		},
	}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithSession(testSessionSecret, false),
		rest.WithEvaluationService(evals),
	).Router()

	res := getWithSession(t, server, "/v1/evaluations/run-1", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, evals.detailCalls, 1)
	require.Equal(t, "run-1", evals.detailCalls[0].id)

	body := res.Body.String()
	require.Contains(t, body, `"data":`)
	require.Contains(t, body, `"threshold_results":`)
	require.Contains(t, body, `"summary":`)
	require.Contains(t, body, `"pass_rate":`)
	require.Contains(t, body, `"avg_latency_ms":`)
	require.Contains(t, body, `"cost_cents":`)
	require.Contains(t, body, `"security_passed":`)
	require.Contains(t, body, `"task_results":`)
	require.Contains(t, body, `"task_ref":`)
	require.Contains(t, body, `"score":`)
	require.Contains(t, body, `"passed":`)
	require.NotContains(t, body, `"ThresholdResults"`)
	require.NotContains(t, body, `"PassRate"`)
	require.NotContains(t, body, `"AvgLatencyMs"`)
	require.NotContains(t, body, `"CostCents"`)
	require.NotContains(t, body, `"TaskResults"`)
	require.NotContains(t, body, `"TaskRef"`)

	var envelope struct {
		Data evaluationapp.EvaluationRunDetail `json:"data"`
		Meta struct {
			ServerTime time.Time `json:"server_time"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &envelope))
	require.Equal(t, "run-1", envelope.Data.ID)
	require.Equal(t, "tenant-1", envelope.Data.TenantID)
	require.Equal(t, "av-1", envelope.Data.AgentVersionID)
	require.Equal(t, "bs-1", envelope.Data.BenchmarkSetID)
	require.Equal(t, "passed", envelope.Data.Status)
	require.Len(t, envelope.Data.ThresholdResults, 1)
	require.Equal(t, "security", envelope.Data.ThresholdResults[0].Name)
	require.True(t, envelope.Data.ThresholdResults[0].Passed)
	require.Len(t, envelope.Data.TaskResults, 2)
	require.Equal(t, "task-1", envelope.Data.TaskResults[0].TaskRef)
	require.Equal(t, 1.0, envelope.Data.TaskResults[0].Score)
	require.True(t, envelope.Data.TaskResults[0].Passed)
	require.Equal(t, "validated", envelope.Data.TaskResults[0].Details["resolution"])
	require.Equal(t, "task-2", envelope.Data.TaskResults[1].TaskRef)
	require.False(t, envelope.Data.TaskResults[1].Passed)
	require.Equal(t, 1.0, envelope.Data.Summary.PassRate)
	require.Equal(t, 100.0, envelope.Data.Summary.AvgLatencyMs)
	require.Equal(t, int64(50), envelope.Data.Summary.CostCents)
	require.True(t, envelope.Data.Summary.SecurityPassed)
	require.NotNil(t, envelope.Data.CompletedAt)
	require.Equal(t, completed, *envelope.Data.CompletedAt)
	require.NotZero(t, envelope.Meta.ServerTime)
}

func TestCreateBenchmarkAcceptsRefsAndTaskDefinitions(t *testing.T) {
	evals := &fakeEvaluationService{}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithSession(testSessionSecret, false),
		rest.WithEvaluationService(evals),
	).Router()

	body := `{
		"name": "Set",
		"tasks": [
			"task-legacy",
			{"task_ref": "task-1", "title": "Fix the bug", "problem": "broken", "constraints": ["repo:org/repo", "base_commit:abc"], "requirements": ["tests pass"], "is_security": true}
		],
		"is_active": false
	}`
	res := postJSONWithSession(t, server, "/v1/benchmarks", body, sessionCookie(t, "owner-1", true))

	require.Equal(t, http.StatusCreated, res.Code)
	require.Len(t, evals.createCmd.Tasks, 2)

	legacy := evals.createCmd.Tasks[0]
	require.Equal(t, "task-legacy", legacy.TaskRef)
	require.Equal(t, 0, legacy.Ordering)
	require.Equal(t, "", legacy.Title)
	require.False(t, legacy.IsSecurity)

	def := evals.createCmd.Tasks[1]
	require.Equal(t, "task-1", def.TaskRef)
	require.Equal(t, 1, def.Ordering)
	require.Equal(t, "Fix the bug", def.Title)
	require.Equal(t, "broken", def.Problem)
	require.Equal(t, []string{"repo:org/repo", "base_commit:abc"}, def.Constraints)
	require.Equal(t, []string{"tests pass"}, def.Requirements)
	require.True(t, def.IsSecurity)

	res = postJSONWithSession(t, server, "/v1/benchmarks", `{"name":"Set","tasks":[42]}`, sessionCookie(t, "owner-1", true))
	require.Equal(t, http.StatusBadRequest, res.Code)
}

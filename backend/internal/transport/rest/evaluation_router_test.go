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
	list      []evaluationapp.EvaluationRunSummary
	listErr   error
	listCalls []struct {
		tenantID       string
		agentVersionID string
	}

	detail      *evaluationapp.EvaluationRunDetail
	detailErr   error
	detailCalls []struct {
		tenantID string
		id       string
	}
}

func (f *fakeEvaluationService) ListBenchmarkSetSummaries(ctx context.Context, principal identityapp.Principal, tenantID string) ([]evaluationapp.BenchmarkSetSummary, error) {
	return nil, nil
}

func (f *fakeEvaluationService) GetBenchmarkSetSummary(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*evaluationapp.BenchmarkSetSummary, error) {
	return nil, nil
}

func (f *fakeEvaluationService) CreateBenchmarkSet(ctx context.Context, cmd evaluationapp.CreateBenchmarkSet) (*evaluationapp.CreateBenchmarkSetResponse, error) {
	return nil, nil
}

func (f *fakeEvaluationService) ListEvaluationRunSummaries(ctx context.Context, principal identityapp.Principal, tenantID, agentVersionID string) ([]evaluationapp.EvaluationRunSummary, error) {
	f.listCalls = append(f.listCalls, struct {
		tenantID       string
		agentVersionID string
	}{tenantID: tenantID, agentVersionID: agentVersionID})
	return f.list, f.listErr
}

func (f *fakeEvaluationService) GetEvaluationRunSummary(ctx context.Context, principal identityapp.Principal, tenantID, id string) (*evaluationapp.EvaluationRunSummary, error) {
	return nil, nil
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
		list: []evaluationapp.EvaluationRunSummary{
			{
				ID:                 "run-1",
				TenantID:           "tenant-1",
				AgentVersionID:     "av-1",
				BenchmarkSetID:     "bs-1",
				Status:             "passed",
				EnvironmentDigest:  "env-1",
				ScoringRuleVersion: "v1",
				StartedAt:          started,
				CompletedAt:        &completed,
			},
		},
	}
	server := rest.NewServer(&fakeApplication{}, &tokenVerifier{},
		rest.WithSession(testSessionSecret, false),
		rest.WithEvaluationService(evals),
	).Router()

	res := getWithSession(t, server, "/v1/evaluations?agent_version_id=av-1", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	require.Len(t, evals.listCalls, 1)
	require.Equal(t, "tenant-1", evals.listCalls[0].tenantID)
	require.Equal(t, "av-1", evals.listCalls[0].agentVersionID)

	body := res.Body.String()
	require.Contains(t, body, `"agent_version_id":`)
	require.Contains(t, body, `"benchmark_set_id":`)
	require.Contains(t, body, `"environment_digest":`)
	require.Contains(t, body, `"scoring_rule_version":`)
	require.Contains(t, body, `"started_at":`)
	require.Contains(t, body, `"completed_at":`)
	require.NotContains(t, body, `"AgentVersionID"`)
	require.NotContains(t, body, `"BenchmarkSetID"`)
	require.NotContains(t, body, `"EnvironmentDigest"`)
	require.NotContains(t, body, `"ScoringRuleVersion"`)
	require.NotContains(t, body, `"StartedAt"`)
	require.NotContains(t, body, `"CompletedAt"`)
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
	require.Contains(t, body, `"threshold_results":`)
	require.Contains(t, body, `"summary":`)
	require.Contains(t, body, `"pass_rate":`)
	require.Contains(t, body, `"avg_latency_ms":`)
	require.Contains(t, body, `"cost_cents":`)
	require.Contains(t, body, `"security_passed":`)
	require.NotContains(t, body, `"ThresholdResults"`)
	require.NotContains(t, body, `"PassRate"`)
	require.NotContains(t, body, `"AvgLatencyMs"`)
	require.NotContains(t, body, `"CostCents"`)

	var parsed evaluationapp.EvaluationRunDetail
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &parsed))
	require.Equal(t, "run-1", parsed.ID)
	require.Equal(t, "tenant-1", parsed.TenantID)
	require.Equal(t, "av-1", parsed.AgentVersionID)
	require.Equal(t, "bs-1", parsed.BenchmarkSetID)
	require.Equal(t, "passed", parsed.Status)
	require.Len(t, parsed.ThresholdResults, 1)
	require.Equal(t, "security", parsed.ThresholdResults[0].Name)
	require.True(t, parsed.ThresholdResults[0].Passed)
	require.Equal(t, 1.0, parsed.Summary.PassRate)
	require.Equal(t, 100.0, parsed.Summary.AvgLatencyMs)
	require.Equal(t, int64(50), parsed.Summary.CostCents)
	require.True(t, parsed.Summary.SecurityPassed)
	require.NotNil(t, parsed.CompletedAt)
	require.Equal(t, completed, *parsed.CompletedAt)
}

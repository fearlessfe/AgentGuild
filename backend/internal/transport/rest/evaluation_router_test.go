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

	var body evaluationapp.EvaluationRunDetail
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "run-1", body.ID)
	require.Equal(t, "tenant-1", body.TenantID)
	require.Equal(t, "av-1", body.AgentVersionID)
	require.Equal(t, "bs-1", body.BenchmarkSetID)
	require.Equal(t, "passed", body.Status)
	require.Len(t, body.ThresholdResults, 1)
	require.Equal(t, "security", body.ThresholdResults[0].Name)
	require.True(t, body.ThresholdResults[0].Passed)
	require.Equal(t, 1.0, body.Summary.PassRate)
	require.Equal(t, 100.0, body.Summary.AvgLatencyMs)
	require.Equal(t, int64(50), body.Summary.CostCents)
	require.True(t, body.Summary.SecurityPassed)
	require.NotNil(t, body.CompletedAt)
	require.Equal(t, completed, *body.CompletedAt)
}

type fakeEvaluationService struct {
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
	return nil, nil
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

package domain_test

import (
	"testing"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/stretchr/testify/require"
)

func TestApplyScoringRulePassed(t *testing.T) {
	taskResults := []domain.TaskResult{
		{TaskRef: "task-1", Passed: true, LatencyMs: 100, Score: 1.0, IsSecurity: true},
		{TaskRef: "task-2", Passed: true, LatencyMs: 200, Score: 1.0},
		{TaskRef: "task-3", Passed: true, LatencyMs: 300, Score: 1.0},
		{TaskRef: "task-4", Passed: false, LatencyMs: 400, Score: 0.0},
		{TaskRef: "task-5", Passed: true, LatencyMs: 500, Score: 1.0},
	}

	thresholds, summary := domain.ApplyScoringRule(taskResults, domain.ScoringRuleVersionV1)
	require.True(t, summary.PassRate >= 0.79)
	require.True(t, summary.AvgLatencyMs <= 1000)

	passed := map[string]bool{}
	for _, th := range thresholds {
		passed[th.Name] = th.Passed
	}
	require.True(t, passed["security_regression"])
	require.True(t, passed["pass_rate"])
	require.True(t, passed["avg_latency"])
	require.True(t, domain.IsAllPassed(thresholds))
}

func TestApplyScoringRuleFailedSecurityRegression(t *testing.T) {
	taskResults := []domain.TaskResult{
		{TaskRef: "task-1", Passed: false, LatencyMs: 100, Score: 0.0, IsSecurity: true},
		{TaskRef: "task-2", Passed: true, LatencyMs: 200, Score: 1.0},
	}

	thresholds, summary := domain.ApplyScoringRule(taskResults, domain.ScoringRuleVersionV1)
	require.False(t, domain.IsAllPassed(thresholds))
	require.False(t, summary.SecurityPassed)
}

func TestApplyScoringRuleFailedPassRate(t *testing.T) {
	taskResults := []domain.TaskResult{
		{TaskRef: "task-1", Passed: true, LatencyMs: 100, Score: 1.0},
		{TaskRef: "task-2", Passed: false, LatencyMs: 200, Score: 0.0},
		{TaskRef: "task-3", Passed: false, LatencyMs: 300, Score: 0.0},
		{TaskRef: "task-4", Passed: false, LatencyMs: 400, Score: 0.0},
	}

	thresholds, _ := domain.ApplyScoringRule(taskResults, domain.ScoringRuleVersionV1)
	require.False(t, domain.IsAllPassed(thresholds))
}

func TestApplyScoringRuleFailedLatency(t *testing.T) {
	taskResults := []domain.TaskResult{
		{TaskRef: "task-1", Passed: true, LatencyMs: 2000, Score: 1.0},
		{TaskRef: "task-2", Passed: true, LatencyMs: 2000, Score: 1.0},
	}

	thresholds, _ := domain.ApplyScoringRule(taskResults, domain.ScoringRuleVersionV1)
	require.False(t, domain.IsAllPassed(thresholds))
}

func TestApplyScoringRuleUnknownVersion(t *testing.T) {
	thresholds, summary := domain.ApplyScoringRule([]domain.TaskResult{}, "v-unknown")
	require.Empty(t, thresholds)
	require.Equal(t, 0.0, summary.PassRate)
	require.False(t, domain.IsKnownScoringRuleVersion("v-unknown"))
	require.True(t, domain.IsKnownScoringRuleVersion(domain.ScoringRuleVersionV1))
}

func TestApplyScoringRulePopulatesCostCents(t *testing.T) {
	thresholds, summary := domain.ApplyScoringRule([]domain.TaskResult{
		{TaskRef: "task-1", Passed: true, LatencyMs: 100, Score: 1.0, CostCents: 5},
		{TaskRef: "task-2", Passed: true, LatencyMs: 200, Score: 1.0, CostCents: 7},
	}, domain.ScoringRuleVersionV1)
	require.NotEmpty(t, thresholds)
	require.Equal(t, int64(12), summary.CostCents)
}

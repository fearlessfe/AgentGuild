package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/stretchr/testify/require"
)

func passedThresholds() []domain.ThresholdResult {
	return []domain.ThresholdResult{
		{Name: "security_regression", Passed: true, Evidence: map[string]any{"security_tasks_count": 2}},
		{Name: "pass_rate", Passed: true, Evidence: map[string]any{"threshold": 0.8, "actual": 0.9}},
		{Name: "avg_latency", Passed: true, Evidence: map[string]any{"threshold_ms": 1000.0, "actual_ms": 500.0}},
	}
}

func failedThresholds() []domain.ThresholdResult {
	return []domain.ThresholdResult{
		{Name: "security_regression", Passed: true, Evidence: map[string]any{"security_tasks_count": 2}},
		{Name: "pass_rate", Passed: false, Evidence: map[string]any{"threshold": 0.8, "actual": 0.6}},
		{Name: "avg_latency", Passed: true, Evidence: map[string]any{"threshold_ms": 1000.0, "actual_ms": 500.0}},
	}
}

func summary() domain.EvaluationSummary {
	return domain.EvaluationSummary{
		PassRate:     0.9,
		AvgLatencyMs: 500.0,
		CostCents:    123,
		Extra: map[string]any{
			"tasks_run": 10,
		},
	}
}

// NewTestEvaluationRun returns a running evaluation run for tests.
func NewTestEvaluationRun(t *testing.T) *domain.EvaluationRun {
	t.Helper()
	run, err := domain.NewEvaluationRun(
		"er-1", "tenant-1", "av-1", "bs-1",
		"env-digest", domain.ScoringRuleVersionV1,
		time.Now(),
	)
	require.NoError(t, err)
	return run
}

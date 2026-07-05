package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
	"github.com/stretchr/testify/require"
)

func TestEvaluationRunFreeze(t *testing.T) {
	run := NewTestEvaluationRun(t)
	require.Equal(t, domain.StatusRunning, run.Status())
	require.Equal(t, "av-1", run.AgentVersionID())
}

func TestEvaluationRunCompletePassed(t *testing.T) {
	run := NewTestEvaluationRun(t)
	require.NoError(t, run.Complete(passedThresholds(), summary()))
	require.Equal(t, domain.StatusPassed, run.Status())
	require.True(t, run.IsPassed())
	require.NotNil(t, run.CompletedAt())
}

func TestEvaluationRunCompleteFailed(t *testing.T) {
	run := NewTestEvaluationRun(t)
	require.NoError(t, run.Complete(failedThresholds(), summary()))
	require.Equal(t, domain.StatusFailed, run.Status())
	require.False(t, run.IsPassed())
}

func TestEvaluationRunCompleteOnlyWhenRunning(t *testing.T) {
	run := NewTestEvaluationRun(t)
	require.NoError(t, run.Complete(passedThresholds(), summary()))
	err := run.Complete(passedThresholds(), summary())
	require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestEvaluationRunIsPassedWithNoThresholds(t *testing.T) {
	run := NewTestEvaluationRun(t)
	require.NoError(t, run.Complete([]domain.ThresholdResult{}, summary()))
	require.Equal(t, domain.StatusPassed, run.Status())
	require.True(t, run.IsPassed())
}

func TestNewEvaluationRunValidatesInputs(t *testing.T) {
	now := time.Now()
	_, err := domain.NewEvaluationRun("", "tenant-1", "av-1", "bs-1", "env", domain.ScoringRuleVersionV1, now)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = domain.NewEvaluationRun("er-1", "", "av-1", "bs-1", "env", domain.ScoringRuleVersionV1, now)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = domain.NewEvaluationRun("er-1", "tenant-1", "", "bs-1", "env", domain.ScoringRuleVersionV1, now)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = domain.NewEvaluationRun("er-1", "tenant-1", "av-1", "", "env", domain.ScoringRuleVersionV1, now)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = domain.NewEvaluationRun("er-1", "tenant-1", "av-1", "bs-1", "", domain.ScoringRuleVersionV1, now)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)

	_, err = domain.NewEvaluationRun("er-1", "tenant-1", "av-1", "bs-1", "env", "", now)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
}

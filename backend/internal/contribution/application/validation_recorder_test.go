package application_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/application"
	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	"github.com/stretchr/testify/require"
)

type stubCriterionRepository struct {
	recorded []domain.CriterionResult
}

func (s *stubCriterionRepository) Record(_ context.Context, result *domain.CriterionResult) (*domain.CriterionResult, bool, error) {
	for _, existing := range s.recorded {
		if existing.ExecutionID == result.ExecutionID &&
			existing.CriterionID == result.CriterionID &&
			existing.SourceKind == result.SourceKind &&
			existing.SourceID == result.SourceID {
			return &existing, false, nil
		}
	}
	stored := *result
	stored.ID = int64(len(s.recorded) + 1)
	s.recorded = append(s.recorded, stored)
	return &stored, true, nil
}

func (s *stubCriterionRepository) ListLatest(context.Context, string, string) ([]domain.CriterionResult, error) {
	return s.recorded, nil
}

func (s *stubCriterionRepository) ListHistory(context.Context, string, string) ([]domain.CriterionResult, error) {
	return s.recorded, nil
}

type stubCriteriaSource struct {
	criteria application.TaskCriteria
}

func (s stubCriteriaSource) CriteriaForExecution(context.Context, string, string) (application.TaskCriteria, error) {
	return s.criteria, nil
}

func newValidationRecorder(t *testing.T, source application.TaskCriteriaSource) (*application.ValidationCriterionRecorder, *stubCriterionRepository) {
	t.Helper()
	repository := &stubCriterionRepository{}
	recorder, err := application.NewCriterionRecorder(repository, application.CriterionRecorderOptions{})
	require.NoError(t, err)
	validation, err := application.NewValidationCriterionRecorder(recorder, source)
	require.NoError(t, err)
	return validation, repository
}

func TestValidationRecorderOnlyJudgesCriteriaBoundToStepsThatRan(t *testing.T) {
	source := stubCriteriaSource{criteria: application.TaskCriteria{
		TaskID: "task-1",
		Criteria: []application.CriterionSpec{
			{ID: "AC-1", Critical: true, VerifierKind: "command", VerifierRef: "public_tests", Automated: true},
			{ID: "AC-2", Critical: true, VerifierKind: "command", VerifierRef: "security_scan", Automated: true},
			// 绑定到一个本次没有运行的步骤。
			{ID: "AC-3", Critical: true, VerifierKind: "command", VerifierRef: "hidden_tests", Automated: true},
			// 没有绑定验证步骤，只能人工判定。
			{ID: "AC-4", Critical: true, VerifierKind: "manual", Automated: false},
		},
	}}
	recorder, repository := newValidationRecorder(t, source)

	written, err := recorder.RecordValidationOutcome(context.Background(), application.ValidationOutcome{
		TenantID:      "tenant-sponsor",
		ExecutionID:   "execution-1",
		JobID:         "validation-1",
		StepResults:   map[string]bool{"public_tests": true, "security_scan": false},
		ObservedAt:    time.Now().UTC(),
		ConfigVersion: "config-7",
	})

	require.NoError(t, err)
	require.Equal(t, 2, written)
	require.Len(t, repository.recorded, 2)
	require.Equal(t, "AC-1", repository.recorded[0].CriterionID)
	require.True(t, repository.recorded[0].Passed)
	require.Equal(t, "AC-2", repository.recorded[1].CriterionID)
	require.False(t, repository.recorded[1].Passed)

	// AC-3 与 AC-4 必须停留在“未验证”，而不是被默认为通过。
	coverage := domain.SummarizeCriteria([]domain.SpecCriterion{
		{ID: "AC-1", Critical: true}, {ID: "AC-2", Critical: true},
		{ID: "AC-3", Critical: true}, {ID: "AC-4", Critical: true},
	}, repository.recorded)
	require.ElementsMatch(t, []string{"AC-3", "AC-4"}, coverage.Unverified)
	require.Equal(t, []string{"AC-2"}, coverage.FailedRequired)
	require.False(t, coverage.AllRequiredPassed())
}

func TestValidationRecorderIsIdempotentPerJob(t *testing.T) {
	source := stubCriteriaSource{criteria: application.TaskCriteria{
		TaskID:   "task-1",
		Criteria: []application.CriterionSpec{{ID: "AC-1", Critical: true, VerifierKind: "command", VerifierRef: "build", Automated: true}},
	}}
	recorder, repository := newValidationRecorder(t, source)
	outcome := application.ValidationOutcome{
		TenantID:    "tenant-sponsor",
		ExecutionID: "execution-1",
		JobID:       "validation-1",
		StepResults: map[string]bool{"build": true},
		ObservedAt:  time.Now().UTC(),
	}

	first, err := recorder.RecordValidationOutcome(context.Background(), outcome)
	require.NoError(t, err)
	require.Equal(t, 1, first)

	second, err := recorder.RecordValidationOutcome(context.Background(), outcome)
	require.NoError(t, err)
	require.Zero(t, second, "redelivering the same validation job must not append a second fact")
	require.Len(t, repository.recorded, 1)
}

func TestValidationRecorderSkipsExecutionsWithoutAPublicSpec(t *testing.T) {
	recorder, repository := newValidationRecorder(t, stubCriteriaSource{})

	written, err := recorder.RecordValidationOutcome(context.Background(), application.ValidationOutcome{
		TenantID:    "tenant-sponsor",
		ExecutionID: "execution-1",
		JobID:       "validation-1",
		StepResults: map[string]bool{"build": true},
		ObservedAt:  time.Now().UTC(),
	})

	require.NoError(t, err)
	require.Zero(t, written)
	require.Empty(t, repository.recorded)
}

package criteria

import (
	"context"

	contributionapp "agentguild.dev/agentguild/backend/internal/contribution/application"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
)

// ValidationSink 把 validation worker 的终态转交给 contribution 的 criterion
// 记录器，满足 gitapp.CriterionSink，从而让 git 模块无需认识 contribution。
type ValidationSink struct {
	recorder *contributionapp.ValidationCriterionRecorder
}

func NewValidationSink(recorder *contributionapp.ValidationCriterionRecorder) *ValidationSink {
	return &ValidationSink{recorder: recorder}
}

func (s *ValidationSink) RecordValidationOutcome(ctx context.Context, outcome gitapp.ValidationCriterionOutcome) error {
	if s == nil || s.recorder == nil {
		return nil
	}
	_, err := s.recorder.RecordValidationOutcome(ctx, contributionapp.ValidationOutcome{
		TenantID:      outcome.TenantID,
		ExecutionID:   outcome.ExecutionID,
		JobID:         outcome.JobID,
		StepResults:   outcome.StepResults,
		ObservedAt:    outcome.ObservedAt,
		ConfigVersion: outcome.ConfigVersion,
	})
	return err
}

var _ gitapp.CriterionSink = (*ValidationSink)(nil)

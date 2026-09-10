package criteria

import (
	"context"

	contributionapp "agentguild.dev/agentguild/backend/internal/contribution/application"
	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
)

// ReviewSink 把评审人的逐条验收结论写入 criterion 账本。
type ReviewSink struct {
	recorder *contributionapp.CriterionRecorder
	source   contributionapp.TaskCriteriaSource
}

func NewReviewSink(recorder *contributionapp.CriterionRecorder, source contributionapp.TaskCriteriaSource) *ReviewSink {
	return &ReviewSink{recorder: recorder, source: source}
}

// RecordReviewOutcome 只接受任务规格里真实存在的 criterion。评审人对规格之外
// 的标识给出的结论会被丢弃，避免凭空造出验收标准。
func (s *ReviewSink) RecordReviewOutcome(ctx context.Context, outcome reviewapp.ReviewCriterionOutcome) error {
	if s == nil || s.recorder == nil || len(outcome.Verdicts) == 0 {
		return nil
	}
	spec, err := s.source.CriteriaForExecution(ctx, outcome.TenantID, outcome.ExecutionID)
	if err != nil {
		return err
	}
	if spec.TaskID == "" {
		return nil
	}
	known := make(map[string]contributionapp.CriterionSpec, len(spec.Criteria))
	for _, criterion := range spec.Criteria {
		known[criterion.ID] = criterion
	}

	commands := make([]contributionapp.RecordCriterionResult, 0, len(outcome.Verdicts))
	for _, verdict := range outcome.Verdicts {
		criterion, found := known[verdict.CriterionID]
		if !found {
			continue
		}
		commands = append(commands, contributionapp.RecordCriterionResult{
			ResourceTenantID: outcome.TenantID,
			TaskID:           spec.TaskID,
			ExecutionID:      outcome.ExecutionID,
			CriterionID:      verdict.CriterionID,
			Critical:         criterion.Critical,
			VerifierKind:     contributiondomain.VerifierManual,
			Passed:           verdict.Passed,
			SourceKind:       contributiondomain.SourceReview,
			SourceID:         outcome.ReviewID,
			VerifiedBy:       "reviewer:" + outcome.ReviewerID,
			EvidenceURI:      verdict.EvidenceURI,
			ObservedAt:       outcome.ObservedAt,
		})
	}
	_, err = s.recorder.RecordBatch(ctx, commands)
	return err
}

var _ reviewapp.CriterionSink = (*ReviewSink)(nil)

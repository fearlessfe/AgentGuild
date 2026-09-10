package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
)

// CriterionSpec 是任务规格里一条验收标准的最小投影。contribution 模块不
// 依赖 publictask，调用方负责映射。
type CriterionSpec struct {
	ID           string
	Critical     bool
	VerifierKind string
	VerifierRef  string
	// Automated 表示这条标准绑定了一个已知的验证步骤，可以被自动判定。
	Automated bool
}

// TaskCriteriaSource 解析某个 Execution 所属任务规格中的验收标准。
type TaskCriteriaSource interface {
	CriteriaForExecution(ctx context.Context, tenantID, executionID string) (TaskCriteria, error)
}

type TaskCriteria struct {
	TaskID   string
	Criteria []CriterionSpec
}

// ValidationOutcome 是一次验证作业的终态。StepResults 以步骤名为键，
// 值为该步骤是否通过。
type ValidationOutcome struct {
	TenantID      string
	ExecutionID   string
	JobID         string
	StepResults   map[string]bool
	ObservedAt    time.Time
	ConfigVersion string
}

// ValidationCriterionRecorder 把验证步骤的结果翻译成逐条 criterion 的事实。
//
// 只有显式绑定到已执行步骤的自动化标准才会被判定。未绑定、绑定到未知步骤、
// 或该步骤本次没有运行的标准一律不落结果，停留在“未验证”——把没跑过的检查
// 当成通过，是这条链路上最危险的错误。
type ValidationCriterionRecorder struct {
	recorder *CriterionRecorder
	source   TaskCriteriaSource
}

func NewValidationCriterionRecorder(recorder *CriterionRecorder, source TaskCriteriaSource) (*ValidationCriterionRecorder, error) {
	if recorder == nil || source == nil {
		return nil, domain.ErrInvalidArgument
	}
	return &ValidationCriterionRecorder{recorder: recorder, source: source}, nil
}

// RecordValidationOutcome 返回本次新写入的事实条数。重复投递同一个
// validation job 不会产生新条目。
func (r *ValidationCriterionRecorder) RecordValidationOutcome(ctx context.Context, outcome ValidationOutcome) (int, error) {
	if outcome.TenantID == "" || outcome.ExecutionID == "" || outcome.JobID == "" {
		return 0, domain.ErrInvalidArgument
	}
	if outcome.ObservedAt.IsZero() {
		return 0, domain.ErrInvalidArgument
	}
	spec, err := r.source.CriteriaForExecution(ctx, outcome.TenantID, outcome.ExecutionID)
	if err != nil {
		return 0, err
	}
	if spec.TaskID == "" {
		return 0, nil
	}

	commands := make([]RecordCriterionResult, 0, len(spec.Criteria))
	for _, criterion := range spec.Criteria {
		if !criterion.Automated || criterion.VerifierRef == "" {
			continue
		}
		passed, ran := outcome.StepResults[criterion.VerifierRef]
		if !ran {
			continue
		}
		commands = append(commands, RecordCriterionResult{
			ResourceTenantID: outcome.TenantID,
			TaskID:           spec.TaskID,
			ExecutionID:      outcome.ExecutionID,
			CriterionID:      criterion.ID,
			Critical:         criterion.Critical,
			VerifierKind:     verifierKindOf(criterion.VerifierKind),
			Passed:           passed,
			SourceKind:       domain.SourceValidationJob,
			SourceID:         outcome.JobID,
			VerifiedBy:       "validation_worker",
			VerifierVersion:  outcome.ConfigVersion,
			ObservedAt:       outcome.ObservedAt,
		})
	}
	return r.recorder.RecordBatch(ctx, commands)
}

func verifierKindOf(kind string) domain.VerifierKind {
	if kind == string(domain.VerifierCI) {
		return domain.VerifierCI
	}
	return domain.VerifierCommand
}

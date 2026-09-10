package application

import (
	"context"
	"time"
)

// ValidationCriterionOutcome 是一次验证作业终态的可移植描述。它刻意不引用
// git 领域类型，好让 criterion 记录方无需依赖 git 模块。
type ValidationCriterionOutcome struct {
	TenantID      string
	ExecutionID   string
	JobID         string
	ConfigVersion string
	// StepResults 以验证步骤名为键，值为该步骤是否通过。跳过的步骤不出现
	// 在这里——没跑过的检查不能产生任何结论。
	StepResults map[string]bool
	ObservedAt  time.Time
}

// CriterionSink 接收验证终态并把它翻译成逐条验收标准的事实。
type CriterionSink interface {
	RecordValidationOutcome(ctx context.Context, outcome ValidationCriterionOutcome) error
}

// NopCriterionSink 用于未接入 criterion 账本的部署与测试。
type NopCriterionSink struct{}

func (NopCriterionSink) RecordValidationOutcome(context.Context, ValidationCriterionOutcome) error {
	return nil
}

var _ CriterionSink = (*NopCriterionSink)(nil)

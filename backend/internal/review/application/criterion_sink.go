package application

import (
	"context"
	"time"
)

// CriterionVerdict 是评审人对单条验收标准给出的结论。
type CriterionVerdict struct {
	CriterionID string
	Passed      bool
	// EvidenceURI 指向支撑该结论的证据（评论、日志、截图），可为空。
	EvidenceURI string
}

// ReviewCriterionOutcome 是一次评审定稿携带的逐条验收结论。
type ReviewCriterionOutcome struct {
	TenantID    string
	ExecutionID string
	ReviewID    string
	ReviewerID  string
	Verdicts    []CriterionVerdict
	ObservedAt  time.Time
}

// CriterionSink 接收评审定稿时的逐条验收结论。review 模块只认识这个接口，
// 不依赖 contribution 账本的实现。
type CriterionSink interface {
	RecordReviewOutcome(ctx context.Context, outcome ReviewCriterionOutcome) error
}

// NopCriterionSink 用于未接入 criterion 账本的部署与测试。
type NopCriterionSink struct{}

func (NopCriterionSink) RecordReviewOutcome(context.Context, ReviewCriterionOutcome) error {
	return nil
}

var _ CriterionSink = (*NopCriterionSink)(nil)

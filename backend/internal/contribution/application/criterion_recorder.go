package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
)

// CriterionRecorder 是验收标准结果的唯一写入入口。validation worker 与
// review 服务都经由它落库，保证幂等语义只有一处实现。
type CriterionRecorder struct {
	repository CriterionRepository
	now        func() time.Time
}

type CriterionRecorderOptions struct {
	Now func() time.Time
}

func NewCriterionRecorder(repository CriterionRepository, options CriterionRecorderOptions) (*CriterionRecorder, error) {
	if repository == nil {
		return nil, domain.ErrInvalidArgument
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &CriterionRecorder{repository: repository, now: now}, nil
}

type RecordCriterionResult struct {
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	CriterionID      string
	Critical         bool
	VerifierKind     domain.VerifierKind
	Passed           bool
	SourceKind       domain.SourceKind
	SourceID         string
	VerifiedBy       string
	VerifierVersion  string
	EvidenceURI      string
	EvidenceHash     string
	ObservedAt       time.Time
}

// Record 记录单条结果。重复投递返回既有事实，不产生新的账本条目。
func (r *CriterionRecorder) Record(ctx context.Context, command RecordCriterionResult) (*domain.CriterionResult, bool, error) {
	observedAt := command.ObservedAt
	if observedAt.IsZero() {
		observedAt = r.now().UTC()
	}
	result, err := domain.NewCriterionResult(domain.NewCriterionResultParams{
		ResourceTenantID: command.ResourceTenantID,
		TaskID:           command.TaskID,
		ExecutionID:      command.ExecutionID,
		CriterionID:      command.CriterionID,
		Critical:         command.Critical,
		VerifierKind:     command.VerifierKind,
		Passed:           command.Passed,
		SourceKind:       command.SourceKind,
		SourceID:         command.SourceID,
		VerifiedBy:       command.VerifiedBy,
		VerifierVersion:  command.VerifierVersion,
		EvidenceURI:      command.EvidenceURI,
		EvidenceHash:     command.EvidenceHash,
		ObservedAt:       observedAt,
	})
	if err != nil {
		return nil, false, err
	}
	return r.repository.Record(ctx, result)
}

// RecordBatch 记录一组结果。任何一条失败都立刻返回，已成功的条目保留在
// 账本中——账本是 append-only 的，部分成功不是需要回滚的状态。
func (r *CriterionRecorder) RecordBatch(ctx context.Context, commands []RecordCriterionResult) (int, error) {
	recorded := 0
	for _, command := range commands {
		_, inserted, err := r.Record(ctx, command)
		if err != nil {
			return recorded, err
		}
		if inserted {
			recorded++
		}
	}
	return recorded, nil
}

// Coverage 把任务规格里的验收标准与已记录的最新态对齐，供奖励门禁和
// 声望投影共用。
func (r *CriterionRecorder) Coverage(ctx context.Context, tenantID, executionID string, spec []domain.SpecCriterion) (domain.CriterionCoverage, error) {
	latest, err := r.repository.ListLatest(ctx, tenantID, executionID)
	if err != nil {
		return domain.CriterionCoverage{}, err
	}
	return domain.SummarizeCriteria(spec, latest), nil
}

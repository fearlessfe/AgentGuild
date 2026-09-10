package domain

import (
	"regexp"
	"strings"
	"time"
)

// VerifierKind 是结果侧的验证方式，由平台在写入点决定，不采信任务规格里
// 分析器自由生成的文本。
type VerifierKind string

const (
	VerifierCommand VerifierKind = "command"
	VerifierCI      VerifierKind = "ci"
	VerifierManual  VerifierKind = "manual"
)

// SourceKind 标识产生该结果的验证来源。它与 SourceID 一起构成幂等键：
// 同一来源对同一 criterion 的重复投递不会产生第二条事实。
type SourceKind string

const (
	SourceValidationJob  SourceKind = "validation_job"
	SourceReview         SourceKind = "review"
	SourceManualOverride SourceKind = "manual_override"
)

// CriterionResult 是单条验收标准的验证事实。它是 append-only 的：修正只能
// 追加新的结果，最新态由 observed_at 决定。
type CriterionResult struct {
	ID               int64
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	CriterionID      string
	Critical         bool
	VerifierKind     VerifierKind
	Passed           bool
	SourceKind       SourceKind
	SourceID         string
	VerifiedBy       string
	VerifierVersion  string
	EvidenceURI      string
	EvidenceHash     string
	ObservedAt       time.Time
	RecordedAt       time.Time
}

type NewCriterionResultParams struct {
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	CriterionID      string
	Critical         bool
	VerifierKind     VerifierKind
	Passed           bool
	SourceKind       SourceKind
	SourceID         string
	VerifiedBy       string
	VerifierVersion  string
	EvidenceURI      string
	EvidenceHash     string
	ObservedAt       time.Time
}

var evidenceHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

func NewCriterionResult(params NewCriterionResultParams) (*CriterionResult, error) {
	fields := []struct{ name, value string }{
		{"resource_tenant_id", params.ResourceTenantID},
		{"task_id", params.TaskID},
		{"execution_id", params.ExecutionID},
		{"criterion_id", params.CriterionID},
		{"source_id", params.SourceID},
		{"verified_by", params.VerifiedBy},
	}
	for _, field := range fields {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalidArgument(field.name)
		}
	}
	if !validVerifierKind(params.VerifierKind) {
		return nil, invalidArgument("verifier_kind")
	}
	if !validSourceKind(params.SourceKind) {
		return nil, invalidArgument("source_kind")
	}
	if params.EvidenceHash != "" && !evidenceHash.MatchString(params.EvidenceHash) {
		return nil, invalidArgument("evidence_hash")
	}
	if params.ObservedAt.IsZero() {
		return nil, invalidArgument("observed_at")
	}
	return &CriterionResult{
		ResourceTenantID: params.ResourceTenantID,
		TaskID:           params.TaskID,
		ExecutionID:      params.ExecutionID,
		CriterionID:      params.CriterionID,
		Critical:         params.Critical,
		VerifierKind:     params.VerifierKind,
		Passed:           params.Passed,
		SourceKind:       params.SourceKind,
		SourceID:         params.SourceID,
		VerifiedBy:       params.VerifiedBy,
		VerifierVersion:  params.VerifierVersion,
		EvidenceURI:      params.EvidenceURI,
		EvidenceHash:     params.EvidenceHash,
		ObservedAt:       params.ObservedAt,
	}, nil
}

// CriterionCoverage 是某个 Execution 上全部 criterion 最新态的汇总，
// 供奖励释放门禁与声望 correctness 维度共同使用。
type CriterionCoverage struct {
	Required        int
	RequiredPassed  int
	Optional        int
	OptionalPassed  int
	Unverified      []string
	FailedRequired  []string
	LatestObserved  time.Time
	ResultsByID     map[string]CriterionResult
	SpecCriterionID []string
}

// SummarizeCriteria 把任务规格（期望的 criterion 全集）与已记录的最新态
// 结果对齐。规格里存在但没有任何结果的 criterion 记入 Unverified，
// 绝不当作通过——这是"未验证"与"验证失败"必须区分的地方。
func SummarizeCriteria(spec []SpecCriterion, latest []CriterionResult) CriterionCoverage {
	byID := make(map[string]CriterionResult, len(latest))
	for _, result := range latest {
		byID[result.CriterionID] = result
	}
	coverage := CriterionCoverage{
		ResultsByID:     byID,
		SpecCriterionID: make([]string, 0, len(spec)),
	}
	for _, criterion := range spec {
		coverage.SpecCriterionID = append(coverage.SpecCriterionID, criterion.ID)
		if criterion.Critical {
			coverage.Required++
		} else {
			coverage.Optional++
		}
		result, found := byID[criterion.ID]
		if !found {
			coverage.Unverified = append(coverage.Unverified, criterion.ID)
			continue
		}
		if result.ObservedAt.After(coverage.LatestObserved) {
			coverage.LatestObserved = result.ObservedAt
		}
		switch {
		case result.Passed && criterion.Critical:
			coverage.RequiredPassed++
		case result.Passed:
			coverage.OptionalPassed++
		case criterion.Critical:
			coverage.FailedRequired = append(coverage.FailedRequired, criterion.ID)
		}
	}
	return coverage
}

// SpecCriterion 是任务规格中一条验收标准的最小投影。contribution 模块不
// 依赖 publictask 包，调用方负责映射。
type SpecCriterion struct {
	ID       string
	Critical bool
}

// AllRequiredPassed 是奖励释放的硬门禁：任一 required criterion 未通过或
// 未验证，Agent 份额都不得释放（doc §10）。规格里一条 required criterion
// 都没有时同样返回 false —— 没有硬性验收标准的任务不具备自动释放条件，
// 只能走人工批准，这里 fail closed。
func (c CriterionCoverage) AllRequiredPassed() bool {
	if c.Required == 0 {
		return false
	}
	if len(c.FailedRequired) > 0 {
		return false
	}
	return c.RequiredPassed == c.Required
}

func validVerifierKind(kind VerifierKind) bool {
	return kind == VerifierCommand || kind == VerifierCI || kind == VerifierManual
}

func validSourceKind(kind SourceKind) bool {
	return kind == SourceValidationJob || kind == SourceReview || kind == SourceManualOverride
}

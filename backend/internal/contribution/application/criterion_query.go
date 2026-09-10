package application

import (
	"context"
	"time"

	coreapp "agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	coredomain "agentguild.dev/agentguild/backend/internal/domain"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
)

// CriterionResultView 是单条验收标准的对外投影。它刻意不包含 sponsor 租户，
// 因此可以安全地返回给持有任务级 grant 的跨租户 Agent。
type CriterionResultView struct {
	CriterionID     string    `json:"criterion_id"`
	Critical        bool      `json:"critical"`
	Status          string    `json:"status"`
	VerifierKind    string    `json:"verifier_kind"`
	SourceKind      string    `json:"source_kind"`
	VerifiedBy      string    `json:"verified_by"`
	VerifierVersion string    `json:"verifier_version,omitempty"`
	EvidenceURI     string    `json:"evidence_uri,omitempty"`
	EvidenceHash    string    `json:"evidence_hash,omitempty"`
	ObservedAt      time.Time `json:"observed_at,omitempty"`
}

// CriterionCoverageView 汇总一个 Execution 的验收进度。
type CriterionCoverageView struct {
	ExecutionID       string                `json:"execution_id"`
	TaskID            string                `json:"task_id"`
	RequiredTotal     int                   `json:"required_total"`
	RequiredPassed    int                   `json:"required_passed"`
	OptionalTotal     int                   `json:"optional_total"`
	OptionalPassed    int                   `json:"optional_passed"`
	AllRequiredPassed bool                  `json:"all_required_passed"`
	Criteria          []CriterionResultView `json:"criteria"`
}

const (
	criterionStatusPassed     = "passed"
	criterionStatusFailed     = "failed"
	criterionStatusUnverified = "unverified"
)

// CriterionQueryService 对外提供验收证据的只读视图。
type CriterionQueryService struct {
	repository    CriterionRepository
	source        TaskCriteriaSource
	participation coreapp.ParticipationAuthorizer
}

func NewCriterionQueryService(repository CriterionRepository, source TaskCriteriaSource, participation coreapp.ParticipationAuthorizer) (*CriterionQueryService, error) {
	if repository == nil || source == nil {
		return nil, domain.ErrInvalidArgument
	}
	return &CriterionQueryService{repository: repository, source: source, participation: participation}, nil
}

// ExecutionCriteria 返回某个 Execution 上逐条验收标准的最新态。
func (s *CriterionQueryService) ExecutionCriteria(ctx context.Context, principal auth.Principal, executionID string) (coreapp.Envelope[CriterionCoverageView], error) {
	var envelope coreapp.Envelope[CriterionCoverageView]
	if executionID == "" {
		return envelope, domain.ErrInvalidArgument
	}
	tenantID, err := s.resolveTenant(ctx, principal, executionID)
	if err != nil {
		return envelope, err
	}

	spec, err := s.source.CriteriaForExecution(ctx, tenantID, executionID)
	if err != nil {
		return envelope, err
	}
	latest, err := s.repository.ListLatest(ctx, tenantID, executionID)
	if err != nil {
		return envelope, err
	}

	specCriteria := make([]domain.SpecCriterion, len(spec.Criteria))
	for i, criterion := range spec.Criteria {
		specCriteria[i] = domain.SpecCriterion{ID: criterion.ID, Critical: criterion.Critical}
	}
	coverage := domain.SummarizeCriteria(specCriteria, latest)

	views := make([]CriterionResultView, 0, len(spec.Criteria))
	for _, criterion := range spec.Criteria {
		view := CriterionResultView{
			CriterionID: criterion.ID,
			Critical:    criterion.Critical,
			Status:      criterionStatusUnverified,
		}
		if result, verified := coverage.ResultsByID[criterion.ID]; verified {
			view.Status = criterionStatusFailed
			if result.Passed {
				view.Status = criterionStatusPassed
			}
			view.VerifierKind = string(result.VerifierKind)
			view.SourceKind = string(result.SourceKind)
			view.VerifiedBy = result.VerifiedBy
			view.VerifierVersion = result.VerifierVersion
			view.EvidenceURI = result.EvidenceURI
			view.EvidenceHash = result.EvidenceHash
			view.ObservedAt = result.ObservedAt
		}
		views = append(views, view)
	}

	envelope.Data = CriterionCoverageView{
		ExecutionID:       executionID,
		TaskID:            spec.TaskID,
		RequiredTotal:     coverage.Required,
		RequiredPassed:    coverage.RequiredPassed,
		OptionalTotal:     coverage.Optional,
		OptionalPassed:    coverage.OptionalPassed,
		AllRequiredPassed: coverage.AllRequiredPassed(),
		Criteria:          views,
	}
	envelope.Meta = coreapp.Meta{ServerTime: time.Now().UTC()}
	return envelope, nil
}

// resolveTenant 复用与 review 一致的资源授权形状：租户内主体按自身租户读取，
// 全局 Agent 必须持有该 Execution 的任务级 grant。
func (s *CriterionQueryService) resolveTenant(ctx context.Context, principal auth.Principal, executionID string) (string, error) {
	if !principal.IsGlobalAgent() {
		if principal.TenantID == "" {
			return "", coredomain.ErrForbidden
		}
		return principal.TenantID, nil
	}
	if s.participation == nil {
		return "", coredomain.ErrForbidden
	}
	grant, err := s.participation.Authorize(ctx, principal,
		participationdomain.ResourceExecution, executionID, participationdomain.ScopeExecutionRead)
	if err != nil {
		return "", err
	}
	return grant.ResourceTenantID, nil
}

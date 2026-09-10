package domain

import (
	"regexp"
	"strings"
	"time"
)

const (
	StatusPublished = "published"
	StatusRevoked   = "revoked"

	QualityStandard      = "standard"
	QualityHighAssurance = "high_assurance"
	QualityHumanReviewed = "human_reviewed"

	// 难度分级只能由分析流水线写入，绝不接受 Agent 自报（doc §4.4）。
	// 声望的 impact 维度按此加权，取值区间是预先版本化的。
	DifficultyTrivial     = "trivial"
	DifficultyStandard    = "standard"
	DifficultySubstantial = "substantial"
	DifficultyComplex     = "complex"
)

// DifficultyClasses 是允许的难度分级全集。
var DifficultyClasses = []string{
	DifficultyTrivial, DifficultyStandard, DifficultySubstantial, DifficultyComplex,
}

// ValidDifficultyClass 报告 value 是否是已知的难度分级。
func ValidDifficultyClass(value string) bool {
	for _, class := range DifficultyClasses {
		if class == value {
			return true
		}
	}
	return false
}

type PublicationChecks struct {
	QualityGatePassed      bool
	VisibilityAllowed      bool
	SensitivityCheckPassed bool
	RepositoryPublic       bool
}

func (c PublicationChecks) Passed() bool {
	return c.QualityGatePassed && c.VisibilityAllowed && c.SensitivityCheckPassed && c.RepositoryPublic
}

type AcceptanceCriterion struct {
	ID             string `json:"id"`
	Statement      string `json:"statement"`
	Critical       bool   `json:"critical"`
	VerifierKind   string `json:"verifier_kind"`
	ExpectedResult string `json:"expected_result"`
	// VerifierRef 把一条自动化验收标准绑定到具体的验证步骤名。它是可选的：
	// 缺失或指向未知步骤时，该标准不会被自动判定，只能停留在“未验证”直到
	// 人工评审给出结论。分析器是不可信输入，这里 fail closed。
	VerifierRef string `json:"verifier_ref,omitempty"`
}

// AutomatedVerifier 返回该标准绑定的验证步骤名，以及它是否可被自动判定。
func (c AcceptanceCriterion) AutomatedVerifier() (string, bool) {
	if c.VerifierKind != "command" && c.VerifierKind != "ci" {
		return "", false
	}
	if c.VerifierRef == "" {
		return "", false
	}
	return c.VerifierRef, true
}

type EvidenceRef struct {
	CommitSHA   string `json:"commit_sha"`
	Path        string `json:"path"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	ContentHash string `json:"content_hash"`
}

// Projection is an explicit public-field whitelist. Operator, organization,
// sponsor policy, private evidence, and model traces have no representation.
type Projection struct {
	ID                         string
	ResourceTenantID           string
	TaskID                     string
	TaskSpecificationVersionID string
	CanonicalRepository        string
	SourceIssueURL             string
	IssueRevision              string
	BaseCommit                 string
	Title                      string
	Summary                    string
	ProblemDiagnosis           string
	Impact                     string
	ProposedSolution           string
	ImplementationSteps        []string
	Constraints                []string
	NonGoals                   []string
	Risks                      []string
	AcceptanceCriteria         []AcceptanceCriterion
	EvidenceRefs               []EvidenceRef
	QualityLevel               string
	// DifficultyClass 由分析流水线判定，用于声望的难度加权。
	DifficultyClass string
	// SpecHash 是任务规格的规范化摘要，让奖励决定可以引用一个稳定的规格
	// 版本而不暴露规格正文。它由 NewProjection 计算，不接受外部传入。
	SpecHash         string
	Status           string
	PublishedAt      time.Time
	RevokedAt        *time.Time
	RevocationActor  string
	RevocationReason string
}

type NewProjectionParams struct {
	Projection
	Checks PublicationChecks
}

var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

func NewProjection(params NewProjectionParams) (*Projection, error) {
	if !params.Checks.Passed() {
		return nil, ErrPublicationRejected
	}
	p := params.Projection
	fields := []struct{ name, value string }{
		{"id", p.ID}, {"resource_tenant_id", p.ResourceTenantID}, {"task_id", p.TaskID},
		{"task_specification_version_id", p.TaskSpecificationVersionID}, {"canonical_repository", p.CanonicalRepository},
		{"source_issue_url", p.SourceIssueURL}, {"issue_revision", p.IssueRevision},
		{"title", p.Title}, {"summary", p.Summary}, {"problem_diagnosis", p.ProblemDiagnosis},
		{"impact", p.Impact}, {"proposed_solution", p.ProposedSolution},
	}
	for _, field := range fields {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	if !commitSHA.MatchString(p.BaseCommit) {
		return nil, invalid("base_commit")
	}
	if p.QualityLevel != QualityStandard && p.QualityLevel != QualityHighAssurance && p.QualityLevel != QualityHumanReviewed {
		return nil, invalid("quality_level")
	}
	if p.DifficultyClass == "" {
		p.DifficultyClass = DifficultyStandard
	}
	if !ValidDifficultyClass(p.DifficultyClass) {
		return nil, invalid("difficulty_class")
	}
	if p.PublishedAt.IsZero() {
		return nil, invalid("published_at")
	}
	if len(p.AcceptanceCriteria) == 0 {
		return nil, invalid("acceptance_criteria")
	}
	seen := make(map[string]struct{}, len(p.AcceptanceCriteria))
	for _, criterion := range p.AcceptanceCriteria {
		if criterion.ID == "" || criterion.Statement == "" || criterion.VerifierKind == "" || criterion.ExpectedResult == "" {
			return nil, invalid("acceptance_criteria")
		}
		if _, exists := seen[criterion.ID]; exists {
			return nil, invalid("acceptance_criteria")
		}
		seen[criterion.ID] = struct{}{}
	}
	for _, evidence := range p.EvidenceRefs {
		if !commitSHA.MatchString(evidence.CommitSHA) || evidence.Path == "" || evidence.ContentHash == "" || evidence.StartLine < 1 || evidence.EndLine < evidence.StartLine {
			return nil, invalid("evidence_refs")
		}
	}
	// 规格摘要由平台计算，忽略调用方传入的任何值。
	specHash, err := p.ComputeSpecHash()
	if err != nil {
		return nil, invalid("spec_hash")
	}
	p.SpecHash = specHash
	p.Status = StatusPublished
	p.RevokedAt = nil
	p.RevocationActor = ""
	p.RevocationReason = ""
	return &p, nil
}

func (p *Projection) Revoke(actor, reason string, now time.Time) error {
	if p.Status != StatusPublished || actor == "" || reason == "" || now.IsZero() {
		return ErrStateConflict
	}
	p.Status = StatusRevoked
	p.RevokedAt = &now
	p.RevocationActor = actor
	p.RevocationReason = reason
	return nil
}

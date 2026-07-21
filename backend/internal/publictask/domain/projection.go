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
)

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
	Status                     string
	PublishedAt                time.Time
	RevokedAt                  *time.Time
	RevocationActor            string
	RevocationReason           string
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

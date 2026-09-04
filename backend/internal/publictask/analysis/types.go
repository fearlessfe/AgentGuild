// Package analysis defines the private boundary between public-task source
// inspection and the projection publisher.
package analysis

import (
	"context"
	"strings"

	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
)

// SourceFile is a bounded, text-only excerpt from the pinned repository state.
// It is never persisted in the public task projection.
type SourceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Input is the complete context an analyzer may inspect. All repository
// content and Issue text are untrusted data, not platform instructions.
type Input struct {
	Repository  string
	BaseCommit  string
	IssueURL    string
	IssueNumber int
	Title       string
	Problem     string
	Files       []SourceFile
}

// Result is the model's proposed public task contract. The publisher validates
// it again before it can become visible or claimable.
type Result struct {
	Title               string                                 `json:"title"`
	Summary             string                                 `json:"summary"`
	ProblemDiagnosis    string                                 `json:"problem_diagnosis"`
	Impact              string                                 `json:"impact"`
	ProposedSolution    string                                 `json:"proposed_solution"`
	ImplementationSteps []string                               `json:"implementation_steps"`
	Constraints         []string                               `json:"constraints"`
	NonGoals            []string                               `json:"non_goals"`
	Risks               []string                               `json:"risks"`
	AcceptanceCriteria  []publictaskdomain.AcceptanceCriterion `json:"acceptance_criteria"`
}

// Analyzer turns a pinned source snapshot and Issue into a structured task.
type Analyzer interface {
	Analyze(context.Context, Input) (Result, error)
}

// Normalize trims model-generated scalar fields and empty list entries before
// the domain publication checks run.
func (r Result) Normalize() Result {
	r.Title = strings.TrimSpace(r.Title)
	r.Summary = strings.TrimSpace(r.Summary)
	r.ProblemDiagnosis = strings.TrimSpace(r.ProblemDiagnosis)
	r.Impact = strings.TrimSpace(r.Impact)
	r.ProposedSolution = strings.TrimSpace(r.ProposedSolution)
	r.ImplementationSteps = cleanStrings(r.ImplementationSteps)
	r.Constraints = cleanStrings(r.Constraints)
	r.NonGoals = cleanStrings(r.NonGoals)
	r.Risks = cleanStrings(r.Risks)
	for i := range r.AcceptanceCriteria {
		r.AcceptanceCriteria[i].ID = strings.TrimSpace(r.AcceptanceCriteria[i].ID)
		r.AcceptanceCriteria[i].Statement = strings.TrimSpace(r.AcceptanceCriteria[i].Statement)
		r.AcceptanceCriteria[i].VerifierKind = strings.TrimSpace(r.AcceptanceCriteria[i].VerifierKind)
		r.AcceptanceCriteria[i].ExpectedResult = strings.TrimSpace(r.AcceptanceCriteria[i].ExpectedResult)
	}
	return r
}

func cleanStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

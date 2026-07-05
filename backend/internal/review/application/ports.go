package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
)

// Store is the review application transaction boundary.
// It exposes only the code-review repositories plus transaction time.
type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

// Tx is the review application transaction handle.
// It exposes only the code-review repositories plus transaction time.
// The review service itself orchestrates with the global application.Tx
// interface, which embeds these same repository interfaces.
type Tx interface {
	Now(context.Context) (time.Time, error)
	Reviews() application.ReviewRepository
	LineComments() application.LineCommentRepository
	Rubrics() application.RubricRepository
	Reviewers() application.ReviewerRepository
}

// DiffLine represents a single line inside a diff hunk.
type DiffLine struct {
	Type   string `json:"type"`
	Text   string `json:"text"`
	OldLine int   `json:"old_line,omitempty"`
	NewLine int   `json:"new_line,omitempty"`
}

// Hunk represents a contiguous block of changed lines.
type Hunk struct {
	OldStart int        `json:"old_start"`
	OldLines int        `json:"old_lines"`
	NewStart int        `json:"new_start"`
	NewLines int        `json:"new_lines"`
	HunkHash string     `json:"hunk_hash"`
	Lines    []DiffLine `json:"lines"`
}

// FileDiff represents a structured diff for a single file.
type FileDiff struct {
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
	Hunks   []Hunk `json:"hunks"`
}

// DiffProvider supplies the structured diff for a submission.
type DiffProvider interface {
	GetDiff(ctx context.Context, submissionID string) ([]FileDiff, error)
}

// ValidationStatus is the result returned by a ValidationProvider.
type ValidationStatus interface {
	AllHardGatesPassed() bool
}

// ValidationProvider reports whether a submission has passed its hard gates.
type ValidationProvider interface {
	GetValidationStatus(ctx context.Context, submissionID string) (ValidationStatus, error)
}

// SyntheticDiffProvider is a temporary stub that returns a deterministic
// structured diff from submission metadata. Replace with git-delivery-and-validation.
// TODO: replace with git-delivery-and-validation implementation
type SyntheticDiffProvider struct{}

// GetDiff returns a single-file synthetic diff for the submission.
func (SyntheticDiffProvider) GetDiff(_ context.Context, submissionID string) ([]FileDiff, error) {
	return []FileDiff{{
		Path: "submission.go",
		Hunks: []Hunk{{
			OldStart: 1,
			OldLines: 0,
			NewStart: 1,
			NewLines: 3,
			HunkHash: "hunk-" + submissionID,
			Lines: []DiffLine{
				{Type: "add", Text: "+// Generated diff for submission " + submissionID, NewLine: 1},
				{Type: "add", Text: "+func Run() {}", NewLine: 2},
				{Type: "context", Text: " // end", NewLine: 3},
			},
		}},
	}}, nil
}

// AlwaysPassValidationProvider is a temporary stub that reports all hard gates passed.
// TODO: replace with git-delivery-and-validation implementation
type AlwaysPassValidationProvider struct{}

// GetValidationStatus returns a status where all hard gates pass.
func (AlwaysPassValidationProvider) GetValidationStatus(context.Context, string) (ValidationStatus, error) {
	return alwaysPassValidationStatus{}, nil
}

type alwaysPassValidationStatus struct{}

func (alwaysPassValidationStatus) AllHardGatesPassed() bool { return true }

// AlwaysFailValidationProvider is a temporary stub useful for tests.
type AlwaysFailValidationProvider struct{}

// GetValidationStatus returns a status where hard gates failed.
func (AlwaysFailValidationProvider) GetValidationStatus(context.Context, string) (ValidationStatus, error) {
	return alwaysFailValidationStatus{}, nil
}

type alwaysFailValidationStatus struct{}

func (alwaysFailValidationStatus) AllHardGatesPassed() bool { return false }

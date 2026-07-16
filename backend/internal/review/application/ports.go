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
	Type    string `json:"type"`
	Text    string `json:"text"`
	OldLine int    `json:"old_line,omitempty"`
	NewLine int    `json:"new_line,omitempty"`
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
	GetDiff(ctx context.Context, tenantID, submissionID string) ([]FileDiff, error)
}

// ValidationStatus is the result returned by a ValidationProvider.
type ValidationStatus interface {
	AllHardGatesPassed() bool
}

// ValidationProvider reports whether a submission has passed its hard gates.
type ValidationProvider interface {
	GetValidationStatus(ctx context.Context, tenantID, submissionID string) (ValidationStatus, error)
}

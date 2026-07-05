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

// DiffProvider supplies the raw diff for a submission.
type DiffProvider interface {
	GetDiff(ctx context.Context, submissionID string) ([]byte, error)
}

// ValidationStatus is the result returned by a ValidationProvider.
type ValidationStatus interface {
	AllHardGatesPassed() bool
}

// ValidationProvider reports whether a submission has passed its hard gates.
type ValidationProvider interface {
	GetValidationStatus(ctx context.Context, submissionID string) (ValidationStatus, error)
}

package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/review/domain"
)

type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

type Tx interface {
	Reviews() ReviewRepository
	LineComments() LineCommentRepository
	Rubrics() RubricRepository
	Reviewers() ReviewerRepository
	Now(context.Context) (time.Time, error)
}

type ReviewRepository interface {
	Insert(context.Context, *domain.Review) error
	Update(context.Context, *domain.Review) error
	GetByID(context.Context, string, string) (*domain.Review, error)
	ListBySubmission(context.Context, string, string) ([]domain.Review, error)
}

type LineCommentRepository interface {
	Insert(context.Context, *domain.LineComment) error
	ListByReview(context.Context, string, string) ([]domain.LineComment, error)
}

type RubricRepository interface {
	GetActive(context.Context, string) (*domain.RubricVersion, error)
	GetByID(context.Context, string, string) (*domain.RubricVersion, error)
	ListVersions(context.Context, string) ([]domain.RubricVersion, error)
	CreateVersion(context.Context, *domain.RubricVersion) error
}

type ReviewerRepository interface {
	Insert(context.Context, *domain.ReviewerProfile) error
	GetByID(context.Context, string, string) (*domain.ReviewerProfile, error)
	ListActive(context.Context, string, int) ([]domain.ReviewerProfile, error)
	IncrementLoad(context.Context, string, string) error
	DecrementLoad(context.Context, string, string) error
}

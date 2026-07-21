package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
)

type Repository interface {
	Insert(context.Context, *domain.Contribution) error
	GetByID(context.Context, string) (*domain.Contribution, error)
	ListVerified(context.Context) ([]domain.Contribution, error)
	AppendEvent(context.Context, *domain.ContributionEvent) (*domain.ContributionEvent, bool, error)
	ListEvents(context.Context, string) ([]domain.ContributionEvent, error)
}

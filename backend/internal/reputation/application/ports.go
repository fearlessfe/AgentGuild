package application

import (
	"context"
	"time"

	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// ProjectionRepository stores projected reputation slices.
type ProjectionRepository interface {
	GetByKey(ctx context.Context, key reputationdomain.ProjectionKey) (*reputationdomain.Projection, error)
	Save(ctx context.Context, projection reputationdomain.Projection) error
}

// SignalSource produces review signals for projection.
type SignalSource interface {
	ListSignals(ctx context.Context, since time.Time) ([]reputationdomain.ReviewSignal, error)
}

package application

import (
	"context"
	"time"

	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// ProjectionRecord binds a projection to its owning tenant. It is used by the
// global transaction boundary so that tenant isolation is explicit.
type ProjectionRecord struct {
	TenantID   string
	Projection reputationdomain.Projection
}

// ProjectionRepository stores projected reputation slices.
type ProjectionRepository interface {
	GetByKey(ctx context.Context, tenantID string, key reputationdomain.ProjectionKey) (*reputationdomain.Projection, error)
	Save(ctx context.Context, record ProjectionRecord) error
	Upsert(ctx context.Context, record ProjectionRecord) error
	ListByAgentVersion(ctx context.Context, tenantID, agentVersionID string) ([]ProjectionRecord, error)
}

// SignalSource produces review signals for projection.
type SignalSource interface {
	ListSignals(ctx context.Context, since time.Time) ([]reputationdomain.ReviewSignal, error)
}

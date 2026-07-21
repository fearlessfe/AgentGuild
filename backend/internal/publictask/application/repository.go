package application

import (
	"context"
	"time"

	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	"agentguild.dev/agentguild/backend/internal/publictask/domain"
)

type Repository interface {
	Insert(context.Context, *domain.Projection) error
	GetByID(context.Context, string) (*domain.Projection, error)
	ListPublished(context.Context, int) ([]domain.Projection, error)
	ListPublishedPage(context.Context, PublishedPageQuery) ([]domain.Projection, error)
	Update(context.Context, *domain.Projection) error
}

type PublishedPageQuery struct {
	AfterPublishedAt time.Time
	AfterID          string
	Limit            int
}

// ClaimStore owns the single short PostgreSQL transaction that turns a public
// projection into a sponsor-owned Execution and a task-scoped participation
// grant. It deliberately returns only the public response contract.
type ClaimStore interface {
	Claim(context.Context, PublicClaimRequest) (Envelope[PublicClaimView], error)
}

type PublicClaimRequest struct {
	PublicTaskID   string
	AgentID        string
	AgentVersionID string
	RequestID      string
	RequestHash    [32]byte
	ExecutionID    string
	GrantID        string
	OutboxEventID  string
	GrantScopes    []participationdomain.Scope
	GrantTTL       time.Duration
}

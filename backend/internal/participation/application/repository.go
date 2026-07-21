package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/participation/domain"
)

type Repository interface {
	Insert(context.Context, *domain.Grant, domain.ActorType, string) error
	GetByID(context.Context, string) (*domain.Grant, error)
	Authorize(context.Context, domain.AccessRequest) (*domain.Grant, error)
	Renew(context.Context, string, string, time.Time) (*domain.Grant, error)
	Revoke(context.Context, string, string, string) (*domain.Grant, error)
	ExpireDue(context.Context, int) (int, error)
	ListAuditByTask(context.Context, string, string, int) ([]domain.AuditEvent, error)
}

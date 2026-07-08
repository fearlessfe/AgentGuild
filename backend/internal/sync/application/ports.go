package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/sync/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type RuleRepository interface {
	Create(ctx context.Context, rule *domain.Rule) error
	Get(ctx context.Context, tenantID, id string) (*domain.Rule, error)
	List(ctx context.Context, tenantID string) ([]*domain.Rule, error)
	ListEnabledAllTenants(ctx context.Context) ([]*domain.Rule, error)
	Update(ctx context.Context, rule *domain.Rule) error
	Delete(ctx context.Context, tenantID, id string) error
	TouchSynced(ctx context.Context, tenantID, id string, t time.Time) error
}

type Mapping struct {
	TenantID     string
	Repo         string
	IssueNumber  int
	TaskID       string
	IssueState   string
	IssueClosed  bool
	IssueURL     string
	LastSyncedAt time.Time
}

type MapRepository interface {
	Get(ctx context.Context, tenantID, repo string, issueNumber int) (*Mapping, error)
	Upsert(ctx context.Context, mapping *Mapping) error
}

type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

type Tx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Now(context.Context) (time.Time, error)
}

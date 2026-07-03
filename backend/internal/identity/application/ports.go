package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

type Tx interface {
	Agents() AgentRepository
	Versions() VersionRepository
	Credentials() CredentialRepository
	Audits() AuditRepository
	Now(context.Context) (time.Time, error)
}

type AgentRepository interface {
	Insert(context.Context, *domain.Agent) error
	GetByID(context.Context, string, string) (*domain.Agent, error)
	List(context.Context, AgentListQuery) ([]domain.Agent, error)
	Update(context.Context, *domain.Agent) error
}

type VersionRepository interface {
	Insert(context.Context, *domain.AgentVersion) error
	GetByID(context.Context, string, string) (*domain.AgentVersion, error)
	ListByAgent(context.Context, string, string) ([]domain.AgentVersion, error)
}

type CredentialRepository interface {
	Insert(context.Context, *domain.ActivationCredential) error
	GetPending(context.Context, string, string) (*domain.ActivationCredential, error)
	GetPendingByPlaintext(context.Context, string) (*domain.ActivationCredential, error)
	GetLatestByAgent(context.Context, string, string) (*domain.ActivationCredential, error)
	GetByID(context.Context, string, string) (*domain.ActivationCredential, error)
	Save(context.Context, *domain.ActivationCredential) error
}

type AuditRepository interface {
	Append(context.Context, domain.IdentityEvent) error
	ListByAgent(context.Context, string, string, int) ([]domain.IdentityEvent, error)
}

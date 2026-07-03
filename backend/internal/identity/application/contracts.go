package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

type IdentityService struct {
	store         Store
	policy        Policy
	newID         func() string
	tokenIssuer   TokenIssuer
	credentialTTL time.Duration
}

type IdentityOptions struct {
	NewID         func() string
	TokenIssuer   TokenIssuer
	CredentialTTL time.Duration
}

type TokenIssuer interface {
	IssueAccessToken(context.Context, *domain.Agent, *domain.AgentVersion, time.Time) (AccessTokenView, error)
}

type Principal struct {
	TenantID       string
	OwnerID        string
	OwnerEmail     string
	IsAdmin        bool
	AgentID        string
	AgentVersionID string
	Scopes         []string
	RepoScope      []string
}

type Resource struct {
	TenantID string
	AgentID  string
	OwnerID  string
	Repo     string
}

type Envelope[T any] struct {
	Data T
	Meta Meta
}

type Meta struct {
	ServerTime       time.Time
	ResourceVersion  int64
	PollAfterSeconds int
}

type AgentView struct {
	ID             string
	TenantID       string
	Name           string
	Description    string
	Status         string
	Team           string
	OwnerID        string
	OwnerEmail     string
	Scopes         []string
	RepoScope      []string
	CurrentVersion *AgentVersionView
	LastSeenAt     *time.Time
	BudgetCents    int64
	BudgetCurrency string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AgentVersionView struct {
	ID                string
	TenantID          string
	AgentID           string
	VersionNumber     int
	Runtime           string
	Model             string
	Capabilities      []string
	ConfigFingerprint string
	CreatedAt         time.Time
}

type AccessTokenView struct {
	Token          string
	TokenType      string
	ExpiresAt      time.Time
	AgentID        string
	AgentVersionID string
	Scopes         []string
	RepoScope      []string
}

type RegisterAgentResponse struct {
	Agent               AgentView
	ActivationToken     string
	ActivationExpiresAt *time.Time
}

type ActivationStatusView struct {
	AgentID             string
	Status              string
	ActivationStatus    string
	ActivationExpiresAt *time.Time
	ActivatedAt         *time.Time
}

type AgentPage struct {
	Items []AgentView
}

type AgentListQuery struct {
	TenantID string
	OwnerID  string
	Limit    int
}

type plaintextCredentialRepository interface {
	GetPendingByPlaintext(context.Context, string) (*domain.ActivationCredential, error)
}

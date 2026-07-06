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
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

type Meta struct {
	ServerTime       time.Time `json:"server_time"`
	ResourceVersion  int64     `json:"resource_version"`
	PollAfterSeconds int       `json:"poll_after_seconds"`
}

type AgentView struct {
	ID             string            `json:"id"`
	TenantID       string            `json:"tenant_id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Status         string            `json:"status"`
	Team           string            `json:"team,omitempty"`
	OwnerID        string            `json:"owner_id"`
	OwnerEmail     string            `json:"owner_email"`
	Scopes         []string          `json:"scopes,omitempty"`
	RepoScope      []string          `json:"repo_scope,omitempty"`
	CurrentVersion *AgentVersionView `json:"current_version,omitempty"`
	LastSeenAt     *time.Time        `json:"last_seen_at,omitempty"`
	BudgetCents    int64             `json:"budget_cents,omitempty"`
	BudgetCurrency string            `json:"budget_currency,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at,omitempty"`
}

type AgentVersionView struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	AgentID           string    `json:"agent_id"`
	VersionNumber     int       `json:"version_number"`
	Runtime           string    `json:"runtime"`
	Model             string    `json:"model"`
	Capabilities      []string  `json:"capabilities,omitempty"`
	ConfigFingerprint string    `json:"config_fingerprint"`
	CreatedAt         time.Time `json:"created_at"`
}

type AccessTokenView struct {
	Token          string    `json:"token"`
	TokenType      string    `json:"token_type"`
	ExpiresAt      time.Time `json:"expires_at"`
	AgentID        string    `json:"agent_id"`
	AgentVersionID string    `json:"agent_version_id"`
	Scopes         []string  `json:"scopes,omitempty"`
	RepoScope      []string  `json:"repo_scope,omitempty"`
}

type RegisterAgentResponse struct {
	Agent               AgentView  `json:"agent"`
	ActivationToken     string     `json:"activation_token"`
	ActivationExpiresAt *time.Time `json:"activation_expires_at,omitempty"`
}

type ActivationStatusView struct {
	AgentID             string     `json:"agent_id"`
	Status              string     `json:"status"`
	ActivationStatus    string     `json:"activation_status"`
	ActivationExpiresAt *time.Time `json:"activation_expires_at,omitempty"`
	ActivatedAt         *time.Time `json:"activated_at,omitempty"`
}

type AgentPage struct {
	Items []AgentView `json:"items"`
}

type AgentListQuery struct {
	TenantID string
	OwnerID  string
	Limit    int
}

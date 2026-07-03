package auth

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/domain"
)

type Principal struct {
	TenantID       string
	Type           string
	OwnerID        string
	OwnerEmail     string
	AgentID        string
	AgentVersionID string
	Scopes         []string
	RepoScope      []string
	IsAdmin        bool
}

type TokenVerifier interface {
	Verify(context.Context, string) (Principal, error)
}

const (
	PrincipalTypeHuman = "human"
	PrincipalTypeAgent = "agent"
)

type ScopePolicy struct{}

func (ScopePolicy) Require(principal Principal, scope string) error {
	for field, value := range map[string]string{
		"tenant_id": principal.TenantID, "agent_id": principal.AgentID, "agent_version_id": principal.AgentVersionID,
	} {
		if value == "" {
			return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
		}
	}
	for _, candidate := range principal.Scopes {
		if candidate == scope {
			return nil
		}
	}
	return domain.ErrForbidden
}

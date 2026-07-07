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
	if principal.TenantID == "" {
		return &domain.Error{Code: "invalid_argument", Message: "tenant_id is invalid", Field: "tenant_id"}
	}
	if principal.Type == PrincipalTypeHuman {
		return nil
	}
	for field, value := range map[string]string{
		"agent_id": principal.AgentID, "agent_version_id": principal.AgentVersionID,
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

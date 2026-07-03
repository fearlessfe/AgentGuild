package application

import (
	"strings"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

type Policy struct{}

func (Policy) Require(principal Principal, scope string, resource Resource) error {
	if principal.TenantID == "" {
		return domain.ErrForbidden
	}
	if resource.TenantID != "" && resource.TenantID != principal.TenantID {
		return domain.ErrForbidden
	}
	if scope != "" && !hasScope(principal.Scopes, scope) && !principal.IsAdmin {
		return domain.ErrForbidden
	}
	if resource.AgentID != "" && principal.AgentID != "" && resource.AgentID != principal.AgentID {
		return domain.ErrForbidden
	}
	if resource.OwnerID != "" && principal.OwnerID != "" && !principal.IsAdmin && resource.OwnerID != principal.OwnerID {
		return domain.ErrForbidden
	}
	if resource.Repo != "" && !hasRepoScope(principal.RepoScope, resource.Repo) && !principal.IsAdmin {
		return domain.ErrForbidden
	}
	return nil
}

func (Policy) RequireAgentStatus(agent *domain.Agent, allowed ...string) error {
	if agent == nil {
		return domain.ErrNotFound
	}
	for _, status := range allowed {
		if agent.Status == status {
			return nil
		}
	}
	return domain.ErrStateConflict
}

func (p Policy) RequireOwnerOrAdmin(principal Principal, agent *domain.Agent) error {
	if agent == nil {
		return domain.ErrNotFound
	}
	if err := p.Require(principal, "", Resource{TenantID: agent.TenantID}); err != nil {
		return err
	}
	if principal.IsAdmin {
		return nil
	}
	if principal.OwnerID != "" && principal.OwnerID == agent.OwnerID {
		return nil
	}
	return domain.ErrForbidden
}

func (p Policy) RequireAgentSelf(principal Principal, agent *domain.Agent) error {
	if agent == nil {
		return domain.ErrNotFound
	}
	if err := p.Require(principal, "", Resource{TenantID: agent.TenantID, AgentID: agent.ID}); err != nil {
		return err
	}
	if principal.AgentID == "" || principal.AgentID != agent.ID {
		return domain.ErrForbidden
	}
	if principal.AgentVersionID != "" && agent.CurrentVersionID != "" && principal.AgentVersionID != agent.CurrentVersionID {
		return domain.ErrForbidden
	}
	return nil
}

func (p Policy) RequireOwnerAdminOrAgentSelf(principal Principal, agent *domain.Agent) error {
	if principal.AgentID != "" {
		return p.RequireAgentSelf(principal, agent)
	}
	return p.RequireOwnerOrAdmin(principal, agent)
}

func hasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

func hasRepoScope(patterns []string, repo string) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == repo {
			return true
		}
		prefix, ok := strings.CutSuffix(pattern, "/*")
		if ok && strings.HasPrefix(repo, prefix+"/") {
			return true
		}
	}
	return false
}

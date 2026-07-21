package auth

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/domain"
)

type Principal struct {
	SubjectID      string
	IdentityScope  string
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

	IdentityScopeGlobal = "global"
	IdentityScopeTenant = "tenant"
)

type ScopePolicy struct{}

func (ScopePolicy) Require(principal Principal, scope string) error {
	if principal.Type == PrincipalTypeHuman {
		if principal.TenantID == "" {
			return &domain.Error{Code: "invalid_argument", Message: "tenant_id is invalid", Field: "tenant_id"}
		}
		return nil
	}
	for field, value := range map[string]string{
		"agent_id": principal.AgentID, "agent_version_id": principal.AgentVersionID,
	} {
		if value == "" {
			return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
		}
	}
	if principal.IdentityScope == IdentityScopeGlobal {
		if principal.Type != PrincipalTypeAgent || principal.TenantID != "" {
			return &domain.Error{Code: "invalid_argument", Message: "tenant_id is invalid", Field: "tenant_id"}
		}
		if principal.SubjectID != AgentSubject(principal.AgentID) {
			return &domain.Error{Code: "invalid_argument", Message: "subject is invalid", Field: "sub"}
		}
	} else {
		if principal.TenantID == "" {
			field := "tenant_id"
			if principal.Type == PrincipalTypeAgent {
				field = "identity_scope"
			}
			return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
		}
		if principal.IdentityScope != "" && principal.IdentityScope != IdentityScopeTenant {
			return &domain.Error{Code: "invalid_argument", Message: "identity_scope is invalid", Field: "identity_scope"}
		}
		if principal.Type != "" && principal.Type != PrincipalTypeAgent {
			return &domain.Error{Code: "invalid_argument", Message: "principal type is invalid", Field: "type"}
		}
		if principal.SubjectID != "" && principal.SubjectID != AgentSubject(principal.AgentID) {
			return &domain.Error{Code: "invalid_argument", Message: "subject is invalid", Field: "sub"}
		}
	}
	for _, candidate := range principal.Scopes {
		if candidate == scope {
			return nil
		}
	}
	return domain.ErrForbidden
}

// ResourcePolicy separates identity from authorization. Scope checks answer
// what an Agent may do; these methods answer which tenant-owned resources the
// Principal may address. A platform-global Agent never gains ordinary tenant
// access through this policy; public participation must use a separate,
// server-side task grant authorizer.
type ResourcePolicy struct{}

func (ResourcePolicy) Tenant(principal Principal) (string, error) {
	if principal.Type == PrincipalTypeHuman {
		if principal.TenantID == "" {
			return "", domain.ErrForbidden
		}
		return principal.TenantID, nil
	}
	if principal.IsGlobalAgent() || principal.TenantID == "" || principal.IdentityScope == IdentityScopeGlobal {
		return "", domain.ErrForbidden
	}
	if principal.IdentityScope != "" && principal.IdentityScope != IdentityScopeTenant {
		return "", domain.ErrForbidden
	}
	if principal.Type != "" && principal.Type != PrincipalTypeAgent {
		return "", domain.ErrForbidden
	}
	return principal.TenantID, nil
}

func (p ResourcePolicy) RequireTenant(principal Principal, resourceTenantID string) error {
	if resourceTenantID == "" {
		return domain.ErrForbidden
	}
	tenantID, err := p.Tenant(principal)
	if err != nil || tenantID != resourceTenantID {
		return domain.ErrForbidden
	}
	return nil
}

// AgentSubject is the stable JWT subject for a platform-global Agent identity.
func AgentSubject(agentID string) string {
	return "agent:" + agentID
}

// IsGlobalAgent reports whether the principal is a tenant-independent Agent.
func (p Principal) IsGlobalAgent() bool {
	return p.Type == PrincipalTypeAgent && p.IdentityScope == IdentityScopeGlobal && p.TenantID == "" && p.SubjectID == AgentSubject(p.AgentID)
}

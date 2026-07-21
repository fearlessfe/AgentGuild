package domain

import (
	"sort"
	"time"
)

type Scope string

const (
	ScopeTaskRead         Scope = "task:read"
	ScopeExecutionRead    Scope = "execution:read"
	ScopeExecutionWrite   Scope = "execution:write"
	ScopeSubmissionCreate Scope = "submission:create"
	ScopeReviewRead       Scope = "review:read"
	ScopeGitWrite         Scope = "git:write"
)

type Status string

const (
	StatusActive  Status = "active"
	StatusRevoked Status = "revoked"
	StatusExpired Status = "expired"
)

type Grant struct {
	ID               string
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	AgentID          string
	AgentVersionID   string
	Scopes           []Scope
	Status           Status
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	RevokedAt        *time.Time
	RevocationActor  string
	RevocationReason string
}

type NewGrantParams struct {
	ID               string
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	AgentID          string
	AgentVersionID   string
	Scopes           []Scope
	ExpiresAt        time.Time
	CreatedAt        time.Time
}

func NewGrant(params NewGrantParams) (*Grant, error) {
	for _, field := range []struct{ name, value string }{
		{"id", params.ID}, {"resource_tenant_id", params.ResourceTenantID},
		{"task_id", params.TaskID}, {"execution_id", params.ExecutionID},
		{"agent_id", params.AgentID}, {"agent_version_id", params.AgentVersionID},
	} {
		if field.value == "" {
			return nil, invalid(field.name)
		}
	}
	if params.CreatedAt.IsZero() {
		return nil, invalid("created_at")
	}
	if !params.ExpiresAt.After(params.CreatedAt) {
		return nil, invalid("expires_at")
	}
	scopes, err := normalizeScopes(params.Scopes)
	if err != nil {
		return nil, err
	}
	return &Grant{
		ID: params.ID, ResourceTenantID: params.ResourceTenantID,
		TaskID: params.TaskID, ExecutionID: params.ExecutionID,
		AgentID: params.AgentID, AgentVersionID: params.AgentVersionID,
		Scopes: scopes, Status: StatusActive, ExpiresAt: params.ExpiresAt,
		CreatedAt: params.CreatedAt, UpdatedAt: params.CreatedAt,
	}, nil
}

type AccessRequest struct {
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	AgentID          string
	AgentVersionID   string
	Scope            Scope
	ActorType        ActorType
	ActorID          string
}

func ValidateAccessRequest(request AccessRequest) error {
	for _, value := range []string{
		request.ResourceTenantID, request.TaskID, request.ExecutionID,
		request.AgentID, request.AgentVersionID, request.ActorID,
	} {
		if value == "" {
			return ErrInvalidArgument
		}
	}
	if !validScope(request.Scope) ||
		(request.ActorType != ActorAgent && request.ActorType != ActorSystem && request.ActorType != ActorHuman) {
		return ErrInvalidArgument
	}
	return nil
}

func (g Grant) Authorizes(request AccessRequest, now time.Time) error {
	if g.Status == StatusRevoked {
		return ErrRevoked
	}
	if g.Status == StatusExpired || !now.Before(g.ExpiresAt) {
		return ErrExpired
	}
	if g.Status != StatusActive ||
		g.ResourceTenantID != request.ResourceTenantID ||
		g.TaskID != request.TaskID || g.ExecutionID != request.ExecutionID ||
		g.AgentID != request.AgentID || g.AgentVersionID != request.AgentVersionID ||
		!g.HasScope(request.Scope) {
		return ErrForbidden
	}
	return nil
}

func (g Grant) HasScope(scope Scope) bool {
	for _, candidate := range g.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func (g *Grant) Renew(newExpiry time.Time, actorID string, now time.Time) error {
	if actorID == "" || now.IsZero() || g.Status != StatusActive || !now.Before(g.ExpiresAt) || !newExpiry.After(g.ExpiresAt) {
		return ErrStateConflict
	}
	g.ExpiresAt = newExpiry
	g.UpdatedAt = now
	return nil
}

func (g *Grant) Revoke(actorID, reason string, now time.Time) error {
	if actorID == "" || reason == "" || now.IsZero() || g.Status != StatusActive {
		return ErrStateConflict
	}
	g.Status = StatusRevoked
	g.RevokedAt = &now
	g.RevocationActor = actorID
	g.RevocationReason = reason
	g.UpdatedAt = now
	return nil
}

func (g *Grant) Expire(now time.Time) error {
	if now.IsZero() || g.Status != StatusActive || now.Before(g.ExpiresAt) {
		return ErrStateConflict
	}
	g.Status = StatusExpired
	g.UpdatedAt = now
	return nil
}

func normalizeScopes(scopes []Scope) ([]Scope, error) {
	if len(scopes) == 0 {
		return nil, invalid("scopes")
	}
	seen := make(map[Scope]struct{}, len(scopes))
	for _, scope := range scopes {
		if !validScope(scope) {
			return nil, invalid("scopes")
		}
		seen[scope] = struct{}{}
	}
	result := make([]Scope, 0, len(seen))
	for scope := range seen {
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func validScope(scope Scope) bool {
	switch scope {
	case ScopeTaskRead, ScopeExecutionRead, ScopeExecutionWrite,
		ScopeSubmissionCreate, ScopeReviewRead, ScopeGitWrite:
		return true
	default:
		return false
	}
}

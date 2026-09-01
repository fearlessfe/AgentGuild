package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/participation/domain"
	"github.com/stretchr/testify/require"
)

func TestGrantAuthorizesOnlyExactResourceAgentVersionAndScope(t *testing.T) {
	now := time.Now().UTC()
	grant := newGrant(t, now, now.Add(time.Hour))
	request := accessRequest()
	require.NoError(t, grant.Authorizes(request, now))

	tests := []struct {
		name   string
		mutate func(*domain.AccessRequest)
	}{
		{"tenant", func(r *domain.AccessRequest) { r.ResourceTenantID = "other" }},
		{"task", func(r *domain.AccessRequest) { r.TaskID = "other" }},
		{"execution", func(r *domain.AccessRequest) { r.ExecutionID = "other" }},
		{"agent", func(r *domain.AccessRequest) { r.AgentID = "other" }},
		{"version", func(r *domain.AccessRequest) { r.AgentVersionID = "other" }},
		{"scope", func(r *domain.AccessRequest) { r.Scope = domain.ScopeGitWrite }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := request
			test.mutate(&candidate)
			require.ErrorIs(t, grant.Authorizes(candidate, now), domain.ErrForbidden)
		})
	}
}

func TestGrantExpiryRenewalAndRevocation(t *testing.T) {
	now := time.Now().UTC()
	grant := newGrant(t, now, now.Add(time.Hour))
	require.ErrorIs(t, grant.Authorizes(accessRequest(), now.Add(time.Hour)), domain.ErrExpired)

	require.NoError(t, grant.Renew(now.Add(2*time.Hour), "governor", now.Add(30*time.Minute)))
	require.NoError(t, grant.Authorizes(accessRequest(), now.Add(90*time.Minute)))
	require.NoError(t, grant.Revoke("governor", "abuse", now.Add(100*time.Minute)))
	require.ErrorIs(t, grant.Authorizes(accessRequest(), now.Add(101*time.Minute)), domain.ErrRevoked)
	wrongAgent := accessRequest()
	wrongAgent.AgentID = "other-agent"
	require.ErrorIs(t, grant.Authorizes(wrongAgent, now.Add(101*time.Minute)), domain.ErrForbidden)
}

func TestGrantNormalizesAndRejectsScopes(t *testing.T) {
	now := time.Now().UTC()
	grant := newGrant(t, now, now.Add(time.Hour))
	require.Equal(t, []domain.Scope{domain.ScopeExecutionWrite, domain.ScopeTaskRead}, grant.Scopes)

	params := validGrantParams(now, now.Add(time.Hour))
	params.Scopes = []domain.Scope{"tenant:list"}
	_, err := domain.NewGrant(params)
	require.ErrorIs(t, err, domain.ErrInvalidArgument)
	require.Equal(t, "scopes", domain.FieldOf(err))
}

func newGrant(t *testing.T, createdAt, expiresAt time.Time) *domain.Grant {
	t.Helper()
	grant, err := domain.NewGrant(validGrantParams(createdAt, expiresAt))
	require.NoError(t, err)
	return grant
}

func validGrantParams(createdAt, expiresAt time.Time) domain.NewGrantParams {
	return domain.NewGrantParams{
		ID: "grant-1", ResourceTenantID: "tenant-sponsor", TaskID: "task-1",
		ExecutionID: "execution-1", AgentID: "agent-global", AgentVersionID: "version-global",
		Scopes:    []domain.Scope{domain.ScopeTaskRead, domain.ScopeExecutionWrite, domain.ScopeTaskRead},
		CreatedAt: createdAt, ExpiresAt: expiresAt,
	}
}

func accessRequest() domain.AccessRequest {
	return domain.AccessRequest{
		ResourceTenantID: "tenant-sponsor", TaskID: "task-1", ExecutionID: "execution-1",
		AgentID: "agent-global", AgentVersionID: "version-global",
		Scope: domain.ScopeExecutionWrite, ActorType: domain.ActorAgent, ActorID: "agent-global",
	}
}

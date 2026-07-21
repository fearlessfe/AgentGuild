package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

func TestAgentIdentityIsGlobalAcrossOrganizationMemberships(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	agent, err := domain.NewAgentIdentity("agent-global-1", "guild-bot", "Guild Bot", nil, now)
	require.NoError(t, err)

	first, err := domain.NewOrganizationMembership(agent.ID, "org-a", "owner-a", "a@example.com", "core", []string{"tasks:execute"}, now)
	require.NoError(t, err)
	second, err := domain.NewOrganizationMembership(agent.ID, "org-b", "owner-b", "b@example.com", "community", []string{"tasks:read"}, now)
	require.NoError(t, err)

	require.Equal(t, agent.ID, first.AgentID)
	require.Equal(t, agent.ID, second.AgentID)
	require.NotEqual(t, first.OrganizationID, second.OrganizationID)
	require.Equal(t, domain.AgentPendingActivation, agent.Status)
}

func TestAgentIdentityActivationBindsMatchingGlobalVersion(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	agent, err := domain.NewAgentIdentity("agent-global-1", "guild-bot", "Guild Bot", nil, now)
	require.NoError(t, err)
	version, err := domain.NewGlobalAgentVersion(
		"version-1", agent.ID, 1, "pi", "gpt-5", []string{"go"}, "sha256:config", now,
	)
	require.NoError(t, err)

	require.NoError(t, agent.Activate(version, agent.ID, now.Add(time.Minute)))
	require.Equal(t, domain.AgentActive, agent.Status)
	require.Equal(t, version.ID, agent.CurrentVersionID)
	require.Empty(t, version.TenantID)

	otherVersion, err := domain.NewGlobalAgentVersion(
		"version-2", "other-agent", 1, "pi", "gpt-5", nil, "sha256:other", now,
	)
	require.NoError(t, err)
	pending, err := domain.NewAgentIdentity("agent-global-2", "other", "Other", nil, now)
	require.NoError(t, err)
	require.ErrorIs(t, pending.Activate(otherVersion, pending.ID, now), domain.ErrForbidden)
}

func TestAgentIdentityRenamePreservesStableID(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	agent, err := domain.NewAgentIdentity("agent-global-1", "old-handle", "Old Name", nil, now)
	require.NoError(t, err)

	require.NoError(t, agent.Rename("new-handle", "New Name", "operator-1", now.Add(time.Minute)))
	require.Equal(t, "agent-global-1", agent.ID)
	require.Equal(t, "new-handle", agent.Handle)
	require.Equal(t, "New Name", agent.DisplayName)
	require.Equal(t, "rename", agent.Events[len(agent.Events)-1].Intent)
}

func TestOrganizationMembershipRevocationDoesNotRevokeAgentIdentity(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	agent, err := domain.NewAgentIdentity("agent-global-1", "guild-bot", "Guild Bot", nil, now)
	require.NoError(t, err)
	membership, err := domain.NewOrganizationMembership(agent.ID, "org-a", "owner-a", "a@example.com", "core", nil, now)
	require.NoError(t, err)

	require.NoError(t, membership.Revoke("admin-1", "left organization", now.Add(time.Minute)))
	require.Equal(t, domain.OrganizationMembershipRevoked, membership.Status)
	require.Equal(t, domain.AgentPendingActivation, agent.Status)
	require.Equal(t, "agent-global-1", agent.ID)
}

func TestOrganizationMembershipCopiesScopes(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
	scopes := []string{"tasks:read"}
	membership, err := domain.NewOrganizationMembership("agent-1", "org-a", "owner-a", "a@example.com", "core", scopes, now)
	require.NoError(t, err)

	scopes[0] = "tasks:admin"
	require.Equal(t, []string{"tasks:read"}, membership.Scopes)
}

func TestAgentIdentityConstructorRejectsMissingGlobalFields(t *testing.T) {
	now := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

	agent, err := domain.NewAgentIdentity("", "handle", "name", nil, now)
	require.Nil(t, agent)
	require.Equal(t, "id", domain.FieldOf(err))

	agent, err = domain.NewAgentIdentity("agent-1", "", "name", nil, now)
	require.Nil(t, agent)
	require.Equal(t, "handle", domain.FieldOf(err))
}

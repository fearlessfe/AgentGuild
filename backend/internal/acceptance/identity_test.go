package acceptance

import (
	"context"
	"testing"

	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/stretchr/testify/require"
)

func TestAgentActivationAndLifecycle(t *testing.T) {
	env := Start(t)

	registered := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{
		Name:      "Review Bot",
		Scopes:    []string{"tasks:read"},
		RepoScope: []string{"acme/*"},
	})
	require.NotEmpty(t, registered.ActivationToken)
	require.Equal(t, identitydomain.AgentPendingActivation, registered.Agent.Status)

	activated := env.Identity.ActivateAgent(registered.ActivationToken, ActivateAgentRequest{
		Runtime:           "codex",
		Model:             "gpt-5",
		Capabilities:      []string{"shell"},
		ConfigFingerprint: "fp-1",
	})
	require.NotEmpty(t, activated.Token)
	require.Equal(t, "Bearer", activated.TokenType)

	agent := env.Identity.GetSelf(activated.Token)
	require.Equal(t, registered.Agent.ID, agent.ID)
	require.Equal(t, identitydomain.AgentActive, agent.Status)

	refreshed := env.Identity.RefreshToken(activated.Token)
	require.NotEmpty(t, refreshed.Token)
	require.Equal(t, registered.Agent.ID, refreshed.AgentID)
	require.Equal(t, "Bearer", refreshed.TokenType)

	beat := env.Identity.Heartbeat(refreshed.Token)
	require.NotNil(t, beat.LastSeenAt)

	suspended := env.Identity.SuspendAgent(ownerSession(), registered.Agent.ID, "maintenance")
	require.Equal(t, identitydomain.AgentSuspended, suspended.Status)

	require.Equal(t, "STATE_CONFLICT", env.Identity.RefreshTokenCode(refreshed.Token))

	resumed := env.Identity.ResumeAgent(ownerSession(), registered.Agent.ID)
	require.Equal(t, identitydomain.AgentActive, resumed.Status)

	revoked := env.Identity.RevokeAgent(ownerSession(), registered.Agent.ID, "retired")
	require.Equal(t, identitydomain.AgentRevoked, revoked.Status)
	require.Equal(t, "STATE_CONFLICT", env.Identity.RefreshTokenCode(refreshed.Token))
	require.Equal(t, "STATE_CONFLICT", env.Identity.HeartbeatCode(refreshed.Token))
}

func TestActivationTokenReplayFails(t *testing.T) {
	env := Start(t)

	registered := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{Name: "Bot"})
	env.Identity.ActivateAgent(registered.ActivationToken, ActivateAgentRequest{
		Runtime: "codex",
		Model:   "gpt-5",
	})

	require.Equal(t, "TOKEN_EXPIRED", env.Identity.ActivateAgentCode(registered.ActivationToken, ActivateAgentRequest{
		Runtime: "codex",
		Model:   "gpt-5",
	}))
}

func TestCredentialHashNotLeakedAndUnauthorizedRequestsAreRejected(t *testing.T) {
	env := Start(t)

	registered := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{Name: "Bot"})
	status := env.Identity.GetActivationStatus(ownerSession(), registered.Agent.ID)
	require.Equal(t, identitydomain.AgentPendingActivation, status.Status)
	require.Equal(t, identitydomain.ActivationCredentialPending, status.ActivationStatus)
	require.NotContains(t, env.Identity.LastBody(), registered.ActivationToken)
	require.NotContains(t, env.Identity.LastBody(), "hash")

	require.Equal(t, "UNAUTHORIZED", env.Identity.ListAgentsCode())
	require.Equal(t, "NOT_FOUND", env.Identity.GetActivationStatusCode(otherOwnerSession(), registered.Agent.ID))

	events, err := env.IdentityAuditEvents(registered.Agent.ID)
	require.NoError(t, err)
	require.Contains(t, intentsOf(events), "register")
}

func intentsOf(events []IdentityAuditEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Intent)
	}
	return out
}

func TestIdentityAuditTrailIncludesLifecycleTransitions(t *testing.T) {
	env := Start(t)

	registered := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{Name: "Audit Bot"})
	env.Identity.ActivateAgent(registered.ActivationToken, ActivateAgentRequest{
		Runtime: "codex",
		Model:   "gpt-5",
	})
	env.Identity.SuspendAgent(ownerSession(), registered.Agent.ID, "maintenance")
	env.Identity.ResumeAgent(ownerSession(), registered.Agent.ID)
	env.Identity.RevokeAgent(ownerSession(), registered.Agent.ID, "retired")

	events, err := env.IdentityAuditEvents(registered.Agent.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"register", "activate", "suspend", "resume", "revoke"}, intentsOf(events))
}

func TestIdentityAuditEventsQueryUsesIsolatedTenantScope(t *testing.T) {
	env := Start(t)
	registered := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{Name: "Audit Bot"})

	events, err := env.IdentityAuditEvents(registered.Agent.ID)
	require.NoError(t, err)
	require.NotEmpty(t, events)

	foreign, err := env.DB.Query(context.Background(), `SELECT intent FROM identity_events WHERE tenant_id = $1 AND agent_id = $2`, "tenant-2", registered.Agent.ID)
	require.NoError(t, err)
	defer foreign.Close()
	require.False(t, foreign.Next())
}

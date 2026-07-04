package acceptance

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
	"time"

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
	require.Equal(t, "TOKEN_REVOKED", env.Identity.RefreshTokenCode(refreshed.Token))
	require.Equal(t, "TOKEN_REVOKED", env.Identity.HeartbeatCode(refreshed.Token))
}

func TestSuspendedIdentityIssuedTokenCannotUseTaskOperations(t *testing.T) {
	env := Start(t)
	registered := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{
		Name:   "Suspended Worker",
		Scopes: []string{"tasks:publish", "tasks:claim", "tasks:execute", "tasks:read"},
	})
	activated := env.Identity.ActivateAgent(registered.ActivationToken, ActivateAgentRequest{
		Runtime: "codex",
		Model:   "gpt-5",
	})
	agent := env.AsAgent(activated.Token)
	taskID := env.PublishTask(time.Now().Add(time.Hour))
	claimed := agent.TaskClaim(taskID, "claim-before-suspend")

	env.Identity.SuspendAgent(ownerSession(), registered.Agent.ID, "maintenance")

	require.Equal(t, "STATE_CONFLICT", agent.PublishTaskCode(time.Now().Add(time.Hour), "publish-after-suspend"))
	require.Equal(t, "STATE_CONFLICT", agent.TaskClaimCode(env.PublishTask(time.Now().Add(time.Hour)), "claim-after-suspend"))
	require.Equal(t, "STATE_CONFLICT", agent.StartExecutionCode(claimed.ID, claimed.LeaseGeneration, "start-after-suspend"))
	require.Equal(t, "STATE_CONFLICT", agent.HeartbeatCode(claimed.ID, claimed.LeaseGeneration, "beat-after-suspend"))
	require.Equal(t, "STATE_CONFLICT", agent.GetExecutionCode(claimed.ID))
}

func TestRevokedIdentityIssuedTokenReturnsTokenRevokedForTaskOperations(t *testing.T) {
	env := Start(t)
	registered := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{
		Name:   "Revoked Worker",
		Scopes: []string{"tasks:publish", "tasks:claim", "tasks:execute", "tasks:read"},
	})
	activated := env.Identity.ActivateAgent(registered.ActivationToken, ActivateAgentRequest{
		Runtime: "codex",
		Model:   "gpt-5",
	})
	agent := env.AsAgent(activated.Token)
	taskID := env.PublishTask(time.Now().Add(time.Hour))
	claimed := agent.TaskClaim(taskID, "claim-before-revoke")

	env.Identity.RevokeAgent(ownerSession(), registered.Agent.ID, "retired")

	require.Equal(t, "TOKEN_REVOKED", agent.PublishTaskCode(time.Now().Add(time.Hour), "publish-after-revoke"))
	require.Equal(t, "TOKEN_REVOKED", agent.TaskClaimCode(env.PublishTask(time.Now().Add(time.Hour)), "claim-after-revoke"))
	require.Equal(t, "TOKEN_REVOKED", agent.StartExecutionCode(claimed.ID, claimed.LeaseGeneration, "start-after-revoke"))
	require.Equal(t, "TOKEN_REVOKED", agent.HeartbeatCode(claimed.ID, claimed.LeaseGeneration, "beat-after-revoke"))
	require.Equal(t, "TOKEN_REVOKED", agent.GetExecutionCode(claimed.ID))
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
	var decoded struct {
		Data struct {
			AgentID          string
			Status           string
			ActivationStatus string
		} `json:"data"`
	}
	env.Identity.DecodeLastBody(&decoded)
	require.Equal(t, registered.Agent.ID, decoded.Data.AgentID)
	require.Equal(t, identitydomain.AgentPendingActivation, decoded.Data.Status)
	require.Equal(t, identitydomain.ActivationCredentialPending, decoded.Data.ActivationStatus)
	require.NotContains(t, env.Identity.LastBody(), registered.ActivationToken)

	stored := env.ActivationCredentialRecord(registered.Agent.TenantID, registered.Agent.ID)
	require.Equal(t, identitydomain.ActivationCredentialPending, stored.Status)
	require.NotEmpty(t, stored.HashBase64)
	require.Len(t, stored.Hash, 32)
	require.NotEqual(t, registered.ActivationToken, stored.HashBase64)
	tokenBytes, err := base64.RawURLEncoding.DecodeString(registered.ActivationToken)
	require.NoError(t, err)
	require.NotEqual(t, tokenBytes, stored.Hash)
	wantHash := sha256.Sum256([]byte(registered.ActivationToken))
	require.Equal(t, wantHash[:], stored.Hash)
	require.False(t, stored.HasPlaintextColumn)

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
	tenant1Agent := env.Identity.RegisterAgent(ownerSession(), RegisterAgentRequest{Name: "Tenant 1 Audit Bot"})
	tenant2Agent := env.Identity.RegisterAgent(tenantOwnerSession("tenant-2", "owner-2"), RegisterAgentRequest{Name: "Tenant 2 Audit Bot"})

	require.Equal(t, "NOT_FOUND", env.Identity.GetAgentCode(ownerSession(), tenant2Agent.Agent.ID))
	require.Equal(t, "NOT_FOUND", env.Identity.GetAgentCode(tenantOwnerSession("tenant-2", "owner-2"), tenant1Agent.Agent.ID))
	require.Equal(t, "NOT_FOUND", env.Identity.GetActivationStatusCode(ownerSession(), tenant2Agent.Agent.ID))
	require.Equal(t, "NOT_FOUND", env.Identity.GetActivationStatusCode(tenantOwnerSession("tenant-2", "owner-2"), tenant1Agent.Agent.ID))
	require.Equal(t, "NOT_FOUND", env.Identity.SuspendAgentCode(ownerSession(), tenant2Agent.Agent.ID, "wrong tenant"))
	require.Equal(t, "NOT_FOUND", env.Identity.RevokeAgentCode(tenantOwnerSession("tenant-2", "owner-2"), tenant1Agent.Agent.ID, "wrong tenant"))

	tenant1Agents := env.Identity.ListAgents(ownerSession())
	require.Len(t, tenant1Agents.Items, 1)
	require.Equal(t, tenant1Agent.Agent.ID, tenant1Agents.Items[0].ID)

	tenant2Agents := env.Identity.ListAgents(tenantOwnerSession("tenant-2", "owner-2"))
	require.Len(t, tenant2Agents.Items, 1)
	require.Equal(t, tenant2Agent.Agent.ID, tenant2Agents.Items[0].ID)

	tenant1Events, err := env.IdentityAuditEventsForTenant("tenant-1", tenant1Agent.Agent.ID)
	require.NoError(t, err)
	require.NotEmpty(t, tenant1Events)

	tenant2Events, err := env.IdentityAuditEventsForTenant("tenant-2", tenant2Agent.Agent.ID)
	require.NoError(t, err)
	require.NotEmpty(t, tenant2Events)

	tenant2CrossEvents, err := env.IdentityAuditEventsForTenant("tenant-2", tenant1Agent.Agent.ID)
	require.NoError(t, err)
	require.Empty(t, tenant2CrossEvents)

	tenant1CrossEvents, err := env.IdentityAuditEventsForTenant("tenant-1", tenant2Agent.Agent.ID)
	require.NoError(t, err)
	require.Empty(t, tenant1CrossEvents)
}

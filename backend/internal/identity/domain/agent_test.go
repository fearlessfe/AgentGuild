package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

func manifest(t *testing.T, now time.Time) *domain.AgentVersion {
	t.Helper()
	version, err := domain.NewAgentVersion(
		"agent-1.v1",
		"tenant-1",
		"agent-1",
		1,
		"python-3.12",
		"gpt-4o",
		[]string{"tasks:read", "tasks:execute"},
		"sha256:abc123",
		now,
	)
	require.NoError(t, err)
	return version
}

func TestAgentCannotActivateTwice(t *testing.T) {
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", []string{"tasks:read"}, nil)
	require.NoError(t, err)

	cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
	require.NoError(t, err)

	require.NoError(t, agent.Activate(cred, manifest(t, time.Now()), token, time.Now()))
	require.ErrorIs(t, agent.Activate(cred, manifest(t, time.Now()), token, time.Now()), domain.ErrStateConflict)
}

func TestRevokedAgentCannotResume(t *testing.T) {
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)

	agent.Revoke("admin-1", "compromised", time.Now())
	err = agent.Resume("admin-1", time.Now())
	require.ErrorIs(t, err, domain.ErrStateConflict)
}

func TestAgentStateMachine(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		from    string
		intent  string
		to      string
		wantErr error
	}{
		{name: "pending activates", from: domain.AgentPendingActivation, intent: "activate", to: domain.AgentActive},
		{name: "active suspends", from: domain.AgentActive, intent: "suspend", to: domain.AgentSuspended},
		{name: "suspended resumes", from: domain.AgentSuspended, intent: "resume", to: domain.AgentActive},
		{name: "active revokes", from: domain.AgentActive, intent: "revoke", to: domain.AgentRevoked},
		{name: "suspended revokes", from: domain.AgentSuspended, intent: "revoke", to: domain.AgentRevoked},
		{name: "pending revokes", from: domain.AgentPendingActivation, intent: "revoke", to: domain.AgentRevoked},
		{name: "active cannot activate", from: domain.AgentActive, intent: "activate", wantErr: domain.ErrStateConflict},
		{name: "suspended cannot activate", from: domain.AgentSuspended, intent: "activate", wantErr: domain.ErrStateConflict},
		{name: "revoked cannot suspend", from: domain.AgentRevoked, intent: "suspend", wantErr: domain.ErrStateConflict},
		{name: "revoked cannot resume", from: domain.AgentRevoked, intent: "resume", wantErr: domain.ErrStateConflict},
		{name: "pending cannot suspend", from: domain.AgentPendingActivation, intent: "suspend", wantErr: domain.ErrStateConflict},
		{name: "pending cannot resume", from: domain.AgentPendingActivation, intent: "resume", wantErr: domain.ErrStateConflict},
		{name: "active cannot resume", from: domain.AgentActive, intent: "resume", wantErr: domain.ErrStateConflict},
		{name: "suspended cannot suspend", from: domain.AgentSuspended, intent: "suspend", wantErr: domain.ErrStateConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := agentInState(t, tc.from, now)
			err := applyIntent(t, agent, tc.intent, now)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.to, agent.Status)
		})
	}
}

func TestAgentConstructorRejectsInvalidIdentity(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		tenantID   string
		ownerID    string
		ownerEmail string
		field      string
	}{
		{name: "empty agent ID", tenantID: "tenant-1", ownerID: "owner-1", ownerEmail: "owner@example.com", field: "id"},
		{name: "empty tenant ID", id: "agent-1", ownerID: "owner-1", ownerEmail: "owner@example.com", field: "tenant_id"},
		{name: "empty owner ID", id: "agent-1", tenantID: "tenant-1", ownerEmail: "owner@example.com", field: "owner_id"},
		{name: "empty owner email", id: "agent-1", tenantID: "tenant-1", ownerID: "owner-1", field: "owner_email"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := domain.NewAgent(tt.id, tt.tenantID, tt.ownerID, tt.ownerEmail, "team-a", nil, nil)
			require.Nil(t, agent)
			assertInvalidArgument(t, err, tt.field)
		})
	}
}

func TestAgentActivateRequiresMatchingTenant(t *testing.T) {
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)

	cred, token, err := domain.NewActivationCredential(agent.ID, "tenant-2", time.Hour)
	require.NoError(t, err)

	err = agent.Activate(cred, manifest(t, time.Now()), token, time.Now())
	require.ErrorIs(t, err, domain.ErrForbidden)
}

func TestAgentHeartbeatUpdatesLastSeen(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)

	cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
	require.NoError(t, err)
	require.NoError(t, agent.Activate(cred, manifest(t, time.Now()), token, now))

	heartbeatAt := now.Add(time.Minute)
	require.NoError(t, agent.Heartbeat(heartbeatAt))
	require.NotNil(t, agent.LastSeenAt)
	require.True(t, agent.LastSeenAt.Equal(heartbeatAt))
}

func TestAgentHeartbeatRejectedWhenPendingOrRevoked(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)

	pending, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)
	require.ErrorIs(t, pending.Heartbeat(now), domain.ErrStateConflict)

	revoked, err := domain.NewAgent("agent-2", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)
	revoked.Revoke("admin-1", "compromised", now)
	require.ErrorIs(t, revoked.Heartbeat(now), domain.ErrStateConflict)
}

func TestAgentRevokeIsTerminal(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)

	require.NoError(t, agent.Revoke("admin-1", "compromised", now))
	require.Equal(t, domain.AgentRevoked, agent.Status)

	require.ErrorIs(t, agent.Suspend("admin-1", now), domain.ErrStateConflict)
	require.ErrorIs(t, agent.Resume("admin-1", now), domain.ErrStateConflict)
}

func TestAgentSuspendResumeRequireActor(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)

	cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
	require.NoError(t, err)
	require.NoError(t, agent.Activate(cred, manifest(t, time.Now()), token, now))

	require.ErrorIs(t, agent.Suspend("", now), domain.ErrForbidden)
	require.NoError(t, agent.Suspend("admin-1", now))
	require.ErrorIs(t, agent.Resume("", now), domain.ErrForbidden)
}

func mustNewCredential(t *testing.T, agentID, tenantID string) *domain.ActivationCredential {
	t.Helper()
	cred, token, err := domain.NewActivationCredential(agentID, tenantID, time.Hour)
	require.NoError(t, err)
	_ = token
	return cred
}

func mustNewToken(t *testing.T, agentID, tenantID string) string {
	t.Helper()
	_, token, err := domain.NewActivationCredential(agentID, tenantID, time.Hour)
	require.NoError(t, err)
	return token
}

func agentInState(t *testing.T, status string, now time.Time) *domain.Agent {
	t.Helper()
	agent, err := domain.NewAgent("agent-1", "tenant-1", "owner-1", "owner@example.com", "team-a", nil, nil)
	require.NoError(t, err)

	switch status {
	case domain.AgentPendingActivation:
		return agent
	case domain.AgentActive:
		cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
		require.NoError(t, err)
		require.NoError(t, agent.Activate(cred, manifest(t, time.Now()), token, now))
		return agent
	case domain.AgentSuspended:
		cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
		require.NoError(t, err)
		require.NoError(t, agent.Activate(cred, manifest(t, time.Now()), token, now))
		require.NoError(t, agent.Suspend("admin-1", now))
		return agent
	case domain.AgentRevoked:
		require.NoError(t, agent.Revoke("admin-1", "test", now))
		return agent
	default:
		t.Fatalf("unknown status %q", status)
		return nil
	}
}

func applyIntent(t *testing.T, agent *domain.Agent, intent string, now time.Time) error {
	t.Helper()
	switch intent {
	case "activate":
		cred, token, err := domain.NewActivationCredential(agent.ID, agent.TenantID, time.Hour)
		require.NoError(t, err)
		return agent.Activate(cred, manifest(t, time.Now()), token, now)
	case "suspend":
		return agent.Suspend("admin-1", now)
	case "resume":
		return agent.Resume("admin-1", now)
	case "revoke":
		return agent.Revoke("admin-1", "test", now)
	default:
		t.Fatalf("unknown intent %q", intent)
		return nil
	}
}

func assertInvalidArgument(t *testing.T, err error, field string) {
	t.Helper()
	require.ErrorIs(t, err, domain.Error{Code: "invalid_argument"})
	var domainErr *domain.Error
	require.True(t, errors.As(err, &domainErr))
	require.Equal(t, field, domainErr.Field)
}

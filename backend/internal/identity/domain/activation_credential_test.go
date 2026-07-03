package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"agentguild.dev/agentguild/backend/internal/identity/domain"
)

func TestConsumedCredentialReplayFails(t *testing.T) {
	cred, token, err := domain.NewActivationCredential("agent-1", "tenant-1", time.Hour)
	require.NoError(t, err)

	require.NoError(t, cred.Consume(token, time.Now()))
	require.ErrorIs(t, cred.Consume(token, time.Now()), domain.ErrTokenExpired)
}

func TestActivationCredentialExpiresAfterTTL(t *testing.T) {
	cred, token, err := domain.NewActivationCredential("agent-1", "tenant-1", time.Hour)
	require.NoError(t, err)

	err = cred.Consume(token, cred.ExpiresAt.Add(time.Nanosecond))
	require.ErrorIs(t, err, domain.ErrTokenExpired)
}

func TestActivationCredentialRejectsWrongToken(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	cred, _, err := domain.NewActivationCredential("agent-1", "tenant-1", time.Hour)
	require.NoError(t, err)

	err = cred.Consume("wrong-token", now)
	require.ErrorIs(t, err, domain.ErrTokenExpired)
}

func TestActivationCredentialSingleConsumption(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	cred, token, err := domain.NewActivationCredential("agent-1", "tenant-1", time.Hour)
	require.NoError(t, err)

	require.Equal(t, domain.ActivationCredentialPending, cred.Status)
	require.NoError(t, cred.Consume(token, now))
	require.Equal(t, domain.ActivationCredentialConsumed, cred.Status)
	require.NotNil(t, cred.ConsumedAt)
	require.True(t, cred.ConsumedAt.Equal(now))
}

func TestActivationCredentialConstructorRejectsInvalidIdentity(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		tenantID string
		field    string
	}{
		{name: "empty agent ID", tenantID: "tenant-1", field: "agent_id"},
		{name: "empty tenant ID", agentID: "agent-1", field: "tenant_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cred, token, err := domain.NewActivationCredential(tt.agentID, tt.tenantID, time.Hour)
			require.Nil(t, cred)
			require.Empty(t, token)
			assertInvalidArgument(t, err, tt.field)
		})
	}
}

func TestActivationCredentialTokenFormat(t *testing.T) {
	_, token, err := domain.NewActivationCredential("agent-1", "tenant-1", time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.Len(t, token, 43)
}

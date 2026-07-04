package domain_test

import (
	"testing"

	avdomain "agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"github.com/stretchr/testify/require"
)

func TestCanRollbackTo(t *testing.T) {
	require.True(t, avdomain.CanRollbackTo(avdomain.StatusActive))
	require.True(t, avdomain.CanRollbackTo(avdomain.StatusEligible))
	require.True(t, avdomain.CanRollbackTo(avdomain.StatusRetired))
	require.False(t, avdomain.CanRollbackTo(avdomain.StatusDraft))
	require.False(t, avdomain.CanRollbackTo(avdomain.StatusEvaluating))
	require.False(t, avdomain.CanRollbackTo(avdomain.StatusRejected))
}

func TestRetireActiveVersion(t *testing.T) {
	now := avdomain.Now()
	v := avdomain.NewTestAgentVersion(t, avdomain.StatusActive)

	require.NoError(t, v.Retire(now))
	require.Equal(t, avdomain.StatusRetired, v.StatusValue())
	require.NotNil(t, v.RetiredAt)
}

func TestRetireOnlyFromActive(t *testing.T) {
	v := avdomain.NewTestAgentVersion(t, avdomain.StatusEligible)
	require.ErrorIs(t, v.Retire(avdomain.Now()), avdomain.ErrStateConflict)
}

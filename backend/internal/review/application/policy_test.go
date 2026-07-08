package application_test

import (
	"testing"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	reviewapp "agentguild.dev/agentguild/backend/internal/review/application"
	"github.com/stretchr/testify/require"
)

func TestReviewRequireScopeHumanStillNeedsScope(t *testing.T) {
	policy := reviewapp.Policy{}
	humanWithoutReviewScope := auth.Principal{
		TenantID: "tenant-1",
		Type:     auth.PrincipalTypeHuman,
		OwnerID:  "owner-1",
	}

	require.ErrorIs(t, policy.CanViewSubmission(humanWithoutReviewScope), domain.ErrForbidden)
}

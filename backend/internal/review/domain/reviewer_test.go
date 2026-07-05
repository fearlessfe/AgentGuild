package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

func mustNewReviewerProfile(t *testing.T) *reviewdomain.ReviewerProfile {
	t.Helper()
	profile, err := reviewdomain.NewReviewerProfile("profile-1", "tenant-1", "user-1", []string{"go", "architecture"}, time.Now())
	require.NoError(t, err)
	return profile
}

func TestReviewerProfileConstructorRejectsInvalidFields(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		id       string
		tenantID string
		userID   string
		field    string
	}{
		{name: "empty id", tenantID: "t", userID: "u", field: "id"},
		{name: "empty tenant_id", id: "id", userID: "u", field: "tenant_id"},
		{name: "empty user_id", id: "id", tenantID: "t", field: "user_id"},
		{name: "zero created_at", id: "id", tenantID: "t", userID: "u", field: "created_at"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var nowArg time.Time
			if tc.field != "created_at" {
				nowArg = now
			}
			profile, err := reviewdomain.NewReviewerProfile(tc.id, tc.tenantID, tc.userID, nil, nowArg)
			require.Nil(t, profile)
			assertInvalidArgument(t, err, tc.field)
		})
	}
}

func TestReviewerProfileIsActiveByDefault(t *testing.T) {
	profile := mustNewReviewerProfile(t)
	require.True(t, profile.IsActive)
}

func TestReviewerProfileSetLoadRejectsNegative(t *testing.T) {
	profile := mustNewReviewerProfile(t)
	now := time.Now()
	assertInvalidArgument(t, profile.SetLoad(-1, now), "current_load")
}

func TestReviewerProfileAssignAndRelease(t *testing.T) {
	profile := mustNewReviewerProfile(t)
	now := time.Now()

	require.NoError(t, profile.Assign(now))
	require.Equal(t, 1, profile.CurrentLoad)

	require.NoError(t, profile.Release(now.Add(time.Minute)))
	require.Equal(t, 0, profile.CurrentLoad)
}

func TestReviewerProfileAssignRejectsInactive(t *testing.T) {
	profile := mustNewReviewerProfile(t)
	now := time.Now()
	profile.Deactivate(now)
	require.ErrorIs(t, profile.Assign(now.Add(time.Minute)), appdomain.ErrStateConflict)
}

func TestReviewerProfileReleaseRejectsZeroLoad(t *testing.T) {
	profile := mustNewReviewerProfile(t)
	require.ErrorIs(t, profile.Release(time.Now()), appdomain.ErrStateConflict)
}

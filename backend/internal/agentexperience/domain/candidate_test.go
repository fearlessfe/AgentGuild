package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewExperienceCandidateRequiresRequiredFields(t *testing.T) {
	now := time.Now()
	_, err := NewExperienceCandidate("", "tenant", "agent", "task", "sub", "rev", []byte("evidence"), nil, "public", now)
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.Equal(t, "id", FieldOf(err))
}

func TestCandidateAutoRejectForbidden(t *testing.T) {
	c := NewTestCandidate(t)
	policy := MockSensitivityPolicy{Class: SensitivityForbidden, Reason: "secret detected"}
	require.NoError(t, c.ClassifyAndApply(policy, []byte("evidence-content")))
	require.Equal(t, StatusRejected, c.StatusValue())
	require.NotEmpty(t, c.PolicyReason)

	c2 := NewTestCandidate(t)
	policy2 := MockSensitivityPolicy{Class: SensitivityInternal}
	require.NoError(t, c2.ClassifyAndApply(policy2, []byte("evidence-content")))
	require.Equal(t, StatusPendingReview, c2.StatusValue())

	// forbidden/pending_review cannot approve
	cForbidden := NewTestCandidate(t)
	_ = cForbidden.ClassifyAndApply(MockSensitivityPolicy{Class: SensitivityForbidden, Reason: "secret"}, []byte("evidence-content"))
	require.ErrorIs(t, cForbidden.Approve("r1", time.Now()), ErrStateConflict)

	// rejected by reviewer cannot approve
	c3 := NewTestCandidate(t)
	_ = c3.ClassifyAndApply(MockSensitivityPolicy{Class: SensitivityInternal}, []byte("evidence-content"))
	require.NoError(t, c3.Reject("r1", "noisy", time.Now()))
	require.ErrorIs(t, c3.Approve("r2", time.Now()), ErrStateConflict)

	c4 := NewTestCandidate(t)
	_ = c4.ClassifyAndApply(MockSensitivityPolicy{Class: SensitivityInternal}, []byte("evidence-content"))
	require.NoError(t, c4.Approve("r1", time.Now()))
	require.Equal(t, StatusApproved, c4.StatusValue())
	require.Equal(t, "r1", c4.ReviewedBy)
	require.NotNil(t, c4.ReviewedAt)

	// approved cannot approve again
	require.ErrorIs(t, c4.Approve("r2", time.Now()), ErrStateConflict)
}

func TestCandidateRejectRecordsReason(t *testing.T) {
	c := NewTestCandidate(t)
	_ = c.ClassifyAndApply(MockSensitivityPolicy{Class: SensitivityInternal}, []byte("evidence-content"))
	require.NoError(t, c.Reject("r1", "contains PII", time.Now()))
	require.Equal(t, StatusRejected, c.StatusValue())
	require.Equal(t, "contains PII", c.PolicyReason)
}

func TestCandidateTenantScopeImmutable(t *testing.T) {
	c := NewTestCandidate(t)
	err := c.SetTenantScope("other-tenant")
	require.ErrorIs(t, err, ErrImmutableResource)
}

func TestCandidateContentHash(t *testing.T) {
	c := NewTestCandidate(t)
	require.NotEmpty(t, c.ContentHash)
}

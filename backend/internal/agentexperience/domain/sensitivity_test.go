package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleBasedSensitivityClassifiesForbidden(t *testing.T) {
	policy := NewRuleBasedSensitivityPolicy()
	class, reason := policy.Classify([]byte(`{"log": "password = super-secret-123"}`))
	require.Equal(t, SensitivityForbidden, class)
	require.Contains(t, reason, "password")
}

func TestRuleBasedSensitivityClassifiesRestricted(t *testing.T) {
	policy := NewRuleBasedSensitivityPolicy()
	class, reason := policy.Classify([]byte(`contact: user@example.com`))
	require.Equal(t, SensitivityRestricted, class)
	require.Contains(t, reason, "user@example.com")
}

func TestRuleBasedSensitivityClassifiesPublic(t *testing.T) {
	policy := NewRuleBasedSensitivityPolicy()
	class, reason := policy.Classify([]byte(`general code review feedback`))
	require.Equal(t, SensitivityPublic, class)
	require.Empty(t, reason)
}

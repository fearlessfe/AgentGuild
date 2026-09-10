package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func validCriterionParams() NewCriterionResultParams {
	return NewCriterionResultParams{
		ResourceTenantID: "tenant-1",
		TaskID:           "task-1",
		ExecutionID:      "exec-1",
		CriterionID:      "AC-1",
		Critical:         true,
		VerifierKind:     VerifierCommand,
		Passed:           true,
		SourceKind:       SourceValidationJob,
		SourceID:         "validation-1",
		VerifiedBy:       "validation_worker",
		ObservedAt:       time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestNewCriterionResultRejectsInvalidFields(t *testing.T) {
	cases := []struct {
		name  string
		field string
		mutp  func(*NewCriterionResultParams)
	}{
		{"blank criterion", "criterion_id", func(p *NewCriterionResultParams) { p.CriterionID = "" }},
		{"untrimmed execution", "execution_id", func(p *NewCriterionResultParams) { p.ExecutionID = " exec-1" }},
		{"unknown verifier", "verifier_kind", func(p *NewCriterionResultParams) { p.VerifierKind = "vibes" }},
		{"unknown source", "source_kind", func(p *NewCriterionResultParams) { p.SourceKind = "guess" }},
		{"blank source id", "source_id", func(p *NewCriterionResultParams) { p.SourceID = "" }},
		{"short evidence hash", "evidence_hash", func(p *NewCriterionResultParams) { p.EvidenceHash = "abc" }},
		{"zero observed", "observed_at", func(p *NewCriterionResultParams) { p.ObservedAt = time.Time{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			params := validCriterionParams()
			tc.mutp(&params)
			result, err := NewCriterionResult(params)
			require.Nil(t, result)
			require.Equal(t, "invalid_argument", CodeOf(err))
			require.Equal(t, tc.field, FieldOf(err))
		})
	}
}

func TestNewCriterionResultAcceptsValidFacts(t *testing.T) {
	params := validCriterionParams()
	params.EvidenceHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	result, err := NewCriterionResult(params)
	require.NoError(t, err)
	require.Equal(t, "AC-1", result.CriterionID)
	require.True(t, result.Passed)
	require.Equal(t, VerifierCommand, result.VerifierKind)
}

func TestSummarizeCriteriaSeparatesUnverifiedFromFailed(t *testing.T) {
	spec := []SpecCriterion{
		{ID: "AC-1", Critical: true},
		{ID: "AC-2", Critical: true},
		{ID: "AC-3", Critical: false},
		{ID: "AC-4", Critical: true},
	}
	observed := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	latest := []CriterionResult{
		{CriterionID: "AC-1", Passed: true, ObservedAt: observed},
		{CriterionID: "AC-2", Passed: false, ObservedAt: observed.Add(time.Hour)},
		{CriterionID: "AC-3", Passed: true, ObservedAt: observed},
		// AC-4 从未被验证过。
	}

	coverage := SummarizeCriteria(spec, latest)

	require.Equal(t, 3, coverage.Required)
	require.Equal(t, 1, coverage.RequiredPassed)
	require.Equal(t, 1, coverage.Optional)
	require.Equal(t, 1, coverage.OptionalPassed)
	require.Equal(t, []string{"AC-2"}, coverage.FailedRequired)
	require.Equal(t, []string{"AC-4"}, coverage.Unverified)
	require.Equal(t, observed.Add(time.Hour), coverage.LatestObserved)
	require.False(t, coverage.AllRequiredPassed())
}

func TestAllRequiredPassedGatesReward(t *testing.T) {
	observed := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	t.Run("every required criterion verified and passing", func(t *testing.T) {
		coverage := SummarizeCriteria(
			[]SpecCriterion{{ID: "AC-1", Critical: true}, {ID: "AC-2", Critical: false}},
			[]CriterionResult{
				{CriterionID: "AC-1", Passed: true, ObservedAt: observed},
				{CriterionID: "AC-2", Passed: false, ObservedAt: observed},
			},
		)
		require.True(t, coverage.AllRequiredPassed(), "optional failure must not block release")
	})

	t.Run("unverified required criterion is not a pass", func(t *testing.T) {
		coverage := SummarizeCriteria([]SpecCriterion{{ID: "AC-1", Critical: true}}, nil)
		require.False(t, coverage.AllRequiredPassed())
	})

	t.Run("spec without any required criterion fails closed", func(t *testing.T) {
		coverage := SummarizeCriteria(
			[]SpecCriterion{{ID: "AC-1", Critical: false}},
			[]CriterionResult{{CriterionID: "AC-1", Passed: true, ObservedAt: observed}},
		)
		require.False(t, coverage.AllRequiredPassed())
	})
}

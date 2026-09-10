package application_test

import (
	"testing"
	"time"

	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	"agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/stretchr/testify/require"
)

var factTime = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func baseFact() application.ContributionFact {
	submittedAt := factTime
	return application.ContributionFact{
		ContributionID:       "contribution-1",
		AgentID:              "agent-1",
		AgentVersionID:       "version-1",
		Capability:           "bug",
		CanonicalRepository:  "acme/widgets",
		TaskID:               "task-1",
		ExecutionID:          "execution-1",
		DifficultyClass:      "standard",
		DifficultyMultiplier: 1,
		CreatedAt:            factTime,
		AttemptOrdinal:       1,
		ExecutionStatus:      "accepted",
		SubmittedAt:          &submittedAt,
		TaskDeadline:         factTime.Add(24 * time.Hour),
	}
}

func event(outcome contributiondomain.Outcome, id int64) application.OutcomeFact {
	return application.OutcomeFact{EventID: id, Outcome: outcome, OccurredAt: factTime}
}

func byDimension(items []reputationdomain.Observation, dimension reputationdomain.Dimension) []reputationdomain.Observation {
	filtered := make([]reputationdomain.Observation, 0, len(items))
	for _, item := range items {
		if item.Dimension == dimension {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func TestUnverifiedCriterionProducesNoObservation(t *testing.T) {
	fact := baseFact()
	// 规格里有三条标准，但只有一条被验证过。未验证的两条不得产生任何观测。
	fact.Criteria = []application.CriterionFact{
		{CriterionID: "AC-1", Critical: true, Passed: true, ObservedAt: factTime},
	}
	correctness := byDimension(fact.Observations(), reputationdomain.DimensionCorrectness)
	require.Len(t, correctness, 1)
	require.True(t, correctness[0].Passed)
	require.Equal(t, 1.0, correctness[0].Weight, "必需标准权重为 1")
}

func TestOptionalCriterionWeighsLessThanCritical(t *testing.T) {
	fact := baseFact()
	fact.Criteria = []application.CriterionFact{
		{CriterionID: "AC-1", Critical: true, Passed: true, ObservedAt: factTime},
		{CriterionID: "AC-2", Critical: false, Passed: true, ObservedAt: factTime},
	}
	correctness := byDimension(fact.Observations(), reputationdomain.DimensionCorrectness)
	require.Len(t, correctness, 2)
	require.Equal(t, 1.0, correctness[0].Weight)
	require.Equal(t, 0.5, correctness[1].Weight)
}

func TestSecurityDimensionHasNoObservationWithoutEvidence(t *testing.T) {
	fact := baseFact()
	fact.Criteria = []application.CriterionFact{{CriterionID: "AC-1", Critical: true, Passed: true, ObservedAt: factTime}}
	fact.Events = []application.OutcomeFact{event(contributiondomain.OutcomeMerged, 5)}
	require.Empty(t, byDimension(fact.Observations(), reputationdomain.DimensionSecurity),
		"没有安全步骤也没有安全标准时必须是零样本，而不是一条通过")
}

func TestSecurityDimensionReadsValidationStepAndBoundCriterion(t *testing.T) {
	fact := baseFact()
	fact.ValidationSteps = []application.ValidationStepFact{
		{JobID: "job-1", Step: application.SecurityValidationStep, Passed: false, ObservedAt: factTime},
		{JobID: "job-1", Step: "build", Passed: true, ObservedAt: factTime},
	}
	fact.Criteria = []application.CriterionFact{
		{CriterionID: "AC-9", Critical: true, VerifierRef: application.SecurityValidationStep, Passed: true, ObservedAt: factTime},
	}
	security := byDimension(fact.Observations(), reputationdomain.DimensionSecurity)
	require.Len(t, security, 2, "只有安全步骤与绑定安全步骤的标准计入 security")
	require.False(t, security[0].Passed)
	require.True(t, security[1].Passed)
}

func TestReliabilityRecordsMissedDeadlineAndRepeatedAttempt(t *testing.T) {
	late := factTime.Add(48 * time.Hour)
	fact := baseFact()
	fact.SubmittedAt = &late
	fact.AttemptOrdinal = 2

	reliability := byDimension(fact.Observations(), reputationdomain.DimensionReliability)
	require.Len(t, reliability, 2)
	require.False(t, reliability[0].Passed, "超过截止时间的交付不算可靠")
	require.False(t, reliability[1].Passed, "同一任务的重复尝试记一次失败")
}

func TestReliabilityIgnoresNonTerminalExecution(t *testing.T) {
	fact := baseFact()
	fact.ExecutionStatus = "running"
	require.Empty(t, byDimension(fact.Observations(), reputationdomain.DimensionReliability),
		"进行中的执行既不算成功也不算失败")
}

func TestImpactIsWeightedByDifficultyClassOnly(t *testing.T) {
	fact := baseFact()
	fact.DifficultyClass = "complex"
	fact.DifficultyMultiplier = 1.5
	fact.Events = []application.OutcomeFact{event(contributiondomain.OutcomeMerged, 7)}

	impact := byDimension(fact.Observations(), reputationdomain.DimensionImpact)
	require.Len(t, impact, 1)
	require.True(t, impact[0].Passed)
	require.Equal(t, 1.5, impact[0].Weight)
}

func TestRevertUndoesImpactAndMaintainability(t *testing.T) {
	fact := baseFact()
	fact.Events = []application.OutcomeFact{
		event(contributiondomain.OutcomeMerged, 7),
		event(contributiondomain.OutcomeReverted, 8),
	}
	items := fact.Observations()
	impact := byDimension(items, reputationdomain.DimensionImpact)
	require.Len(t, impact, 1)
	require.False(t, impact[0].Passed)

	maintainability := byDimension(items, reputationdomain.DimensionMaintainability)
	require.Len(t, maintainability, 1)
	require.False(t, maintainability[0].Passed)

	correctness := byDimension(items, reputationdomain.DimensionCorrectness)
	require.Len(t, correctness, 1)
	require.False(t, correctness[0].Passed, "revert 本身就是一条 correctness 失败观测")
}

func TestCollaborationOnlyObservedWhenRevisionWasRequested(t *testing.T) {
	clean := baseFact()
	clean.Events = []application.OutcomeFact{
		event(contributiondomain.OutcomeReviewed, 1),
		event(contributiondomain.OutcomeApproved, 2),
	}
	require.Empty(t, byDimension(clean.Observations(), reputationdomain.DimensionCollaboration),
		"没有返工循环就没有可观测的协作信号")

	revised := baseFact()
	revised.Events = []application.OutcomeFact{
		event(contributiondomain.OutcomeChangesRequested, 1),
		event(contributiondomain.OutcomeApproved, 2),
	}
	collaboration := byDimension(revised.Observations(), reputationdomain.DimensionCollaboration)
	require.Len(t, collaboration, 1)
	require.True(t, collaboration[0].Passed)

	reviewability := byDimension(revised.Observations(), reputationdomain.DimensionReviewability)
	require.Len(t, reviewability, 1)
	require.False(t, reviewability[0].Passed, "被要求返工意味着可审查性不佳")
}

// 同一个 provider 事件被重复投递不得改变任何维度的观测数量。
func TestRedeliveredEventsDoNotChangeObservations(t *testing.T) {
	fact := baseFact()
	fact.Events = []application.OutcomeFact{
		event(contributiondomain.OutcomeCIPassed, 1),
		event(contributiondomain.OutcomeMerged, 2),
	}
	once := fact.Observations()

	replayed := baseFact()
	replayed.Events = []application.OutcomeFact{
		event(contributiondomain.OutcomeCIPassed, 1),
		event(contributiondomain.OutcomeCIPassed, 3),
		event(contributiondomain.OutcomeMerged, 2),
		event(contributiondomain.OutcomeMerged, 4),
	}
	require.Equal(t, once, replayed.Observations())
}

func TestLatestEventIDIsTheHighWaterMark(t *testing.T) {
	fact := baseFact()
	fact.Events = []application.OutcomeFact{event(contributiondomain.OutcomeCIPassed, 11), event(contributiondomain.OutcomeMerged, 4)}
	require.Equal(t, int64(11), fact.LatestEventID())
}

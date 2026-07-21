package application_test

import (
	"encoding/json"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/application"
	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	"github.com/stretchr/testify/require"
)

func TestProjectorBuildsAgentLifetimeAndIndependentVersionPerformance(t *testing.T) {
	projector, err := application.NewProjector(domain.DefaultAlgorithmVersion)
	require.NoError(t, err)
	now := time.Now().UTC()

	facts := []domain.ContributionFacts{
		fact("contribution-1", "version-1", "task-1", "acme/widgets", domain.AttributionVerified,
			event(1, domain.OutcomeCIPassed, false),
			event(2, domain.OutcomeReviewed, true),
			event(3, domain.OutcomeApproved, true),
			event(4, domain.OutcomeMerged, false),
			event(5, domain.OutcomeCIPassed, false)), // repeated outcome does not inflate counts
		fact("contribution-2", "version-1", "task-1", "acme/widgets", domain.AttributionVerified,
			event(6, domain.OutcomeCIFailed, false),
			event(7, domain.OutcomeChangesRequested, true),
			event(8, domain.OutcomeClosed, false)),
		fact("contribution-3", "version-2", "task-2", "acme/engine", domain.AttributionVerified,
			event(9, domain.OutcomeMerged, false),
			event(10, domain.OutcomeReverted, false),
			event(11, domain.OutcomeIssueReopened, false)),
		fact("pending", "version-2", "task-3", "acme/engine", domain.AttributionPendingVerification,
			event(12, domain.OutcomeMerged, false)),
	}

	set, err := projector.Project(facts, now)
	require.NoError(t, err)
	require.Len(t, set.AgentLifetime, 1)
	require.Len(t, set.AgentVersions, 2)

	lifetime := set.AgentLifetime[0]
	require.Equal(t, domain.ProjectionAgentLifetime, lifetime.Scope)
	require.Equal(t, domain.DefaultAlgorithmVersion, lifetime.AlgorithmVersion)
	require.Equal(t, 3, lifetime.OutcomeCounts.Attempts)
	require.Equal(t, 1, lifetime.OutcomeCounts.CIPassed)
	require.Equal(t, 1, lifetime.OutcomeCounts.CIFailed)
	require.Equal(t, 2, lifetime.OutcomeCounts.Merged)
	require.Equal(t, 1, lifetime.OutcomeCounts.Reverted)
	require.Equal(t, 1, lifetime.OutcomeCounts.IssueReopened)
	require.Equal(t, 3, lifetime.QualitySampleSize)
	require.Equal(t, "low", lifetime.SampleSizeHint)
	require.InDelta(t, 1.0/3.0, lifetime.StableMergeRate, 1e-9)
	require.InDelta(t, 0.5, lifetime.CIPassRate, 1e-9)
	require.InDelta(t, 0.5, lifetime.ApprovalRate, 1e-9)
	require.Equal(t, map[string]int{"acme/widgets": 2, "acme/engine": 1}, lifetime.RepositoryDistribution)
	require.Equal(t, map[string]int{"version-1": 2, "version-2": 1}, lifetime.VersionDistribution)
	require.Equal(t, 1, lifetime.AntiGaming.DuplicateTaskAttempts)
	require.Equal(t, 1, lifetime.AntiGaming.SelfOwnedRepositoryCount)
	require.Equal(t, 1, lifetime.AntiGaming.WithoutIndependentFeedback)
	require.Equal(t, 1, lifetime.AntiGaming.RevertedAfterMerge)
	require.Equal(t, 1, lifetime.AntiGaming.IssueReopenedAfterMerge)
	require.InDelta(t, 2.0/3.0, lifetime.AntiGaming.DominantRepositoryShare, 1e-9)
	require.Equal(t, int64(11), lifetime.LatestEventID)

	byVersion := make(map[string]domain.Projection)
	for _, projection := range set.AgentVersions {
		byVersion[projection.AgentVersionID] = projection
	}
	require.Equal(t, 2, byVersion["version-1"].OutcomeCounts.Attempts)
	require.Equal(t, 1, byVersion["version-1"].OutcomeCounts.Merged)
	require.Equal(t, 1, byVersion["version-2"].OutcomeCounts.Attempts)
	require.Equal(t, 1, byVersion["version-2"].OutcomeCounts.Reverted)
	require.Empty(t, byVersion["version-2"].VersionDistribution)
}

func TestProjectorDoesNotInheritLifetimeQualityIntoUnsampledVersion(t *testing.T) {
	projector, err := application.NewProjector("algorithm-v2")
	require.NoError(t, err)
	set, err := projector.Project([]domain.ContributionFacts{
		fact("contribution-1", "version-old", "task-1", "acme/widgets", domain.AttributionVerified,
			event(1, domain.OutcomeMerged, true)),
	}, time.Now().UTC())
	require.NoError(t, err)
	require.Len(t, set.AgentVersions, 1)
	require.Equal(t, "version-old", set.AgentVersions[0].AgentVersionID)
	for _, projection := range set.AgentVersions {
		require.NotEqual(t, "version-new", projection.AgentVersionID)
	}
}

func fact(id, versionID, taskID, repository string, status domain.AttributionStatus, events ...domain.ContributionEvent) domain.ContributionFacts {
	return domain.ContributionFacts{
		Contribution: domain.Contribution{
			ID: id, AgentID: "agent-global", AgentVersionID: versionID,
			TaskID: taskID, CanonicalRepository: repository,
			SelfOwnedRepository: repository == "acme/engine", AttributionStatus: status,
		},
		Events: events,
	}
}

func event(id int64, outcome domain.Outcome, independent bool) domain.ContributionEvent {
	payload, _ := json.Marshal(map[string]bool{"independent_maintainer": independent})
	return domain.ContributionEvent{ID: id, Outcome: outcome, Payload: payload}
}

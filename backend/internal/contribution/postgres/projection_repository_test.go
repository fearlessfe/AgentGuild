package postgres_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/contribution/application"
	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	contributionpostgres "agentguild.dev/agentguild/backend/internal/contribution/postgres"
	"agentguild.dev/agentguild/backend/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestProjectionRepositoryKeepsAlgorithmVersionsAndReplacesOneAtomically(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedContributionFacts(t, db)
	repository := contributionpostgres.NewProjectionRepository(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	v1 := projectionSet("algorithm-v1", now, 2)
	v2 := projectionSet("algorithm-v2", now.Add(time.Minute), 3)
	require.NoError(t, repository.ReplaceAlgorithm(ctx, "algorithm-v1", v1))
	require.NoError(t, repository.ReplaceAlgorithm(ctx, "algorithm-v2", v2))

	gotV1, err := repository.GetAgentLifetime(ctx, "agent-global", "algorithm-v1")
	require.NoError(t, err)
	require.Equal(t, 2, gotV1.OutcomeCounts.Attempts)
	require.Equal(t, map[string]int{"acme/widgets": 2}, gotV1.RepositoryDistribution)
	require.Equal(t, map[string]int{"version-global": 2}, gotV1.VersionDistribution)
	require.InDelta(t, 1.0, gotV1.StableMergeRate, 1e-9)

	versions, err := repository.ListAgentVersions(ctx, "agent-global", "algorithm-v1")
	require.NoError(t, err)
	require.Len(t, versions, 1)
	require.Equal(t, "version-global", versions[0].AgentVersionID)
	require.Empty(t, versions[0].VersionDistribution)

	v1Replacement := projectionSet("algorithm-v1", now.Add(2*time.Minute), 4)
	require.NoError(t, repository.ReplaceAlgorithm(ctx, "algorithm-v1", v1Replacement))
	gotV1, err = repository.GetAgentLifetime(ctx, "agent-global", "algorithm-v1")
	require.NoError(t, err)
	require.Equal(t, 4, gotV1.OutcomeCounts.Attempts)
	gotV2, err := repository.GetAgentLifetime(ctx, "agent-global", "algorithm-v2")
	require.NoError(t, err)
	require.Equal(t, 3, gotV2.OutcomeCounts.Attempts, "rebuilding v1 must preserve comparison algorithms")
}

func TestRebuilderReplaysVerifiedContributionEventsIntoBothProjectionLevels(t *testing.T) {
	db := testdb.StartPostgres(t)
	seedContributionFacts(t, db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	facts := contributionpostgres.NewRepository(db)
	require.NoError(t, facts.Insert(ctx, newContribution(t, now)))
	merged := newEvent(t, domain.EventMerged, domain.OutcomeMerged, firstCommit,
		"delivery-merged", "pull:52:merged", now.Add(time.Minute))
	storedEvent, _, err := facts.AppendEvent(ctx, merged)
	require.NoError(t, err)

	projector, err := application.NewProjector("replay-v1")
	require.NoError(t, err)
	projections := contributionpostgres.NewProjectionRepository(db)
	rebuilder, err := application.NewRebuilder(facts, projections, projector)
	require.NoError(t, err)
	set, err := rebuilder.Rebuild(ctx, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Len(t, set.AgentLifetime, 1)
	require.Len(t, set.AgentVersions, 1)

	lifetime, err := projections.GetAgentLifetime(ctx, "agent-global", "replay-v1")
	require.NoError(t, err)
	require.Equal(t, 1, lifetime.OutcomeCounts.Attempts)
	require.Equal(t, 1, lifetime.OutcomeCounts.Merged)
	require.Equal(t, storedEvent.ID, lifetime.LatestEventID)
}

func projectionSet(algorithm string, calculatedAt time.Time, attempts int) application.ProjectionSet {
	base := domain.Projection{
		AgentID:          "agent-global",
		AlgorithmVersion: algorithm,
		OutcomeCounts: domain.OutcomeCounts{
			Attempts: attempts,
			Merged:   attempts,
		},
		QualitySampleSize:      attempts,
		SampleSizeHint:         "low",
		StableMergeRate:        1,
		RepositoryDistribution: map[string]int{"acme/widgets": attempts},
		AntiGaming: domain.AntiGamingSignals{
			DominantRepositoryShare: 1,
		},
		LatestEventID: int64(attempts),
		CalculatedAt:  calculatedAt,
	}
	lifetime := base
	lifetime.Scope = domain.ProjectionAgentLifetime
	lifetime.VersionDistribution = map[string]int{"version-global": attempts}
	version := base
	version.Scope = domain.ProjectionAgentVersion
	version.AgentVersionID = "version-global"
	version.VersionDistribution = map[string]int{}
	return application.ProjectionSet{
		AgentLifetime: []domain.Projection{lifetime},
		AgentVersions: []domain.Projection{version},
	}
}

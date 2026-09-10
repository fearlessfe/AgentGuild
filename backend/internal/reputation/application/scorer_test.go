package application_test

import (
	"testing"
	"time"

	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	"agentguild.dev/agentguild/backend/internal/reputation/application"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/stretchr/testify/require"
)

var scoreEvaluatedAt = time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)

func mergedFact(id string, versionID string, eventID int64) application.ContributionFact {
	fact := baseFact()
	fact.ContributionID = id
	fact.AgentVersionID = versionID
	fact.Events = []application.OutcomeFact{
		{EventID: eventID, Outcome: contributiondomain.OutcomeCIPassed, OccurredAt: factTime},
		{EventID: eventID + 1, Outcome: contributiondomain.OutcomeMerged, OccurredAt: factTime},
	}
	fact.Criteria = []application.CriterionFact{
		{CriterionID: "AC-1", Critical: true, Passed: true, ObservedAt: factTime},
	}
	return fact
}

func cardFor(t *testing.T, cards []reputationdomain.ScoreCard, key reputationdomain.ScoreKey) reputationdomain.ScoreCard {
	t.Helper()
	for _, card := range cards {
		if card.Key == key {
			return card
		}
	}
	t.Fatalf("score card %+v not found", key)
	return reputationdomain.ScoreCard{}
}

func newScorer(t *testing.T) *application.Scorer {
	t.Helper()
	scorer, err := application.NewScorer(reputationdomain.DefaultParams())
	require.NoError(t, err)
	return scorer
}

func TestScorerProducesThreeProjectionLayers(t *testing.T) {
	cards, err := newScorer(t).Score([]application.ContributionFact{mergedFact("contribution-1", "version-1", 1)}, scoreEvaluatedAt)
	require.NoError(t, err)
	require.Len(t, cards, 3)

	scopes := map[reputationdomain.ScopeKind]bool{}
	for _, card := range cards {
		scopes[card.Key.Scope] = true
		require.Equal(t, scoreEvaluatedAt, card.EvaluatedAt)
		require.Len(t, card.Dimensions, 7)
	}
	require.True(t, scopes[reputationdomain.ScopeAgentLifetime])
	require.True(t, scopes[reputationdomain.ScopeAgentVersion])
	require.True(t, scopes[reputationdomain.ScopeCapability])
}

// 新建 Agent Version 的质量样本必须归零，但 agent lifetime 不能丢。
func TestNewAgentVersionStartsFromZeroWhileLifetimeIsRetained(t *testing.T) {
	facts := make([]application.ContributionFact, 0, 7)
	for i := 0; i < 6; i++ {
		facts = append(facts, mergedFact("contribution-old-"+string(rune('a'+i)), "version-1", int64(i*10+1)))
	}
	facts = append(facts, mergedFact("contribution-new", "version-2", 100))

	cards, err := newScorer(t).Score(facts, scoreEvaluatedAt)
	require.NoError(t, err)

	lifetime := cardFor(t, cards, reputationdomain.ScoreKey{Scope: reputationdomain.ScopeAgentLifetime, AgentID: "agent-1"})
	require.Equal(t, 7, lifetime.SampleSize)
	require.NotNil(t, lifetime.OverallScore)

	newVersion := cardFor(t, cards, reputationdomain.ScoreKey{
		Scope: reputationdomain.ScopeAgentVersion, AgentID: "agent-1", AgentVersionID: "version-2",
	})
	require.Equal(t, 1, newVersion.SampleSize)
	require.Nil(t, newVersion.OverallScore, "新版本样本不足，不给确定性分数")
	require.Equal(t, "unverified", newVersion.SampleSizeHint)

	oldVersion := cardFor(t, cards, reputationdomain.ScoreKey{
		Scope: reputationdomain.ScopeAgentVersion, AgentID: "agent-1", AgentVersionID: "version-1",
	})
	require.Equal(t, 6, oldVersion.SampleSize)
	require.NotNil(t, oldVersion.OverallScore)
}

func TestScorerIsDeterministicAcrossFactOrder(t *testing.T) {
	facts := []application.ContributionFact{
		mergedFact("contribution-1", "version-1", 1),
		mergedFact("contribution-2", "version-2", 21),
		mergedFact("contribution-3", "version-1", 41),
	}
	forward, err := newScorer(t).Score(facts, scoreEvaluatedAt)
	require.NoError(t, err)

	reversed := []application.ContributionFact{facts[2], facts[0], facts[1]}
	backward, err := newScorer(t).Score(reversed, scoreEvaluatedAt)
	require.NoError(t, err)
	require.Equal(t, forward, backward, "投影不得依赖事实的到达顺序")
}

func TestScorerTracksLatestEventWatermark(t *testing.T) {
	cards, err := newScorer(t).Score([]application.ContributionFact{
		mergedFact("contribution-1", "version-1", 1),
		mergedFact("contribution-2", "version-1", 51),
	}, scoreEvaluatedAt)
	require.NoError(t, err)
	lifetime := cardFor(t, cards, reputationdomain.ScoreKey{Scope: reputationdomain.ScopeAgentLifetime, AgentID: "agent-1"})
	require.Equal(t, int64(52), lifetime.LatestEventID)
}

func TestScorerRejectsFactsWithoutGlobalAttribution(t *testing.T) {
	fact := mergedFact("contribution-1", "", 1)
	_, err := newScorer(t).Score([]application.ContributionFact{fact}, scoreEvaluatedAt)
	require.Error(t, err)

	_, err = newScorer(t).Score(nil, time.Time{})
	require.Error(t, err, "缺少固定评估时刻时必须拒绝，重算才可复现")
}

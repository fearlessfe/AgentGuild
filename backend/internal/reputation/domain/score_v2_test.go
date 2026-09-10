package domain_test

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/stretchr/testify/require"
)

var evaluatedAt = time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)

func lifetimeKey() domain.ScoreKey {
	return domain.ScoreKey{Scope: domain.ScopeAgentLifetime, AgentID: "agent-1"}
}

func observations(dimension domain.Dimension, passed int, failed int) []domain.Observation {
	items := make([]domain.Observation, 0, passed+failed)
	for i := 0; i < passed; i++ {
		items = append(items, domain.Observation{
			Dimension: dimension, Passed: true, Weight: 1,
			ObservedAt: evaluatedAt, EvidenceID: "evidence-pass",
		})
	}
	for i := 0; i < failed; i++ {
		items = append(items, domain.Observation{
			Dimension: dimension, Passed: false, Weight: 1,
			ObservedAt: evaluatedAt, EvidenceID: "evidence-fail",
		})
	}
	return items
}

func dimensionOf(t *testing.T, card domain.ScoreCard, dimension domain.Dimension) domain.DimensionScore {
	t.Helper()
	for _, score := range card.Dimensions {
		if score.Dimension == dimension {
			return score
		}
	}
	t.Fatalf("dimension %s missing from score card", dimension)
	return domain.DimensionScore{}
}

func TestScoreAlwaysEmitsAllSevenDimensions(t *testing.T) {
	card, err := domain.Score(lifetimeKey(), 10, observations(domain.DimensionCorrectness, 10, 0),
		domain.DefaultParams(), evaluatedAt)
	require.NoError(t, err)
	require.Len(t, card.Dimensions, 7)
	for i, dimension := range domain.Dimensions {
		require.Equal(t, dimension, card.Dimensions[i].Dimension, "维度顺序必须固定，否则无法逐字节比对")
	}
}

// 零观测维度是本阶段最重要的不变量：它必须是 sample_size=0 + unverified，
// 并且被排除在总分之外，而不是按 0 分拉低整体。
func TestZeroObservationDimensionIsExcludedAndRenormalized(t *testing.T) {
	params := domain.DefaultParams()
	onlyCorrectness, err := domain.Score(lifetimeKey(), 10, observations(domain.DimensionCorrectness, 10, 0), params, evaluatedAt)
	require.NoError(t, err)

	security := dimensionOf(t, onlyCorrectness, domain.DimensionSecurity)
	require.Equal(t, 0, security.SampleSize)
	require.False(t, security.Observed, "没有安全证据的维度必须是零样本")
	require.Equal(t, "unverified", security.SampleSizeHint)
	require.Zero(t, security.Score)

	correctness := dimensionOf(t, onlyCorrectness, domain.DimensionCorrectness)
	require.NotNil(t, onlyCorrectness.OverallScore)
	require.InDelta(t, correctness.Score, *onlyCorrectness.OverallScore, 1e-9,
		"唯一被观测到的维度重新归一化后就是总分本身")

	// 补上一个同样全通过的安全维度，总分不应下降。
	both := append(observations(domain.DimensionCorrectness, 10, 0), observations(domain.DimensionSecurity, 10, 0)...)
	withSecurity, err := domain.Score(lifetimeKey(), 10, both, params, evaluatedAt)
	require.NoError(t, err)
	require.NotNil(t, withSecurity.OverallScore)
	require.InDelta(t, *onlyCorrectness.OverallScore, *withSecurity.OverallScore, 1e-9)
}

func TestSecurityWithoutEvidenceIsNeverTreatedAsPassed(t *testing.T) {
	card, err := domain.Score(lifetimeKey(), 30, observations(domain.DimensionCorrectness, 30, 0),
		domain.DefaultParams(), evaluatedAt)
	require.NoError(t, err)
	security := dimensionOf(t, card, domain.DimensionSecurity)
	require.Equal(t, 0, security.PassedCount)
	require.Zero(t, security.RawRate, "零行绝不能被平滑成一个正的通过率")
	require.Zero(t, security.LifetimeConfidence)
	require.Zero(t, security.RecentConfidence)
}

func TestSampleSizeThresholds(t *testing.T) {
	params := domain.DefaultParams()
	cases := []struct {
		name       string
		sampleSize int
		hint       string
		hasScore   bool
	}{
		{name: "零样本", sampleSize: 0, hint: "unverified", hasScore: false},
		{name: "四条样本仍未验证", sampleSize: 4, hint: "unverified", hasScore: false},
		{name: "五条样本达到最小门槛", sampleSize: 5, hint: "low", hasScore: true},
		{name: "十九条仍是低样本", sampleSize: 19, hint: "low", hasScore: true},
		{name: "二十条转为高样本", sampleSize: 20, hint: "high", hasScore: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			card, err := domain.Score(lifetimeKey(), testCase.sampleSize,
				observations(domain.DimensionCorrectness, testCase.sampleSize, 0), params, evaluatedAt)
			require.NoError(t, err)
			require.Equal(t, testCase.hint, card.SampleSizeHint)
			if testCase.hasScore {
				require.NotNil(t, card.OverallScore)
				return
			}
			require.Nil(t, card.OverallScore, "样本不足时不得给出确定性分数")
		})
	}
}

func TestSampleSizeUsesContributionCountNotObservationCount(t *testing.T) {
	// 一次交付产生 8 条 criterion 观测，但样本量仍然只有 1。
	single := observations(domain.DimensionCorrectness, 8, 0)
	card, err := domain.Score(lifetimeKey(), 1, single, domain.DefaultParams(), evaluatedAt)
	require.NoError(t, err)
	require.Equal(t, 1, card.SampleSize)
	require.Nil(t, card.OverallScore, "单次交付不得凭借标准条数越过样本门槛")
}

func TestRecentObservationsOutweighOldOnes(t *testing.T) {
	params := domain.DefaultParams()
	old := evaluatedAt.Add(-720 * 24 * time.Hour) // 四个半衰期前

	recentGood := make([]domain.Observation, 0, 20)
	for i := 0; i < 10; i++ {
		recentGood = append(recentGood, domain.Observation{
			Dimension: domain.DimensionCorrectness, Passed: false, Weight: 1,
			ObservedAt: old, EvidenceID: "old-fail",
		})
		recentGood = append(recentGood, domain.Observation{
			Dimension: domain.DimensionCorrectness, Passed: true, Weight: 1,
			ObservedAt: evaluatedAt, EvidenceID: "new-pass",
		})
	}
	card, err := domain.Score(lifetimeKey(), 20, recentGood, params, evaluatedAt)
	require.NoError(t, err)

	correctness := dimensionOf(t, card, domain.DimensionCorrectness)
	require.Greater(t, correctness.RecentConfidence, correctness.LifetimeConfidence,
		"陈旧的失败被衰减后，近期置信度必须高于终身置信度")
	require.Equal(t, 20, correctness.SampleSize)
	require.Equal(t, 10, correctness.PassedCount)
	require.Less(t, correctness.EffectiveSample, 20.0, "衰减后的有效样本必须小于原始计数")
}

func TestScoreIsDeterministicRegardlessOfInputOrder(t *testing.T) {
	params := domain.DefaultParams()
	items := append(observations(domain.DimensionCorrectness, 6, 2), observations(domain.DimensionImpact, 4, 1)...)
	forward, err := domain.Score(lifetimeKey(), 8, items, params, evaluatedAt)
	require.NoError(t, err)

	reversed := make([]domain.Observation, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		reversed = append(reversed, items[i])
	}
	backward, err := domain.Score(lifetimeKey(), 8, reversed, params, evaluatedAt)
	require.NoError(t, err)
	require.Equal(t, forward, backward)
}

func TestScoreRejectsInvalidInput(t *testing.T) {
	params := domain.DefaultParams()
	cases := []struct {
		name        string
		key         domain.ScoreKey
		sampleSize  int
		params      domain.Params
		evaluatedAt time.Time
	}{
		{name: "缺少 agent", key: domain.ScoreKey{Scope: domain.ScopeAgentLifetime}, params: params, evaluatedAt: evaluatedAt},
		{
			name:   "lifetime 不得带版本",
			key:    domain.ScoreKey{Scope: domain.ScopeAgentLifetime, AgentID: "agent-1", AgentVersionID: "version-1"},
			params: params, evaluatedAt: evaluatedAt,
		},
		{
			name:   "capability 不得带版本",
			key:    domain.ScoreKey{Scope: domain.ScopeCapability, AgentID: "agent-1", AgentVersionID: "version-1", Capability: "bug"},
			params: params, evaluatedAt: evaluatedAt,
		},
		{name: "负样本量", key: lifetimeKey(), sampleSize: -1, params: params, evaluatedAt: evaluatedAt},
		{name: "缺少评估时刻", key: lifetimeKey(), params: params},
		{name: "半衰期非正", key: lifetimeKey(), params: domain.Params{AlgorithmVersion: "v", WilsonZ: 1, PriorAlpha: 1, PriorBeta: 1}, evaluatedAt: evaluatedAt},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := domain.Score(testCase.key, testCase.sampleSize, nil, testCase.params, testCase.evaluatedAt)
			require.Error(t, err)
		})
	}
}

func TestParamsRecentWeight(t *testing.T) {
	require.InDelta(t, 0.70, domain.DefaultParams().RecentWeight(), 1e-9)
}

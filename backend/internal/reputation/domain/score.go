package domain

import (
	"sort"
	"time"
)

// DefaultAlgorithmVersionV2 与迁移 000029 预置的参数行一致。v1 的
// DefaultAlgorithmVersion 保持不变，两族投影互不影响。
const DefaultAlgorithmVersionV2 = "2026-09-09-v2"

// Params 是某个 algorithm_version 的全部可调参数。它是**数据**而不是常量：
// 历史投影必须能用当时的参数复算，因此调用方从
// reputation_algorithm_params 读取后传入，纯函数层不持有默认值以外的状态。
type Params struct {
	AlgorithmVersion  string
	DimensionWeights  map[Dimension]float64
	HalfLife          time.Duration
	RecentWeightBps   int
	WilsonZ           float64
	PriorAlpha        float64
	PriorBeta         float64
	MinSampleForScore int
}

// RecentWeight 是 recent_confidence 在最终分数中的占比（doc §4.4 的 0.70）。
func (p Params) RecentWeight() float64 {
	return clamp01(float64(p.RecentWeightBps) / 10000)
}

// Validate 拒绝会产生无意义分数的参数组合。
func (p Params) Validate() error {
	if p.AlgorithmVersion == "" {
		return invalidArgument("algorithm_version")
	}
	if p.HalfLife <= 0 {
		return invalidArgument("half_life_days")
	}
	if p.RecentWeightBps < 0 || p.RecentWeightBps > 10000 {
		return invalidArgument("recent_weight_bps")
	}
	if p.WilsonZ <= 0 {
		return invalidArgument("wilson_z")
	}
	if p.PriorAlpha <= 0 || p.PriorBeta <= 0 {
		return invalidArgument("prior_alpha")
	}
	if p.MinSampleForScore < 0 {
		return invalidArgument("min_sample_for_score")
	}
	for dimension, weight := range p.DimensionWeights {
		if !ValidDimension(dimension) {
			return invalidArgument("dimension_weights")
		}
		if weight < 0 {
			return invalidArgument("dimension_weights")
		}
	}
	return nil
}

// DefaultParams 返回与迁移预置行一致的参数，仅用于测试与无数据库场景。
// 生产路径必须从数据库读取。
func DefaultParams() Params {
	return Params{
		AlgorithmVersion: DefaultAlgorithmVersionV2,
		DimensionWeights: map[Dimension]float64{
			DimensionCorrectness:     0.30,
			DimensionReliability:     0.15,
			DimensionReviewability:   0.10,
			DimensionMaintainability: 0.15,
			DimensionSecurity:        0.10,
			DimensionCollaboration:   0.10,
			DimensionImpact:          0.10,
		},
		HalfLife:          DefaultHalfLife,
		RecentWeightBps:   7000,
		WilsonZ:           1.96,
		PriorAlpha:        0.5,
		PriorBeta:         0.5,
		MinSampleForScore: 5,
	}
}

// ScopeKind 是投影的三层视图（doc §4.2）。
type ScopeKind string

const (
	ScopeAgentLifetime ScopeKind = "agent_lifetime"
	ScopeAgentVersion  ScopeKind = "agent_version"
	ScopeCapability    ScopeKind = "capability"
)

// ScoreKey 唯一标识一条投影。空字符串是有意义的：agent_lifetime 的三个
// 细分键全空，agent_version 只填版本，capability 填能力与仓库。
type ScoreKey struct {
	Scope               ScopeKind
	AgentID             string
	AgentVersionID      string
	Capability          string
	CanonicalRepository string
}

// Valid 校验 scope 与细分键的组合，与迁移里的 CHECK 约束保持一致。
func (k ScoreKey) Valid() bool {
	if k.AgentID == "" {
		return false
	}
	switch k.Scope {
	case ScopeAgentLifetime:
		return k.AgentVersionID == "" && k.Capability == "" && k.CanonicalRepository == ""
	case ScopeAgentVersion:
		return k.AgentVersionID != "" && k.Capability == "" && k.CanonicalRepository == ""
	case ScopeCapability:
		return k.AgentVersionID == "" && (k.Capability != "" || k.CanonicalRepository != "")
	default:
		return false
	}
}

// ScoreCard 是一条完整的 v2 声望投影。Dimensions 永远包含全部七个维度，
// 零观测的维度也在其中（Observed=false），这样"没有证据"是显式记录的事实
// 而不是缺失的行。
type ScoreCard struct {
	Key              ScoreKey
	AlgorithmVersion string
	EvaluatedAt      time.Time
	// OverallScore 为 nil 表示样本不足，不给确定性排名。
	OverallScore   *float64
	SampleSize     int
	SampleSizeHint string
	LatestEventID  int64
	Dimensions     []DimensionScore
}

// Score 把一组观测降解成一条投影。这是整个算法层唯一的入口，纯函数：
// 相同的输入永远产生逐字节相同的结果。
//
// sampleSize 由调用方给出，口径是**参与本次评分的已验证贡献数**，而不是
// 观测条数。一次交付会产生多条 criterion 观测，若用观测数当样本量，
// 单次交付就能越过样本门槛拿到确定性分数。
func Score(key ScoreKey, sampleSize int, observations []Observation, params Params, evaluatedAt time.Time) (ScoreCard, error) {
	if !key.Valid() {
		return ScoreCard{}, invalidArgument("scope")
	}
	if sampleSize < 0 {
		return ScoreCard{}, invalidArgument("sample_size")
	}
	if err := params.Validate(); err != nil {
		return ScoreCard{}, err
	}
	if evaluatedAt.IsZero() {
		return ScoreCard{}, invalidArgument("evaluated_at")
	}

	groups := groupByDimension(observations)
	card := ScoreCard{
		Key:              key,
		AlgorithmVersion: params.AlgorithmVersion,
		EvaluatedAt:      evaluatedAt.UTC(),
		SampleSize:       sampleSize,
		Dimensions:       make([]DimensionScore, 0, len(Dimensions)),
	}

	// 加权平均只在被观测到的维度上做，并按这些维度的权重和重新归一化。
	// 否则"从没做过安全评审"会被当成"安全表现差"，静默拉低每个 Agent。
	var weightedSum, weightTotal float64
	for _, dimension := range Dimensions {
		score := scoreDimension(dimension, groups[dimension], params, evaluatedAt)
		card.Dimensions = append(card.Dimensions, score)
		if !score.Observed {
			continue
		}
		weight := params.DimensionWeights[dimension]
		if weight <= 0 {
			continue
		}
		weightedSum += weight * score.Score
		weightTotal += weight
	}

	card.SampleSizeHint = SampleSizeHint(card.SampleSize)
	if card.SampleSize >= params.MinSampleForScore && weightTotal > 0 {
		overall := clamp01(weightedSum / weightTotal)
		card.OverallScore = &overall
	}
	// 样本不足时既没有总分，展示口径也必须是 unverified。
	if card.OverallScore == nil {
		card.SampleSizeHint = SampleSizeHint(0)
	}
	return card, nil
}

// SortScoreCards 给出稳定的输出顺序，让"删光后重算"能逐字节比对。
func SortScoreCards(cards []ScoreCard) {
	sort.SliceStable(cards, func(i, j int) bool {
		left, right := cards[i].Key, cards[j].Key
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if left.AgentID != right.AgentID {
			return left.AgentID < right.AgentID
		}
		if left.AgentVersionID != right.AgentVersionID {
			return left.AgentVersionID < right.AgentVersionID
		}
		if left.Capability != right.Capability {
			return left.Capability < right.Capability
		}
		return left.CanonicalRepository < right.CanonicalRepository
	})
}

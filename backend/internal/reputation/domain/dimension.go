package domain

import (
	"sort"
	"time"
)

// Dimension 是 doc §4.3 的七个声望维度。
type Dimension string

const (
	DimensionCorrectness     Dimension = "correctness"
	DimensionReliability     Dimension = "reliability"
	DimensionReviewability   Dimension = "reviewability"
	DimensionMaintainability Dimension = "maintainability"
	DimensionSecurity        Dimension = "security"
	DimensionCollaboration   Dimension = "collaboration"
	DimensionImpact          Dimension = "impact"
)

// Dimensions 是维度的规范顺序。投影永远输出全部七行（零观测的维度也输出），
// 顺序固定使重算结果可以逐字节比对。
var Dimensions = []Dimension{
	DimensionCorrectness,
	DimensionReliability,
	DimensionReviewability,
	DimensionMaintainability,
	DimensionSecurity,
	DimensionCollaboration,
	DimensionImpact,
}

// ValidDimension 判断维度名是否在白名单内。事实映射层是唯一的写入口，
// 这里 fail closed 防止未知维度污染投影。
func ValidDimension(dimension Dimension) bool {
	for _, known := range Dimensions {
		if known == dimension {
			return true
		}
	}
	return false
}

// Observation 是所有维度统一的最小事实单元。把 criterion 结果、CI 事件、
// lease 过期、评审循环等异构信号全部降解成同一种形状，是重算能保持简单
// 且可复现的关键。
//
// Weight 是难度/重要性加权（例如 impact 维度按 task_difficulty_classes 的
// multiplier 加权），与时间衰减相乘后得到有效样本量。
type Observation struct {
	Dimension  Dimension
	Passed     bool
	Weight     float64
	ObservedAt time.Time
	EvidenceID string
}

// DimensionScore 是单个维度的完整可解释结果。
type DimensionScore struct {
	Dimension Dimension
	// SampleSize / PassedCount 是未加权的原始观测计数，用于样本门槛判断。
	SampleSize  int
	PassedCount int
	// EffectiveSample / EffectivePassed 是衰减加权后的伪计数。
	EffectiveSample float64
	EffectivePassed float64
	// RawRate 是带先验的平滑通过率：(passed+α)/(passed+failed+α+β)。
	RawRate float64
	// LifetimeConfidence 用未衰减计数，RecentConfidence 用衰减加权计数。
	LifetimeConfidence float64
	RecentConfidence   float64
	Score              float64
	SampleSizeHint     string
	// Observed 为 false 表示零观测：该维度必须被排除在总分之外并重新归一化。
	Observed bool
}

// SampleSizeHint 把样本量翻译成展示口径。阈值与
// contribution/application/projector.go 的 sampleSizeHint 同族（0 / 5 / 20），
// 但 v2 把 [0,5) 整体归入 unverified：低于最小可评分样本时不给确定性排名。
func SampleSizeHint(size int) string {
	switch {
	case size < 5:
		return "unverified"
	case size < 20:
		return "low"
	default:
		return "high"
	}
}

// scoreDimension 计算单个维度。observations 必须已按该维度过滤。
func scoreDimension(dimension Dimension, observations []Observation, params Params, evaluatedAt time.Time) DimensionScore {
	score := DimensionScore{Dimension: dimension, SampleSizeHint: SampleSizeHint(0)}
	var lifetimePassed, lifetimeFailed float64
	for _, observation := range observations {
		weight := observation.Weight
		if weight <= 0 {
			continue
		}
		decayed := weight * DecayWeight(observation.ObservedAt, evaluatedAt, params.HalfLife)
		score.SampleSize++
		score.EffectiveSample += decayed
		if observation.Passed {
			score.PassedCount++
			score.EffectivePassed += decayed
			lifetimePassed += weight
			continue
		}
		lifetimeFailed += weight
	}
	if score.SampleSize == 0 {
		return score
	}

	score.Observed = true
	score.SampleSizeHint = SampleSizeHint(score.SampleSize)
	total := lifetimePassed + lifetimeFailed
	score.RawRate = clamp01((lifetimePassed + params.PriorAlpha) / (total + params.PriorAlpha + params.PriorBeta))
	score.LifetimeConfidence = WilsonLowerBound(lifetimePassed, lifetimeFailed, params.WilsonZ)
	score.RecentConfidence = WilsonLowerBound(score.EffectivePassed, score.EffectiveSample-score.EffectivePassed, params.WilsonZ)
	recent := params.RecentWeight()
	score.Score = clamp01(recent*score.RecentConfidence + (1-recent)*score.LifetimeConfidence)
	return score
}

// groupByDimension 按维度分组并保持稳定顺序，让同一批事实无论以什么顺序
// 到达都得到同一份投影。
func groupByDimension(observations []Observation) map[Dimension][]Observation {
	groups := make(map[Dimension][]Observation, len(Dimensions))
	for _, observation := range observations {
		if !ValidDimension(observation.Dimension) {
			continue
		}
		groups[observation.Dimension] = append(groups[observation.Dimension], observation)
	}
	for dimension := range groups {
		items := groups[dimension]
		sort.SliceStable(items, func(i, j int) bool {
			if !items[i].ObservedAt.Equal(items[j].ObservedAt) {
				return items[i].ObservedAt.Before(items[j].ObservedAt)
			}
			return items[i].EvidenceID < items[j].EvidenceID
		})
		groups[dimension] = items
	}
	return groups
}

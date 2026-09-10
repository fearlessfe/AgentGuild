package domain

import (
	"math"
	"time"
)

// DefaultHalfLife 是近期置信度的半衰期（doc §4.4）。
const DefaultHalfLife = 180 * 24 * time.Hour

// DecayWeight 返回一条观测在评估时刻的权重：2^(-age/halfLife)。
//
// 未来时间戳（observedAt 晚于 evaluatedAt）按满权重 1 处理而不是外推放大，
// 否则一条时钟偏移的观测就能获得超过 1 的权重。halfLife 非正时退化为不衰减。
func DecayWeight(observedAt, evaluatedAt time.Time, halfLife time.Duration) float64 {
	if halfLife <= 0 {
		return 1
	}
	age := evaluatedAt.Sub(observedAt)
	if age <= 0 {
		return 1
	}
	return math.Exp2(-age.Seconds() / halfLife.Seconds())
}

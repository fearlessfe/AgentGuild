package domain

import "math"

// WilsonLowerBound 返回二项比例的 Wilson 置信区间下界。
//
// 它刻意接受非整数的伪计数：时间衰减把每条观测折算成一个 [0,1] 的权重后，
// 有效样本量本来就不是整数，直接代入即可，无需先四舍五入再特判。
// passed/failed 为负数或总数为 0 时返回 0——没有证据就没有信心，
// 而不是给一个乐观的默认值。
func WilsonLowerBound(passed, failed, z float64) float64 {
	if passed < 0 || failed < 0 || z <= 0 {
		return 0
	}
	n := passed + failed
	if n <= 0 {
		return 0
	}
	phat := passed / n
	z2 := z * z
	denominator := 1 + z2/n
	center := phat + z2/(2*n)
	margin := z * math.Sqrt((phat*(1-phat)+z2/(4*n))/n)
	lower := (center - margin) / denominator
	return clamp01(lower)
}

// clamp01 把浮点误差裁回 [0,1]。数据库对所有比率都有 BETWEEN 0 AND 1 约束，
// 让 1e-17 级别的负数在这里就消失，而不是在写库时炸掉。
func clamp01(value float64) float64 {
	if math.IsNaN(value) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

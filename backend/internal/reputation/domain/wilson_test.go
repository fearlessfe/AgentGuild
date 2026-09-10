package domain_test

import (
	"testing"

	"agentguild.dev/agentguild/backend/internal/reputation/domain"
	"github.com/stretchr/testify/require"
)

func TestWilsonLowerBound(t *testing.T) {
	cases := []struct {
		name           string
		passed, failed float64
		z              float64
		expectMin      float64
		expectMax      float64
	}{
		{name: "零样本没有信心", passed: 0, failed: 0, z: 1.96, expectMin: 0, expectMax: 0},
		{name: "全负样本下界为零", passed: 0, failed: 10, z: 1.96, expectMin: 0, expectMax: 0.001},
		// 全通过但样本很少时，下界必须明显低于 1：这正是保守估计的意义。
		{name: "1/1 全通过下界仍低", passed: 1, failed: 0, z: 1.96, expectMin: 0.15, expectMax: 0.25},
		{name: "20/20 全通过下界升高", passed: 20, failed: 0, z: 1.96, expectMin: 0.83, expectMax: 0.85},
		{name: "非整数伪计数直接可用", passed: 3.5, failed: 1.25, z: 1.96, expectMin: 0.30, expectMax: 0.40},
		{name: "负输入返回零", passed: -1, failed: 3, z: 1.96, expectMin: 0, expectMax: 0},
		{name: "非正 z 返回零", passed: 5, failed: 0, z: 0, expectMin: 0, expectMax: 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := domain.WilsonLowerBound(testCase.passed, testCase.failed, testCase.z)
			require.GreaterOrEqual(t, got, testCase.expectMin)
			require.LessOrEqual(t, got, testCase.expectMax)
		})
	}
}

func TestWilsonLowerBoundIsMonotonicInSampleSize(t *testing.T) {
	previous := 0.0
	for _, n := range []float64{1, 2, 5, 10, 50, 200} {
		got := domain.WilsonLowerBound(n, 0, 1.96)
		require.Greater(t, got, previous, "更多的成功样本必须给出更高的下界")
		require.Less(t, got, 1.0)
		previous = got
	}
}

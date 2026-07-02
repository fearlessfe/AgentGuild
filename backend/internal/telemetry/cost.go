// 遥测包定义执行成本观测的抽象接口与通用类型。
package telemetry

import (
	"context"

	"github.com/shopspring/decimal"
)

// Coverage 表示成本覆盖程度。
type Coverage string

const (
	CoverageFull        Coverage = "full"
	CoveragePartial     Coverage = "partial"
	CoverageUnavailable Coverage = "unavailable"
)

// ExecutionRef 用于定位一次执行以读取其成本。
type ExecutionRef struct {
	TenantID       string
	TaskID         string
	ExecutionID    string
	AgentVersionID string
}

// CostObservation 是单次成本观测结果。
type CostObservation struct {
	ObservedCost     decimal.Decimal
	SelfReportedCost decimal.Decimal
	Coverage         Coverage
	Provider         string
	Cursor           string
}

// TraceCostProvider 是成本观测提供者的可替换接口。
type TraceCostProvider interface {
	Observe(context.Context, ExecutionRef) (CostObservation, error)
}

package domain

import (
	"time"
)

// EvaluationRunTask maps one benchmark task of a running evaluation run to the
// real platform task published for it (platform executor). The row is created
// as a plan entry together with the run; the platform task identifier is
// backfilled after the task is published, and the result fields are resolved
// by the harvest worker (phase 2).
type EvaluationRunTask struct {
	TenantID   string
	RunID      string
	TaskRef    string
	TaskID     string
	Ordering   int
	Resolved   bool
	Passed     *bool
	LatencyMs  *float64
	CostCents  *int64
	Details    map[string]any
	CreatedAt  time.Time
	ResolvedAt *time.Time
}

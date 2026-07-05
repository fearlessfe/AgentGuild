package domain

// EvaluationRunResult is the outcome of a single benchmark task within a run.
type EvaluationRunResult struct {
	EvaluationRunID string
	TenantID        string
	TaskRef         string
	Score           float64
	Passed          bool
	Details         map[string]any
}

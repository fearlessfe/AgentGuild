package domain

// EvaluationRunResult is the outcome of a single benchmark task within a run.
type EvaluationRunResult struct {
	EvaluationRunID string         `json:"evaluation_run_id"`
	TenantID        string         `json:"tenant_id"`
	TaskRef         string         `json:"task_ref"`
	Score           float64        `json:"score"`
	Passed          bool           `json:"passed"`
	Details         map[string]any `json:"details,omitempty"`
}

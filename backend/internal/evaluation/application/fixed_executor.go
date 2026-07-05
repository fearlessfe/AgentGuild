package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
)

// FixedBenchmarkExecutor is a synchronous stub executor that returns a passing
// result for every task in the benchmark set. It is intended for local
// development and tests while a real benchmark runner is not yet wired.
type FixedBenchmarkExecutor struct {
	PassThreshold float64
	Score         float64
}

// NewFixedBenchmarkExecutor creates a stub executor with sensible defaults.
func NewFixedBenchmarkExecutor() *FixedBenchmarkExecutor {
	return &FixedBenchmarkExecutor{
		PassThreshold: 0.8,
		Score:         1.0,
	}
}

// Execute returns a passing task result for every task in the benchmark set.
func (e *FixedBenchmarkExecutor) Execute(_ context.Context, benchmarkSet *domain.BenchmarkSet, environmentDigest string) ([]domain.TaskResult, error) {
	tasks := benchmarkSet.Tasks()
	results := make([]domain.TaskResult, 0, len(tasks))
	for _, task := range tasks {
		results = append(results, domain.TaskResult{
			TaskRef: task.TaskRef,
			Score:   e.Score,
			Passed:  e.Score >= e.PassThreshold,
			Details: map[string]any{
				"stub":              true,
				"task_ref":          task.TaskRef,
				"threshold":         e.PassThreshold,
				"environment_digest": environmentDigest,
			},
		})
	}
	return results, nil
}

var _ BenchmarkExecutor = (*FixedBenchmarkExecutor)(nil)

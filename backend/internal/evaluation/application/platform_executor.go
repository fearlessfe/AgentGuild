package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
)

// PlatformBenchmarkExecutorID identifies the platform executor in run
// summaries: evidence comes from real tasks delivered through the platform's
// own task pipeline.
const PlatformBenchmarkExecutorID = "platform"

// PlatformBenchmarkExecutor is the orchestrating executor behind
// EVALUATION_EXECUTOR=platform. EvaluationService.StartEvaluationRun branches
// on the injected EvaluationTaskPublisher before ever calling Execute: it
// publishes one real platform task per benchmark task and leaves the run
// running for the harvest worker (phase 2). Execute therefore only acts as a
// fail-closed guard against miswiring (publisher not injected).
type PlatformBenchmarkExecutor struct{}

// NewPlatformBenchmarkExecutor creates the platform executor.
func NewPlatformBenchmarkExecutor() *PlatformBenchmarkExecutor {
	return &PlatformBenchmarkExecutor{}
}

// ExecutorID returns the platform executor identifier recorded in run
// summaries.
func (e *PlatformBenchmarkExecutor) ExecutorID() string { return PlatformBenchmarkExecutorID }

// Execute always fails: the platform executor is orchestrated asynchronously
// by EvaluationService and never runs benchmarks synchronously.
func (e *PlatformBenchmarkExecutor) Execute(_ context.Context, _ *domain.BenchmarkSet, _ string) ([]domain.TaskResult, error) {
	return nil, &domain.Error{
		Code:    "evaluation_unavailable",
		Message: "platform executor requires an evaluation task publisher; refusing synchronous execution",
	}
}

var _ BenchmarkExecutor = (*PlatformBenchmarkExecutor)(nil)

package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/evaluation/domain"
)

// RejectingBenchmarkExecutorID identifies the disabled executor. It is never
// recorded in a run summary because the executor refuses every run.
const RejectingBenchmarkExecutorID = "unavailable"

// RejectingBenchmarkExecutor fails evaluation runs closed when no benchmark
// executor is configured (EVALUATION_EXECUTOR unset). It mirrors the
// fail-closed posture of VALIDATION_SANDBOX_IMAGE: the service stays up, but
// execution is refused with a clear domain error.
type RejectingBenchmarkExecutor struct{}

// NewRejectingBenchmarkExecutor creates an executor that refuses every run.
func NewRejectingBenchmarkExecutor() *RejectingBenchmarkExecutor {
	return &RejectingBenchmarkExecutor{}
}

// ExecutorID returns the disabled executor identifier.
func (e *RejectingBenchmarkExecutor) ExecutorID() string { return RejectingBenchmarkExecutorID }

// Execute always fails with domain.ErrEvaluationUnavailable.
func (e *RejectingBenchmarkExecutor) Execute(_ context.Context, _ *domain.BenchmarkSet, _ string) ([]domain.TaskResult, error) {
	return nil, domain.ErrEvaluationUnavailable
}

var _ BenchmarkExecutor = (*RejectingBenchmarkExecutor)(nil)

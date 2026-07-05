package validation_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	"agentguild.dev/agentguild/backend/internal/git/validation"
	"github.com/stretchr/testify/require"
)

func TestRunnerSkipsStepWhenConfigVersionMissing(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	runner := validation.NewRunner(nil, nil, nil, validation.WithClock(func() time.Time { return now }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStepStatusSkipped, result.Status)
	require.Contains(t, result.LogSummary, "no runner config for version")
}

func TestRunnerSkipsStepWhenStepMissing(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	registry := validation.Registry{
		"default": {Steps: map[gitdomain.ValidationStep]validation.StepConfig{}},
	}
	runner := validation.NewRunner(registry, nil, nil, validation.WithClock(func() time.Time { return now }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStepStatusSkipped, result.Status)
	require.Contains(t, result.LogSummary, "no runner config for step")
}

func TestRunnerMarksStepSucceededOnZeroExit(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	registry := validation.Registry{
		"default": {
			Steps: map[gitdomain.ValidationStep]validation.StepConfig{
				gitdomain.ValidationStepBuild: {Command: []string{"echo", "ok"}, Timeout: time.Minute},
			},
		},
	}
	executor := &fakeExecutor{output: []byte("ok")}
	runner := validation.NewRunner(registry, &validation.StaticWorkspaceFactory{Dir: "/tmp"}, executor, validation.WithClock(func() time.Time { return now }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStepStatusSucceeded, result.Status)
	require.Equal(t, "ok", result.LogSummary)
	require.Equal(t, "/tmp", executor.lastDir)
	require.Equal(t, []string{"echo", "ok"}, executor.lastArgs)
}

func TestRunnerMarksStepFailedOnNonZeroExit(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	registry := validation.Registry{
		"default": {
			Steps: map[gitdomain.ValidationStep]validation.StepConfig{
				gitdomain.ValidationStepBuild: {Command: []string{"false"}, Timeout: time.Minute},
			},
		},
	}
	executor := &fakeExecutor{err: errors.New("exit status 1"), output: []byte("compile error")}
	runner := validation.NewRunner(registry, &validation.StaticWorkspaceFactory{Dir: "/tmp"}, executor, validation.WithClock(func() time.Time { return now }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStepStatusFailed, result.Status)
	require.Contains(t, result.LogSummary, "compile error")
	require.Contains(t, result.LogSummary, "exit error")
}

func TestRunnerFailsWhenWorkspaceFactoryFails(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	registry := validation.Registry{
		"default": {
			Steps: map[gitdomain.ValidationStep]validation.StepConfig{
				gitdomain.ValidationStepBuild: {Command: []string{"echo", "ok"}, Timeout: time.Minute},
			},
		},
	}
	factory := &fakeWorkspaceFactory{err: errors.New("no workspace")}
	runner := validation.NewRunner(registry, factory, nil, validation.WithClock(func() time.Time { return now }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.Equal(t, gitdomain.ValidationStepStatusFailed, result.Status)
	require.Contains(t, result.LogSummary, "workspace preparation failed")
}

func TestRunnerPropagatesHardGateFromConfig(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	registry := validation.Registry{
		"default": {
			Steps: map[gitdomain.ValidationStep]validation.StepConfig{
				gitdomain.ValidationStepBuild: {Command: []string{"echo", "ok"}, Timeout: time.Minute, HardGate: true},
			},
		},
	}
	executor := &fakeExecutor{output: []byte("ok")}
	runner := validation.NewRunner(registry, &validation.StaticWorkspaceFactory{Dir: "/tmp"}, executor, validation.WithClock(func() time.Time { return now }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.True(t, result.HardGate)
}

func TestRunnerRedactsSecretsFromOutput(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	registry := validation.Registry{
		"default": {
			Steps: map[gitdomain.ValidationStep]validation.StepConfig{
				gitdomain.ValidationStepBuild: {Command: []string{"echo", "token"}, Timeout: time.Minute},
			},
		},
	}
	executor := &fakeExecutor{output: []byte("using token=ghp_123456789012345678901234567890123456 and password=secret")}
	runner := validation.NewRunner(registry, &validation.StaticWorkspaceFactory{Dir: "/tmp"}, executor, validation.WithClock(func() time.Time { return now }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.NotContains(t, result.LogSummary, "ghp_123456789012345678901234567890123456")
	require.NotContains(t, result.LogSummary, "password=secret")
	require.Contains(t, result.LogSummary, "[REDACTED]")
}

func TestRunnerCapturesElapsedResourceUsage(t *testing.T) {
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	job := newJob(now)
	registry := validation.Registry{
		"default": {
			Steps: map[gitdomain.ValidationStep]validation.StepConfig{
				gitdomain.ValidationStepBuild: {Command: []string{"echo", "ok"}, Timeout: time.Minute},
			},
		},
	}
	executor := &fakeExecutor{output: []byte("ok")}
	clk := now
	runner := validation.NewRunner(registry, &validation.StaticWorkspaceFactory{Dir: "/tmp"}, executor, validation.WithClock(func() time.Time { clk = clk.Add(10 * time.Millisecond); return clk }))

	result, err := runner.RunStep(context.Background(), job, gitdomain.ValidationStepBuild)
	require.NoError(t, err)
	require.Contains(t, string(result.ResourceUsage), "elapsed_ms")
}

func TestTempWorkspaceFactoryIsolatesJobs(t *testing.T) {
	ctx := context.Background()
	factory := &validation.TempWorkspaceFactory{}
	now := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	job1, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "owner/repo", "agentguild/exec-1", "head-sha", "default", now, func() string { return "job-1" })
	require.NoError(t, err)
	job2, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "owner/repo", "agentguild/exec-1", "head-sha", "default", now, func() string { return "job-2" })
	require.NoError(t, err)

	dir1, cleanup1, err := factory.Prepare(ctx, job1)
	require.NoError(t, err)
	defer cleanup1()

	dir2, cleanup2, err := factory.Prepare(ctx, job2)
	require.NoError(t, err)
	defer cleanup2()

	require.NotEqual(t, dir1, dir2)
	_, err = os.Stat(dir1)
	require.NoError(t, err)
	_, err = os.Stat(dir2)
	require.NoError(t, err)
}

func newJob(now time.Time) *gitdomain.ValidationJob {
	job, err := gitdomain.NewValidationJob("tenant-1", "sub-1", "owner/repo", "agentguild/exec-1", "head-sha", "default", now, func() string { return "job-1" })
	if err != nil {
		panic(err)
	}
	return job
}

type fakeExecutor struct {
	output   []byte
	err      error
	lastDir  string
	lastArgs []string
	lastEnv  map[string]string
}

func (e *fakeExecutor) Execute(_ context.Context, dir string, env map[string]string, args ...string) ([]byte, error) {
	e.lastDir = dir
	e.lastArgs = args
	e.lastEnv = env
	return e.output, e.err
}

type fakeWorkspaceFactory struct {
	err error
}

func (f *fakeWorkspaceFactory) Prepare(context.Context, *gitdomain.ValidationJob) (string, func(), error) {
	return "", nil, f.err
}

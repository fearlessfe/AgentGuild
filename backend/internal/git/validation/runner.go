// Package validation implements the validation step runner for submission jobs.
package validation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

// secretPatterns redacts likely credentials and keys from runner output.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`gh[ps]_[A-Za-z0-9_]{36,}`),
	regexp.MustCompile(`token=[A-Za-z0-9_\-]+`),
	regexp.MustCompile(`(?i)authorization:\s*bearer\s+\S+`),
	regexp.MustCompile(`(?i)password=\S+`),
	regexp.MustCompile(`-----BEGIN [A-Z ]+ PRIVATE KEY-----[\s\S]*?-----END [A-Z ]+ PRIVATE KEY-----`),
}

// WorkspaceFactory prepares a directory containing the code under validation.
type WorkspaceFactory interface {
	Prepare(ctx context.Context, job *gitdomain.ValidationJob) (dir string, cleanup func(), err error)
}

// Executor runs a command in a working directory.
type Executor interface {
	Execute(ctx context.Context, dir string, env map[string]string, args ...string) (output []byte, err error)
}

// StepConfig describes how to run one validation step.
type StepConfig struct {
	Command  []string
	Timeout  time.Duration
	HardGate bool
	Env      map[string]string
}

// Config maps validation steps to their execution configuration for one
// config_version.
type Config struct {
	Steps map[gitdomain.ValidationStep]StepConfig
}

// Registry selects a Config by config_version.
type Registry map[string]Config

// Runner implements worker.StepRunner by preparing a workspace and executing
// configured commands for each validation step.
type Runner struct {
	registry Registry
	factory  WorkspaceFactory
	executor Executor
	now      func() time.Time
}

// Option customises a Runner.
type Option func(*Runner)

// WithClock overrides the time source.
func WithClock(now func() time.Time) Option {
	return func(r *Runner) { r.now = now }
}

// NewRunner creates a validation runner. A nil factory causes every step to
// fail with a workspace error until a real factory is configured.
func NewRunner(registry Registry, factory WorkspaceFactory, executor Executor, opts ...Option) *Runner {
	if registry == nil {
		registry = Registry{}
	}
	if factory == nil {
		factory = &errorWorkspaceFactory{}
	}
	if executor == nil {
		executor = &CommandExecutor{}
	}
	r := &Runner{
		registry: registry,
		factory:  factory,
		executor: executor,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// RunStep executes the configured command for the step and returns the result.
func (r *Runner) RunStep(ctx context.Context, job *gitdomain.ValidationJob, step gitdomain.ValidationStep) (gitdomain.Step, error) {
	startedAt := r.now()
	cfg, ok := r.registry[job.ConfigVersion]
	if !ok {
		finishedAt := r.now()
		return gitdomain.Step{
			Step:       step,
			Status:     gitdomain.ValidationStepStatusSkipped,
			LogSummary: fmt.Sprintf("no runner config for version %q", job.ConfigVersion),
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
		}, nil
	}
	stepCfg, ok := cfg.Steps[step]
	if !ok {
		finishedAt := r.now()
		return gitdomain.Step{
			Step:       step,
			Status:     gitdomain.ValidationStepStatusSkipped,
			LogSummary: fmt.Sprintf("no runner config for step %q", step),
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
		}, nil
	}

	dir, cleanup, err := r.factory.Prepare(ctx, job)
	if err != nil {
		finishedAt := r.now()
		return gitdomain.Step{
			Step:       step,
			Status:     gitdomain.ValidationStepStatusFailed,
			LogSummary: "workspace preparation failed: " + err.Error(),
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
		}, nil
	}
	if cleanup != nil {
		defer cleanup()
	}

	timeout := stepCfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, runErr := r.executor.Execute(stepCtx, dir, stepCfg.Env, stepCfg.Command...)
	finishedAt := r.now()
	elapsed := finishedAt.Sub(startedAt)

	result := gitdomain.Step{
		Step:       step,
		HardGate:   stepCfg.HardGate,
		StartedAt:  &startedAt,
		FinishedAt: &finishedAt,
		ResourceUsage: []byte(fmt.Sprintf(`{"elapsed_ms":%d}`, elapsed.Milliseconds())),
	}
	if runErr != nil {
		result.Status = gitdomain.ValidationStepStatusFailed
		result.LogSummary = r.redact(r.summarise(output, runErr))
	} else {
		result.Status = gitdomain.ValidationStepStatusSucceeded
		result.LogSummary = r.redact(r.truncate(string(output), 2000))
	}
	return result, nil
}

func (r *Runner) redact(s string) string {
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

func (r *Runner) summarise(output []byte, err error) string {
	var b strings.Builder
	b.WriteString(r.truncate(string(output), 2000))
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	b.WriteString("exit error: ")
	b.WriteString(err.Error())
	return b.String()
}

func (r *Runner) truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...truncated"
}

// CommandExecutor runs commands using os/exec.
type CommandExecutor struct{}

// Execute runs args[0] with args[1:] in dir and returns combined stdout+stderr.
func (e *CommandExecutor) Execute(ctx context.Context, dir string, env map[string]string, args ...string) (output []byte, err error) {
	if len(args) == 0 {
		return nil, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	return buf.Bytes(), runErr
}

type errorWorkspaceFactory struct{}

func (f *errorWorkspaceFactory) Prepare(context.Context, *gitdomain.ValidationJob) (string, func(), error) {
	return "", nil, errors.New("workspace factory not configured")
}

// StaticWorkspaceFactory always returns the same directory with no cleanup.
type StaticWorkspaceFactory struct {
	Dir string
}

// Prepare implements WorkspaceFactory.
func (f *StaticWorkspaceFactory) Prepare(context.Context, *gitdomain.ValidationJob) (string, func(), error) {
	return f.Dir, nil, nil
}

// DefaultRegistry returns a starter configuration that maps common make targets
// to validation steps. Operators can override it by config_version.
func DefaultRegistry() Registry {
	return Registry{
		"default": {
			Steps: map[gitdomain.ValidationStep]StepConfig{
				gitdomain.ValidationStepBuild:          {Command: []string{"make", "build"}, Timeout: 5 * time.Minute, HardGate: true},
				gitdomain.ValidationStepPublicTests:    {Command: []string{"make", "test-public"}, Timeout: 10 * time.Minute, HardGate: true},
				gitdomain.ValidationStepHiddenTests:    {Command: []string{"make", "test-hidden"}, Timeout: 10 * time.Minute, HardGate: true},
				gitdomain.ValidationStepStaticAnalysis: {Command: []string{"make", "lint"}, Timeout: 5 * time.Minute, HardGate: false},
				gitdomain.ValidationStepSecurityScan:   {Command: []string{"make", "security-scan"}, Timeout: 5 * time.Minute, HardGate: true},
			},
		},
	}
}

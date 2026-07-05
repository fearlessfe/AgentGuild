package domain

import (
	"time"

	core "agentguild.dev/agentguild/backend/internal/domain"
)

// ValidationStatus is the lifecycle state of a validation job.
type ValidationStatus string

const (
	// ValidationStatusPending means the job is waiting for a worker to claim it.
	ValidationStatusPending ValidationStatus = "pending"
	// ValidationStatusRunning means a worker has claimed the job.
	ValidationStatusRunning ValidationStatus = "running"
	// ValidationStatusSucceeded means all validation steps passed.
	ValidationStatusSucceeded ValidationStatus = "succeeded"
	// ValidationStatusFailed means at least one hard-gate step failed.
	ValidationStatusFailed ValidationStatus = "failed"
	// ValidationStatusCancelled means the job was cancelled before completion.
	ValidationStatusCancelled ValidationStatus = "cancelled"
)

// ValidationStepStatus is the state of an individual validation step.
type ValidationStepStatus string

const (
	// ValidationStepStatusPending means the step has not started.
	ValidationStepStatusPending ValidationStepStatus = "pending"
	// ValidationStepStatusRunning means the step is in progress.
	ValidationStepStatusRunning ValidationStepStatus = "running"
	// ValidationStepStatusSucceeded means the step passed.
	ValidationStepStatusSucceeded ValidationStepStatus = "succeeded"
	// ValidationStepStatusFailed means the step failed.
	ValidationStepStatusFailed ValidationStepStatus = "failed"
	// ValidationStepStatusSkipped means the step was skipped.
	ValidationStepStatusSkipped ValidationStepStatus = "skipped"
)

// ValidationStep identifies a validation step.
type ValidationStep string

const (
	// ValidationStepBuild compiles the submission.
	ValidationStepBuild ValidationStep = "build"
	// ValidationStepPublicTests runs public tests.
	ValidationStepPublicTests ValidationStep = "public_tests"
	// ValidationStepHiddenTests runs hidden tests.
	ValidationStepHiddenTests ValidationStep = "hidden_tests"
	// ValidationStepStaticAnalysis runs static analysis.
	ValidationStepStaticAnalysis ValidationStep = "static_analysis"
	// ValidationStepSecurityScan runs security scanning.
	ValidationStepSecurityScan ValidationStep = "security_scan"
)

// DefaultValidationSteps is the canonical order of validation steps.
var DefaultValidationSteps = []ValidationStep{
	ValidationStepBuild,
	ValidationStepPublicTests,
	ValidationStepHiddenTests,
	ValidationStepStaticAnalysis,
	ValidationStepSecurityScan,
}

// ValidationJob is the aggregate root for asynchronous validation of a Submission.
type ValidationJob struct {
	ID            string
	TenantID      string
	SubmissionID  string
	ExecutionID   string
	Repo          string
	Branch        string
	CommitSHA     string
	Status        ValidationStatus
	Attempt       int
	ClaimedUntil  *time.Time
	ClaimedBy     *string
	ConfigVersion string
	Steps         []Step
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Step represents the result of one validation step.
type Step struct {
	Step          ValidationStep
	Status        ValidationStepStatus
	HardGate      bool
	LogSummary    string
	ResourceUsage []byte
	StartedAt     *time.Time
	FinishedAt    *time.Time
	CreatedAt     time.Time
}

// NewValidationJob creates a pending validation job for the submission.
func NewValidationJob(tenantID, submissionID, executionID, repo, branch, commitSHA, configVersion string, now time.Time, newID func() string) (*ValidationJob, error) {
	if tenantID == "" {
		return nil, validationJobInvalidArgument("tenant_id")
	}
	if submissionID == "" {
		return nil, validationJobInvalidArgument("submission_id")
	}
	if executionID == "" {
		return nil, validationJobInvalidArgument("execution_id")
	}
	if repo == "" {
		return nil, validationJobInvalidArgument("repo")
	}
	if branch == "" {
		return nil, validationJobInvalidArgument("branch")
	}
	if commitSHA == "" {
		return nil, validationJobInvalidArgument("commit_sha")
	}
	if configVersion == "" {
		return nil, validationJobInvalidArgument("config_version")
	}
	if now.IsZero() {
		return nil, validationJobInvalidArgument("now")
	}
	if newID == nil {
		return nil, validationJobInvalidArgument("new_id")
	}
	id := newID()
	if id == "" {
		return nil, validationJobInvalidArgument("id")
	}

	steps := make([]Step, len(DefaultValidationSteps))
	for i, s := range DefaultValidationSteps {
		steps[i] = Step{Step: s, Status: ValidationStepStatusPending, HardGate: defaultHardGate(s), CreatedAt: now}
	}

	return &ValidationJob{
		ID:            id,
		TenantID:      tenantID,
		SubmissionID:  submissionID,
		ExecutionID:   executionID,
		Repo:          repo,
		Branch:        branch,
		CommitSHA:     commitSHA,
		Status:        ValidationStatusPending,
		Attempt:       0,
		ConfigVersion: configVersion,
		Steps:         steps,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func defaultHardGate(step ValidationStep) bool {
	switch step {
	case ValidationStepBuild, ValidationStepPublicTests, ValidationStepHiddenTests, ValidationStepSecurityScan:
		return true
	case ValidationStepStaticAnalysis:
		return false
	default:
		return false
	}
}

// Claim records that a worker has claimed the job for processing.
func (j *ValidationJob) Claim(workerID string, until time.Time, now time.Time) error {
	if workerID == "" {
		return validationJobInvalidArgument("worker_id")
	}
	if until.IsZero() || until.Before(now) {
		return validationJobInvalidArgument("claimed_until")
	}
	if j.Status != ValidationStatusPending && j.Status != ValidationStatusRunning {
		return core.ErrStateConflict
	}
	if j.ClaimedUntil != nil && j.ClaimedBy != nil && j.ClaimedUntil.After(now) {
		return core.ErrStateConflict
	}
	j.Status = ValidationStatusRunning
	j.ClaimedBy = &workerID
	j.ClaimedUntil = &until
	j.Attempt++
	j.UpdatedAt = now
	return nil
}

// Cancel marks the job as cancelled.
func (j *ValidationJob) Cancel(now time.Time) error {
	if j.Status == ValidationStatusSucceeded || j.Status == ValidationStatusFailed || j.Status == ValidationStatusCancelled {
		return core.ErrStateConflict
	}
	j.Status = ValidationStatusCancelled
	j.ClaimedBy = nil
	j.ClaimedUntil = nil
	j.UpdatedAt = now
	return nil
}

// StartStep marks a step as running.
func (j *ValidationJob) StartStep(step ValidationStep, now time.Time) error {
	if j.Status != ValidationStatusRunning {
		return core.ErrStateConflict
	}
	i := j.stepIndex(step)
	if i < 0 {
		return validationJobInvalidArgument("step")
	}
	if j.Steps[i].Status != ValidationStepStatusPending && j.Steps[i].Status != ValidationStepStatusFailed {
		return core.ErrStateConflict
	}
	j.Steps[i].Status = ValidationStepStatusRunning
	j.Steps[i].StartedAt = &now
	j.UpdatedAt = now
	return nil
}

// FinishStep records the result of a step and advances the job state.
func (j *ValidationJob) FinishStep(step ValidationStep, status ValidationStepStatus, logSummary string, resourceUsage []byte, now time.Time) error {
	if j.Status != ValidationStatusRunning {
		return core.ErrStateConflict
	}
	i := j.stepIndex(step)
	if i < 0 {
		return validationJobInvalidArgument("step")
	}
	if j.Steps[i].Status != ValidationStepStatusRunning {
		return core.ErrStateConflict
	}
	if status != ValidationStepStatusSucceeded && status != ValidationStepStatusFailed && status != ValidationStepStatusSkipped {
		return validationJobInvalidArgument("status")
	}
	j.Steps[i].Status = status
	j.Steps[i].LogSummary = logSummary
	j.Steps[i].ResourceUsage = resourceUsage
	j.Steps[i].FinishedAt = &now
	j.UpdatedAt = now

	// A hard-gate step failure fails the whole job immediately.
	if status == ValidationStepStatusFailed && j.Steps[i].HardGate {
		j.Status = ValidationStatusFailed
		j.ClaimedBy = nil
		j.ClaimedUntil = nil
		return nil
	}

	// Check whether all steps are terminal.
	complete := true
	anyFailed := false
	for _, s := range j.Steps {
		if s.Status == ValidationStepStatusFailed {
			anyFailed = true
			continue
		}
		if s.Status != ValidationStepStatusSucceeded && s.Status != ValidationStepStatusSkipped {
			complete = false
			break
		}
	}
	if complete {
		if anyFailed {
			j.Status = ValidationStatusFailed
		} else {
			j.Status = ValidationStatusSucceeded
		}
		j.ClaimedBy = nil
		j.ClaimedUntil = nil
	}
	return nil
}

func (j *ValidationJob) stepIndex(step ValidationStep) int {
	for i, s := range j.Steps {
		if s.Step == step {
			return i
		}
	}
	return -1
}

func validationJobInvalidArgument(field string) error {
	return &core.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

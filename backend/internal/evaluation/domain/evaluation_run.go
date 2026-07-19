package domain

import (
	"time"
)

// EvaluationRunStatus is the lifecycle state of an EvaluationRun.
type EvaluationRunStatus string

const (
	StatusRunning EvaluationRunStatus = "running"
	StatusPassed  EvaluationRunStatus = "passed"
	StatusFailed  EvaluationRunStatus = "failed"
)

// EvaluationRun represents a single execution of a BenchmarkSet against a
// frozen AgentVersion.
type EvaluationRun struct {
	id                 string
	tenantID           string
	agentVersionID     string
	benchmarkSetID     string
	status             EvaluationRunStatus
	environmentDigest  string
	scoringRuleVersion string
	thresholdResults   []ThresholdResult
	summary            EvaluationSummary
	startedAt          time.Time
	completedAt        *time.Time
}

// ThresholdResult is the outcome of one hard threshold.
type ThresholdResult struct {
	Name     string         `json:"name"`
	Passed   bool           `json:"passed"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

// EvaluationSummary aggregates metrics produced by the scoring rule.
type EvaluationSummary struct {
	PassRate       float64 `json:"pass_rate"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	CostCents      int64   `json:"cost_cents"`
	SecurityPassed bool    `json:"security_passed"`
	// Executor identifies the benchmark executor that produced the task
	// results, so reviewers can tell stub-produced evidence from real runs.
	Executor string         `json:"executor,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// NewEvaluationRun creates a running evaluation run. It freezes the version and
// benchmark set references.
func NewEvaluationRun(
	id, tenantID, agentVersionID, benchmarkSetID,
	environmentDigest, scoringRuleVersion string,
	now time.Time,
) (*EvaluationRun, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if agentVersionID == "" {
		return nil, invalidArgument("agent_version_id")
	}
	if benchmarkSetID == "" {
		return nil, invalidArgument("benchmark_set_id")
	}
	if environmentDigest == "" {
		return nil, invalidArgument("environment_digest")
	}
	if scoringRuleVersion == "" {
		return nil, invalidArgument("scoring_rule_version")
	}
	if now.IsZero() {
		return nil, invalidArgument("started_at")
	}
	return &EvaluationRun{
		id:                 id,
		tenantID:           tenantID,
		agentVersionID:     agentVersionID,
		benchmarkSetID:     benchmarkSetID,
		status:             StatusRunning,
		environmentDigest:  environmentDigest,
		scoringRuleVersion: scoringRuleVersion,
		thresholdResults:   []ThresholdResult{},
		summary:            EvaluationSummary{},
		startedAt:          now,
	}, nil
}

// ID returns the evaluation run identifier.
func (r *EvaluationRun) ID() string { return r.id }

// TenantID returns the tenant identifier.
func (r *EvaluationRun) TenantID() string { return r.tenantID }

// AgentVersionID returns the frozen agent version identifier.
func (r *EvaluationRun) AgentVersionID() string { return r.agentVersionID }

// BenchmarkSetID returns the frozen benchmark set identifier.
func (r *EvaluationRun) BenchmarkSetID() string { return r.benchmarkSetID }

// Status returns the current evaluation run status.
func (r *EvaluationRun) Status() EvaluationRunStatus { return r.status }

// EnvironmentDigest returns the frozen environment digest.
func (r *EvaluationRun) EnvironmentDigest() string { return r.environmentDigest }

// ScoringRuleVersion returns the scoring rule version used.
func (r *EvaluationRun) ScoringRuleVersion() string { return r.scoringRuleVersion }

// ThresholdResults returns the hard threshold results.
func (r *EvaluationRun) ThresholdResults() []ThresholdResult {
	return append([]ThresholdResult(nil), r.thresholdResults...)
}

// Summary returns the aggregate evaluation metrics.
func (r *EvaluationRun) Summary() EvaluationSummary { return r.summary }

// StartedAt returns the start timestamp.
func (r *EvaluationRun) StartedAt() time.Time { return r.startedAt }

// CompletedAt returns the completion timestamp, or nil if still running.
func (r *EvaluationRun) CompletedAt() *time.Time { return r.completedAt }

// Complete finalises the run using the supplied thresholds and summary. It
// transitions running -> passed when all thresholds pass, otherwise failed.
func (r *EvaluationRun) Complete(thresholdResults []ThresholdResult, summary EvaluationSummary) error {
	return r.CompleteAt(thresholdResults, summary, time.Now())
}

// CompleteAt is the testable variant of Complete.
func (r *EvaluationRun) CompleteAt(thresholdResults []ThresholdResult, summary EvaluationSummary, now time.Time) error {
	if r.status != StatusRunning {
		return ErrStateConflict
	}
	if len(thresholdResults) == 0 {
		return invalidArgument("threshold_results")
	}
	r.thresholdResults = append([]ThresholdResult(nil), thresholdResults...)
	r.summary = summary
	if IsAllPassed(thresholdResults) {
		r.status = StatusPassed
	} else {
		r.status = StatusFailed
	}
	r.completedAt = &now
	return nil
}

// IsPassed reports whether the run has passed.
func (r *EvaluationRun) IsPassed() bool {
	return r.status == StatusPassed
}

// IsAllPassed reports whether every threshold result passed.
func IsAllPassed(thresholdResults []ThresholdResult) bool {
	for _, tr := range thresholdResults {
		if !tr.Passed {
			return false
		}
	}
	return true
}

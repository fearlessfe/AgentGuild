package domain

// ScoringRuleVersion identifies a specific scoring rule implementation.
const ScoringRuleVersionV1 = "v1"

var knownScoringRuleVersions = map[string]struct{}{
	ScoringRuleVersionV1: {},
}

// IsKnownScoringRuleVersion reports whether version is a supported rule version.
func IsKnownScoringRuleVersion(version string) bool {
	_, ok := knownScoringRuleVersions[version]
	return ok
}

// TaskResult is the outcome of a single benchmark task.
type TaskResult struct {
	TaskRef    string
	Passed     bool
	LatencyMs  float64
	Score      float64
	CostCents  int64
	IsSecurity bool
	Details    map[string]any
}

// default thresholds for the v1 hard-gate scoring rule.
const (
	v1PassRateThreshold   = 0.8
	v1MaxAvgLatencyMs     = 1000.0
	v1MinSecurityPassRate = 1.0
)

// ApplyScoringRule evaluates task results against the named rule version and
// returns the threshold results and summary. The only supported version is v1.
func ApplyScoringRule(taskResults []TaskResult, ruleVersion string) ([]ThresholdResult, EvaluationSummary) {
	if !IsKnownScoringRuleVersion(ruleVersion) {
		return nil, EvaluationSummary{}
	}

	total := len(taskResults)
	passed := 0
	var totalLatencyMs float64
	var totalCostCents int64
	securityTotal := 0
	securityPassed := 0

	for _, tr := range taskResults {
		if tr.Passed {
			passed++
		}
		totalLatencyMs += tr.LatencyMs
		totalCostCents += tr.CostCents
		if tr.IsSecurity {
			securityTotal++
			if tr.Passed {
				securityPassed++
			}
		}
	}

	var passRate float64
	if total > 0 {
		passRate = float64(passed) / float64(total)
	}

	var avgLatencyMs float64
	if total > 0 {
		avgLatencyMs = totalLatencyMs / float64(total)
	}

	var securityPassRate float64
	if securityTotal > 0 {
		securityPassRate = float64(securityPassed) / float64(securityTotal)
	} else {
		securityPassRate = 1.0
	}

	securityPassedBool := securityPassRate >= v1MinSecurityPassRate

	thresholds := []ThresholdResult{
		{
			Name:   "security_regression",
			Passed: securityPassedBool,
			Evidence: map[string]any{
				"security_tasks_count": securityTotal,
				"security_passed":      securityPassed,
				"threshold":            v1MinSecurityPassRate,
				"actual":               securityPassRate,
			},
		},
		{
			Name:   "pass_rate",
			Passed: passRate >= v1PassRateThreshold,
			Evidence: map[string]any{
				"threshold": v1PassRateThreshold,
				"actual":    passRate,
				"total":     total,
				"passed":    passed,
			},
		},
		{
			Name:   "avg_latency",
			Passed: avgLatencyMs <= v1MaxAvgLatencyMs,
			Evidence: map[string]any{
				"threshold_ms": v1MaxAvgLatencyMs,
				"actual_ms":    avgLatencyMs,
				"total":        total,
			},
		},
	}

	summary := EvaluationSummary{
		PassRate:       passRate,
		AvgLatencyMs:   avgLatencyMs,
		CostCents:      totalCostCents,
		SecurityPassed: securityPassedBool,
		Extra: map[string]any{
			"tasks_run": total,
			"passed":    passed,
		},
	}

	return thresholds, summary
}

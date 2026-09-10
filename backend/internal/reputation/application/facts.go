// Package application 组装声望投影。v1 的 Projector 按 review 结论聚合，
// 本文件起的 v2 事实映射按 doc §4.3 的七个维度重放不可变事实。
package application

import (
	"time"

	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	reputationdomain "agentguild.dev/agentguild/backend/internal/reputation/domain"
)

// SecurityValidationStep 是唯一被采信为安全信号的验证步骤。这里直接引用
// git 领域常量而不是复制字面量，避免步骤改名后声望悄悄失去安全证据。
var SecurityValidationStep = string(gitdomain.ValidationStepSecurityScan)

// CriterionFact 是某个 Execution 上一条验收标准的最新态。
// 只有真正被验证过的标准才会出现在这里：未验证的标准不产生任何观测，
// 绝不能被当成通过。
type CriterionFact struct {
	CriterionID string
	Critical    bool
	// VerifierRef 来自任务规格。绑定到 security_scan 的标准同时计入
	// security 维度。
	VerifierRef string
	Passed      bool
	ObservedAt  time.Time
}

// ValidationStepFact 是一次验证作业中某个步骤的终态。
type ValidationStepFact struct {
	JobID      string
	Step       string
	Passed     bool
	ObservedAt time.Time
}

// OutcomeFact 是 contribution 事件账本里的一条 provider 事实。
type OutcomeFact struct {
	EventID    int64
	Outcome    contributiondomain.Outcome
	OccurredAt time.Time
}

// ContributionFact 汇总一次已验证交付的全部输入。它是重算的最小单元：
// 事实层不变，任何 algorithm_version 都能从同一批 ContributionFact 复算。
type ContributionFact struct {
	ContributionID      string
	AgentID             string
	AgentVersionID      string
	Capability          string
	CanonicalRepository string
	TaskID              string
	ExecutionID         string
	// DifficultyClass / DifficultyMultiplier 只能来自 task_difficulty_classes，
	// 绝不接受 Agent 自报（doc §4.4）。
	DifficultyClass      string
	DifficultyMultiplier float64
	CreatedAt            time.Time
	// AttemptOrdinal 是同一 Agent 在同一任务上的第几次尝试，1 起。
	// 大于 1 的尝试对 reliability 记一次失败。
	AttemptOrdinal  int
	ExecutionStatus string
	SubmittedAt     *time.Time
	TaskDeadline    time.Time
	Events          []OutcomeFact
	Criteria        []CriterionFact
	ValidationSteps []ValidationStepFact
}

// LatestEventID 返回本次交付的事件水位，供增量 worker 判断是否有新事实。
func (f ContributionFact) LatestEventID() int64 {
	var latest int64
	for _, event := range f.Events {
		if event.EventID > latest {
			latest = event.EventID
		}
	}
	return latest
}

// outcomeIndex 记录每个 outcome 第一次出现的时间，避免同一 provider 事件
// 被重复投递时把计数刷高——重复投递不得改变分数。
func (f ContributionFact) outcomeIndex() map[contributiondomain.Outcome]time.Time {
	index := make(map[contributiondomain.Outcome]time.Time, len(f.Events))
	for _, event := range f.Events {
		if existing, seen := index[event.Outcome]; seen && !event.OccurredAt.Before(existing) {
			continue
		}
		index[event.Outcome] = event.OccurredAt
	}
	return index
}

// Observations 把一次交付降解成跨七个维度的统一观测流。
//
// 映射原则：只有平台自己产生或第三方可核验的事实才成为观测；缺失的证据
// 一律不产生观测（零样本），而不是产生一条"通过"或"失败"。
func (f ContributionFact) Observations() []reputationdomain.Observation {
	outcomes := f.outcomeIndex()
	observed := func(outcome contributiondomain.Outcome) (time.Time, bool) {
		at, ok := outcomes[outcome]
		return at, ok
	}
	fallback := f.CreatedAt

	items := make([]reputationdomain.Observation, 0, 8+len(f.Criteria)+len(f.ValidationSteps))
	add := func(dimension reputationdomain.Dimension, passed bool, weight float64, at time.Time, evidence string) {
		if at.IsZero() {
			at = fallback
		}
		items = append(items, reputationdomain.Observation{
			Dimension:  dimension,
			Passed:     passed,
			Weight:     weight,
			ObservedAt: at.UTC(),
			EvidenceID: evidence,
		})
	}

	// correctness：逐条验收标准 + CI 终态 − revert。
	// 必需标准权重 1，可选标准权重 0.5。
	for _, criterion := range f.Criteria {
		weight := 0.5
		if criterion.Critical {
			weight = 1
		}
		add(reputationdomain.DimensionCorrectness, criterion.Passed, weight,
			criterion.ObservedAt, f.ContributionID+"/criterion/"+criterion.CriterionID)
	}
	ciPassedAt, ciPassed := observed(contributiondomain.OutcomeCIPassed)
	ciFailedAt, ciFailed := observed(contributiondomain.OutcomeCIFailed)
	if ciPassed || ciFailed {
		at := ciPassedAt
		if ciFailed {
			at = ciFailedAt
		}
		add(reputationdomain.DimensionCorrectness, ciPassed && !ciFailed, 1, at, f.ContributionID+"/ci")
	}
	revertedAt, reverted := observed(contributiondomain.OutcomeReverted)
	if reverted {
		add(reputationdomain.DimensionCorrectness, false, 1, revertedAt, f.ContributionID+"/revert")
	}

	// reliability：是否按时交付、是否放弃 lease、是否重复尝试。
	if terminalExecution(f.ExecutionStatus) {
		at := f.CreatedAt
		if f.SubmittedAt != nil {
			at = *f.SubmittedAt
		}
		add(reputationdomain.DimensionReliability, f.deliveredOnTime(), 1, at, f.ContributionID+"/delivery")
	}
	if f.AttemptOrdinal > 1 {
		add(reputationdomain.DimensionReliability, false, 1, f.CreatedAt, f.ContributionID+"/repeat-attempt")
	}

	// reviewability：进入评审后是否被要求返工。
	_, reviewed := observed(contributiondomain.OutcomeReviewed)
	changesRequestedAt, changesRequested := observed(contributiondomain.OutcomeChangesRequested)
	approvedAt, approved := observed(contributiondomain.OutcomeApproved)
	if reviewed || changesRequested || approved {
		at := approvedAt
		if changesRequested {
			at = changesRequestedAt
		}
		add(reputationdomain.DimensionReviewability, !changesRequested, 1, at, f.ContributionID+"/review")
	}

	// maintainability：合并之后是否被 revert 或 issue 重开。
	mergedAt, merged := observed(contributiondomain.OutcomeMerged)
	_, reopened := observed(contributiondomain.OutcomeIssueReopened)
	if merged {
		add(reputationdomain.DimensionMaintainability, !reverted && !reopened, 1, mergedAt,
			f.ContributionID+"/post-merge")
	}

	// security：安全验证步骤 + 绑定到安全步骤的验收标准。
	// 没有任何一条时是零样本，不是通过。
	for _, step := range f.ValidationSteps {
		if step.Step != SecurityValidationStep {
			continue
		}
		add(reputationdomain.DimensionSecurity, step.Passed, 1, step.ObservedAt,
			f.ContributionID+"/security-step/"+step.JobID)
	}
	for _, criterion := range f.Criteria {
		if criterion.VerifierRef != SecurityValidationStep {
			continue
		}
		add(reputationdomain.DimensionSecurity, criterion.Passed, 1, criterion.ObservedAt,
			f.ContributionID+"/security-criterion/"+criterion.CriterionID)
	}

	// collaboration：被要求返工之后是否最终获得批准。没有返工循环就没有
	// 可观测的协作信号。
	if changesRequested {
		add(reputationdomain.DimensionCollaboration, approved, 1, changesRequestedAt,
			f.ContributionID+"/revision-cycle")
	}

	// impact：合并且未被 revert，按难度系数加权。
	if merged {
		weight := f.DifficultyMultiplier
		if weight <= 0 {
			weight = 1
		}
		add(reputationdomain.DimensionImpact, !reverted, weight, mergedAt, f.ContributionID+"/impact")
	}
	return items
}

// deliveredOnTime 判断这次交付是否可靠：没有过期/取消，且在截止时间前提交。
func (f ContributionFact) deliveredOnTime() bool {
	switch f.ExecutionStatus {
	case "expired", "cancelled", "rejected":
		return false
	}
	if f.SubmittedAt == nil {
		return false
	}
	if f.TaskDeadline.IsZero() {
		return true
	}
	return !f.SubmittedAt.After(f.TaskDeadline)
}

// terminalExecution 判断 Execution 是否已经不会再变化。只有终态才对
// reliability 产生观测，进行中的执行不该被计为失败。
func terminalExecution(status string) bool {
	switch status {
	case "accepted", "rejected", "expired", "cancelled":
		return true
	default:
		return false
	}
}

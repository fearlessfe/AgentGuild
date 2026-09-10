package application

import (
	"time"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

// Meta 与其他模块的应用服务保持同一形状，便于 transport 层统一封装。
type Meta struct {
	ServerTime      time.Time `json:"server_time"`
	ResourceVersion int64     `json:"resource_version"`
	NextCursor      string    `json:"next_cursor,omitempty"`
}

type Envelope[T any] struct {
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

// EscrowView 是 sponsor 自己的托管余额视图，只对本租户可见。
type EscrowView struct {
	Currency       string `json:"currency"`
	AvailableMinor int64  `json:"available_minor"`
	LockedMinor    int64  `json:"locked_minor"`
}

// PolicyView 是奖励契约的公开视图。它刻意不含 resource_tenant_id：
// 匿名访问者不得从奖励面反推出 sponsor 租户。
type PolicyView struct {
	ID                      string                   `json:"id"`
	TaskID                  string                   `json:"task_id"`
	PolicyHash              string                   `json:"policy_hash"`
	Currency                string                   `json:"currency"`
	SettlementProvider      string                   `json:"settlement_provider"`
	GrossAmountMinor        int64                    `json:"gross_amount_minor"`
	FundedAmountMinor       int64                    `json:"funded_amount_minor"`
	CriterionWeightsBps     []domain.CriterionWeight `json:"criterion_weights_bps"`
	MaintainerShareBps      int                      `json:"maintainer_share_bps"`
	ReviewerPoolShareBps    int                      `json:"reviewer_pool_share_bps"`
	PlatformFeeBps          int                      `json:"platform_fee_bps"`
	DisputeReserveBps       int                      `json:"dispute_reserve_bps"`
	QualityMultiplierMinBps int                      `json:"quality_multiplier_min_bps"`
	QualityMultiplierMaxBps int                      `json:"quality_multiplier_max_bps"`
	ChallengePeriodSeconds  int64                    `json:"challenge_period_seconds"`
	ExpiresAt               time.Time                `json:"expires_at"`
	Status                  string                   `json:"status"`
}

// LockView 是执行级锁定的视图。
type LockView struct {
	ID                string    `json:"id"`
	PolicyID          string    `json:"policy_id"`
	TaskID            string    `json:"task_id"`
	ExecutionID       string    `json:"execution_id"`
	AgentID           string    `json:"agent_id"`
	AgentVersionID    string    `json:"agent_version_id"`
	PolicyHash        string    `json:"policy_hash"`
	Currency          string    `json:"currency"`
	LockedAmountMinor int64     `json:"locked_amount_minor"`
	Status            string    `json:"status"`
	LockedAt          time.Time `json:"locked_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	DecisionHash      string    `json:"decision_hash,omitempty"`
}

// DecisionView 是可被第三方验证的决策视图。所有字段都是公开可复算的
// 摘要与金额，不含任何私有 Issue、代码或评审内容（doc §10）。
type DecisionView struct {
	DecisionHash           string                    `json:"decision_hash"`
	LockID                 string                    `json:"lock_id"`
	PolicyHash             string                    `json:"policy_hash"`
	TaskSpecHash           string                    `json:"task_spec_hash"`
	ContributionHash       string                    `json:"contribution_hash"`
	AlgorithmVersion       string                    `json:"algorithm_version"`
	Currency               string                    `json:"currency"`
	GrossAmountMinor       int64                     `json:"gross_amount_minor"`
	PlatformFeeMinor       int64                     `json:"platform_fee_minor"`
	DisputeReserveMinor    int64                     `json:"dispute_reserve_minor"`
	NetAmountMinor         int64                     `json:"net_amount_minor"`
	AgentAmountMinor       int64                     `json:"agent_amount_minor"`
	MaintainerAmountMinor  int64                     `json:"maintainer_amount_minor"`
	ReviewerPoolMinor      int64                     `json:"reviewer_pool_amount_minor"`
	UnallocatedAmountMinor int64                     `json:"unallocated_amount_minor"`
	QualityMultiplierBps   int                       `json:"quality_multiplier_bps"`
	CriterionResults       []domain.CriterionOutcome `json:"criterion_results"`
	RequiredCriteriaPassed bool                      `json:"required_criteria_passed"`
	RecipientRef           string                    `json:"recipient_ref"`
	ChallengeDeadline      time.Time                 `json:"challenge_deadline"`
	Signature              string                    `json:"signature"`
	SignatureAlgorithm     string                    `json:"signature_algorithm"`
	DecidedAt              time.Time                 `json:"decided_at"`
}

type DisputeView struct {
	ID                       string     `json:"id"`
	LockID                   string     `json:"lock_id"`
	DecisionHash             string     `json:"decision_hash"`
	Status                   string     `json:"status"`
	Reason                   string     `json:"reason"`
	OpenedAt                 time.Time  `json:"opened_at"`
	Resolution               string     `json:"resolution,omitempty"`
	ResolvedAt               *time.Time `json:"resolved_at,omitempty"`
	ResolvedAgentAmountMinor *int64     `json:"resolved_agent_amount_minor,omitempty"`
}

// ChallengeView 只回传 nonce 与有效期。nonce 由服务端签发且一次性。
type ChallengeView struct {
	Nonce     string    `json:"nonce"`
	Chain     string    `json:"chain"`
	Address   string    `json:"address"`
	ExpiresAt time.Time `json:"expires_at"`
}

// DestinationView 暴露 recipient_ref 而非地址本身。
type DestinationView struct {
	ID           string     `json:"id"`
	Chain        string     `json:"chain"`
	RecipientRef string     `json:"recipient_ref"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	VerifiedAt   *time.Time `json:"verified_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

type DestinationPage struct {
	Items []DestinationView `json:"items"`
}

type ReceiptView struct {
	DecisionHash  string    `json:"decision_hash"`
	Provider      string    `json:"provider"`
	Reference     string    `json:"reference"`
	Kind          string    `json:"kind"`
	State         string    `json:"state"`
	Currency      string    `json:"currency"`
	AmountMinor   int64     `json:"amount_minor"`
	RecipientRef  string    `json:"recipient_ref"`
	FailureReason string    `json:"failure_reason,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

func policyView(policy domain.Policy) PolicyView {
	weights := make([]domain.CriterionWeight, len(policy.CriterionWeights))
	copy(weights, policy.CriterionWeights)
	return PolicyView{
		ID: policy.ID, TaskID: policy.TaskID, PolicyHash: policy.PolicyHash,
		Currency: string(policy.Currency), SettlementProvider: policy.SettlementProvider,
		GrossAmountMinor: policy.GrossAmountMinor, FundedAmountMinor: policy.FundedAmountMinor,
		CriterionWeightsBps: weights, MaintainerShareBps: policy.MaintainerShareBps,
		ReviewerPoolShareBps: policy.ReviewerPoolShareBps, PlatformFeeBps: policy.PlatformFeeBps,
		DisputeReserveBps:       policy.DisputeReserveBps,
		QualityMultiplierMinBps: policy.QualityMultiplierMinBps,
		QualityMultiplierMaxBps: policy.QualityMultiplierMaxBps,
		ChallengePeriodSeconds:  int64(policy.ChallengePeriod / time.Second),
		ExpiresAt:               policy.ExpiresAt, Status: string(policy.Status),
	}
}

func lockView(lock domain.Lock, decisionHash string) LockView {
	return LockView{
		ID: lock.ID, PolicyID: lock.PolicyID, TaskID: lock.TaskID,
		ExecutionID: lock.ExecutionID, AgentID: lock.AgentID,
		AgentVersionID: lock.AgentVersionID, PolicyHash: lock.PolicyHash,
		Currency: string(lock.Currency), LockedAmountMinor: lock.LockedAmountMinor,
		Status: string(lock.Status), LockedAt: lock.LockedAt,
		ExpiresAt: lock.ExpiresAt, DecisionHash: decisionHash,
	}
}

func decisionView(decision domain.Decision) DecisionView {
	outcomes := make([]domain.CriterionOutcome, len(decision.CriterionResults))
	copy(outcomes, decision.CriterionResults)
	return DecisionView{
		DecisionHash: decision.DecisionHash, LockID: decision.LockID,
		PolicyHash: decision.PolicyHash, TaskSpecHash: decision.TaskSpecHash,
		ContributionHash: decision.ContributionHash,
		AlgorithmVersion: decision.AlgorithmVersion, Currency: string(decision.Currency),
		GrossAmountMinor:       decision.Allocation.GrossAmountMinor,
		PlatformFeeMinor:       decision.Allocation.PlatformFeeMinor,
		DisputeReserveMinor:    decision.Allocation.DisputeReserveMinor,
		NetAmountMinor:         decision.Allocation.NetAmountMinor,
		AgentAmountMinor:       decision.Allocation.AgentAmountMinor,
		MaintainerAmountMinor:  decision.Allocation.MaintainerAmountMinor,
		ReviewerPoolMinor:      decision.Allocation.ReviewerPoolAmountMinor,
		UnallocatedAmountMinor: decision.Allocation.UnallocatedAmountMinor,
		QualityMultiplierBps:   decision.QualityMultiplierBps,
		CriterionResults:       outcomes,
		RequiredCriteriaPassed: decision.RequiredCriteriaPassed,
		RecipientRef:           decision.RecipientRef,
		ChallengeDeadline:      decision.ChallengeDeadline,
		Signature:              decision.Signature,
		SignatureAlgorithm:     decision.SignatureAlgorithm,
		DecidedAt:              decision.DecidedAt,
	}
}

func disputeView(dispute domain.Dispute) DisputeView {
	view := DisputeView{
		ID: dispute.ID, LockID: dispute.LockID, DecisionHash: dispute.DecisionHash,
		Status: string(dispute.Status), Reason: dispute.Reason, OpenedAt: dispute.OpenedAt,
		ResolvedAt: dispute.ResolvedAt, ResolvedAgentAmountMinor: dispute.ResolvedAgentAmountMinor,
	}
	if dispute.Resolution != nil {
		view.Resolution = string(*dispute.Resolution)
	}
	return view
}

func destinationView(destination domain.Destination) DestinationView {
	return DestinationView{
		ID: destination.ID, Chain: destination.Chain,
		RecipientRef: destination.RecipientRef, Status: string(destination.Status),
		CreatedAt: destination.CreatedAt, VerifiedAt: destination.VerifiedAt,
		RevokedAt: destination.RevokedAt,
	}
}

func receiptView(receipt domain.Receipt) ReceiptView {
	return ReceiptView{
		DecisionHash: receipt.DecisionHash, Provider: receipt.Provider,
		Reference: receipt.Reference, Kind: string(receipt.Kind),
		State: string(receipt.State), Currency: string(receipt.Currency),
		AmountMinor: receipt.AmountMinor, RecipientRef: receipt.RecipientRef,
		FailureReason: receipt.FailureReason, OccurredAt: receipt.OccurredAt,
	}
}

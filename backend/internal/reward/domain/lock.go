package domain

import (
	"strings"
	"time"
)

// LockStatus 是执行级锁定的状态。
type LockStatus string

const (
	LockLocked     LockStatus = "locked"
	LockReleasable LockStatus = "releasable"
	LockReleased   LockStatus = "released"
	LockRefunded   LockStatus = "refunded"
	LockDisputed   LockStatus = "disputed"
	LockExpired    LockStatus = "expired"
)

// LockIntent 是锁上允许的迁移意图。
type LockIntent string

const (
	LockIntentMarkReleasable LockIntent = "mark_releasable"
	LockIntentRelease        LockIntent = "release"
	LockIntentRefund         LockIntent = "refund"
	LockIntentDispute        LockIntent = "dispute"
	LockIntentExpire         LockIntent = "expire"
)

// PolicySnapshot 是 Claim 时刻冻结的 policy 副本。它随锁一起持久化为 JSONB，
// 之后所有分配计算只读它，绝不回头读 reward_policies 的当前值。
type PolicySnapshot struct {
	PolicyID                string            `json:"policy_id"`
	PolicyHash              string            `json:"policy_hash"`
	TaskID                  string            `json:"task_id"`
	Currency                Currency          `json:"currency"`
	SettlementProvider      string            `json:"settlement_provider"`
	GrossAmountMinor        int64             `json:"gross_amount_minor"`
	CriterionWeightsBps     []CriterionWeight `json:"criterion_weights_bps"`
	MaintainerShareBps      int               `json:"maintainer_share_bps"`
	ReviewerPoolShareBps    int               `json:"reviewer_pool_share_bps"`
	PlatformFeeBps          int               `json:"platform_fee_bps"`
	DisputeReserveBps       int               `json:"dispute_reserve_bps"`
	QualityMultiplierMinBps int               `json:"quality_multiplier_min_bps"`
	QualityMultiplierMaxBps int               `json:"quality_multiplier_max_bps"`
	ChallengePeriodSeconds  int64             `json:"challenge_period_seconds"`
	ExpiresAt               time.Time         `json:"expires_at"`
}

// Lock 是一次 Claim 对资金的占用。
type Lock struct {
	ID                string
	ResourceTenantID  string
	PolicyID          string
	TaskID            string
	ExecutionID       string
	AgentID           string
	AgentVersionID    string
	PolicyHash        string
	PolicySnapshot    PolicySnapshot
	Currency          Currency
	LockedAmountMinor int64
	ChallengePeriod   time.Duration
	Status            LockStatus
	LockedAt          time.Time
	ExpiresAt         time.Time
	ReleasableAt      *time.Time
	ReleasedAt        *time.Time
	RefundedAt        *time.Time
	DisputedAt        *time.Time
	ExpiredAt         *time.Time
}

type NewLockParams struct {
	ID                string
	ResourceTenantID  string
	PolicyID          string
	TaskID            string
	ExecutionID       string
	AgentID           string
	AgentVersionID    string
	Snapshot          PolicySnapshot
	LockedAmountMinor int64
	LockedAt          time.Time
	ExpiresAt         time.Time
}

func NewLock(params NewLockParams) (*Lock, error) {
	for _, field := range []struct{ name, value string }{
		{"id", params.ID},
		{"resource_tenant_id", params.ResourceTenantID},
		{"policy_id", params.PolicyID},
		{"task_id", params.TaskID},
		{"execution_id", params.ExecutionID},
		{"agent_id", params.AgentID},
		{"agent_version_id", params.AgentVersionID},
	} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	if params.Snapshot.PolicyID != params.PolicyID {
		return nil, invalid("policy_snapshot")
	}
	if len(params.Snapshot.PolicyHash) != 64 {
		return nil, invalid("policy_hash")
	}
	if !ValidCurrency(params.Snapshot.Currency) {
		return nil, invalid("currency")
	}
	if params.LockedAmountMinor <= 0 || params.LockedAmountMinor > MaxAmountMinor ||
		params.LockedAmountMinor > params.Snapshot.GrossAmountMinor {
		return nil, invalid("locked_amount_minor")
	}
	if params.LockedAt.IsZero() {
		return nil, invalid("locked_at")
	}
	if params.ExpiresAt.IsZero() || !params.ExpiresAt.After(params.LockedAt) {
		return nil, invalid("expires_at")
	}
	weights := make([]CriterionWeight, len(params.Snapshot.CriterionWeightsBps))
	copy(weights, params.Snapshot.CriterionWeightsBps)
	snapshot := params.Snapshot
	snapshot.CriterionWeightsBps = weights
	return &Lock{
		ID: params.ID, ResourceTenantID: params.ResourceTenantID,
		PolicyID: params.PolicyID, TaskID: params.TaskID,
		ExecutionID: params.ExecutionID, AgentID: params.AgentID,
		AgentVersionID: params.AgentVersionID, PolicyHash: snapshot.PolicyHash,
		PolicySnapshot: snapshot, Currency: snapshot.Currency,
		LockedAmountMinor: params.LockedAmountMinor,
		ChallengePeriod:   time.Duration(snapshot.ChallengePeriodSeconds) * time.Second,
		Status:            LockLocked, LockedAt: params.LockedAt.UTC(),
		ExpiresAt: params.ExpiresAt.UTC(),
	}, nil
}

// IsTerminal 判断锁是否已到终态。终态上的任何意图都是冲突而非 no-op：
// 幂等由应用层比对"目标状态是否已达成"来实现。
func (l Lock) IsTerminal() bool {
	return l.Status == LockReleased || l.Status == LockRefunded || l.Status == LockExpired
}

// Apply 执行锁的状态迁移。
//
// locked → releasable → released 是正常路径；争议、退款与过期是三条旁路。
// 争议只能在资金尚未离开托管时发起，因此终态上不再允许 dispute。
func (l *Lock) Apply(intent LockIntent, now time.Time) error {
	if now.IsZero() {
		return invalid("now")
	}
	moment := now.UTC()
	switch intent {
	case LockIntentMarkReleasable:
		if l.Status != LockLocked {
			return ErrStateConflict
		}
		l.Status = LockReleasable
		l.ReleasableAt = &moment
	case LockIntentRelease:
		// 只有走过决策（releasable）或经争议裁决的锁才能释放。
		if l.Status != LockReleasable && l.Status != LockDisputed {
			return ErrStateConflict
		}
		if l.ReleasableAt == nil {
			l.ReleasableAt = &moment
		}
		l.Status = LockReleased
		l.ReleasedAt = &moment
	case LockIntentRefund:
		if l.Status != LockLocked && l.Status != LockReleasable && l.Status != LockDisputed {
			return ErrStateConflict
		}
		l.Status = LockRefunded
		l.RefundedAt = &moment
	case LockIntentDispute:
		if l.Status != LockLocked && l.Status != LockReleasable {
			return ErrStateConflict
		}
		l.Status = LockDisputed
		l.DisputedAt = &moment
	case LockIntentExpire:
		// 争议中的锁不会因为挑战期外的过期而被自动作废：裁决优先。
		if l.Status != LockLocked && l.Status != LockReleasable {
			return ErrStateConflict
		}
		l.Status = LockExpired
		l.ExpiredAt = &moment
	default:
		return invalid("intent")
	}
	return nil
}

// ChallengeDeadline 返回本次锁定的挑战期截止时刻，基线是决策时刻。
func (l Lock) ChallengeDeadline(decidedAt time.Time) time.Time {
	return decidedAt.UTC().Add(l.ChallengePeriod)
}

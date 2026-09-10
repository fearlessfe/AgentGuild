package domain

import (
	"strings"
	"time"
)

// DisputeStatus 是争议的状态。
type DisputeStatus string

const (
	DisputeOpen     DisputeStatus = "open"
	DisputeResolved DisputeStatus = "resolved"
)

// DisputeResolution 是裁决结果。split 表示部分释放、剩余退款。
type DisputeResolution string

const (
	ResolutionRelease DisputeResolution = "release"
	ResolutionRefund  DisputeResolution = "refund"
	ResolutionSplit   DisputeResolution = "split"
)

// Dispute 是对某条决策的争议。一个锁最多一条争议：重复发起是 no-op，
// 不会派生出第二条裁决路径（doc §10 的幂等要求）。
type Dispute struct {
	ID                       string
	ResourceTenantID         string
	LockID                   string
	DecisionHash             string
	Status                   DisputeStatus
	Reason                   string
	OpenedBy                 string
	OpenedAt                 time.Time
	Resolution               *DisputeResolution
	ResolvedBy               *string
	ResolvedAt               *time.Time
	ResolvedAgentAmountMinor *int64
}

type NewDisputeParams struct {
	ID               string
	ResourceTenantID string
	LockID           string
	DecisionHash     string
	Reason           string
	OpenedBy         string
	OpenedAt         time.Time
}

func NewDispute(params NewDisputeParams) (*Dispute, error) {
	for _, field := range []struct{ name, value string }{
		{"id", params.ID},
		{"resource_tenant_id", params.ResourceTenantID},
		{"lock_id", params.LockID},
		{"opened_by", params.OpenedBy},
	} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	if !isHex64(params.DecisionHash) {
		return nil, invalid("decision_hash")
	}
	if params.OpenedAt.IsZero() {
		return nil, invalid("opened_at")
	}
	return &Dispute{
		ID: params.ID, ResourceTenantID: params.ResourceTenantID,
		LockID: params.LockID, DecisionHash: params.DecisionHash,
		Status: DisputeOpen, Reason: params.Reason,
		OpenedBy: params.OpenedBy, OpenedAt: params.OpenedAt.UTC(),
	}, nil
}

// Resolve 裁决争议。裁决出的 Agent 金额不得超过原决策的 net：
// 争议只能重新分配已托管的资金，不能凭空增发。
func (d *Dispute) Resolve(resolution DisputeResolution, resolvedBy string, agentAmountMinor, netAmountMinor int64, now time.Time) error {
	if d.Status != DisputeOpen {
		return ErrStateConflict
	}
	if resolvedBy == "" || strings.TrimSpace(resolvedBy) != resolvedBy {
		return invalid("resolved_by")
	}
	if now.IsZero() {
		return invalid("now")
	}
	if agentAmountMinor < 0 || agentAmountMinor > netAmountMinor {
		return invalid("resolved_agent_amount_minor")
	}
	switch resolution {
	case ResolutionRelease:
		if agentAmountMinor == 0 {
			return invalid("resolved_agent_amount_minor")
		}
	case ResolutionRefund:
		if agentAmountMinor != 0 {
			return invalid("resolved_agent_amount_minor")
		}
	case ResolutionSplit:
		if agentAmountMinor <= 0 || agentAmountMinor >= netAmountMinor {
			return invalid("resolved_agent_amount_minor")
		}
	default:
		return invalid("resolution")
	}
	moment := now.UTC()
	d.Status = DisputeResolved
	d.Resolution = &resolution
	d.ResolvedBy = &resolvedBy
	d.ResolvedAt = &moment
	d.ResolvedAgentAmountMinor = &agentAmountMinor
	return nil
}

// LockIntentFor 把裁决结果映射到锁的迁移意图。
func (d Dispute) LockIntentFor() (LockIntent, error) {
	if d.Resolution == nil {
		return "", ErrStateConflict
	}
	switch *d.Resolution {
	case ResolutionRefund:
		return LockIntentRefund, nil
	case ResolutionRelease, ResolutionSplit:
		return LockIntentRelease, nil
	default:
		return "", invalid("resolution")
	}
}

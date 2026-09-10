package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	canonicaljson "github.com/gibson042/canonicaljson-go"
)

// PolicyStatus 是任务级出资的状态。
//
// 它与 RewardLock 的状态机刻意分开：doc §3.2 把任务级出资和执行级锁定混成
// 一条链，无法表达"一个任务被多次 claim / 退款"。
type PolicyStatus string

const (
	PolicyUnfunded  PolicyStatus = "unfunded"
	PolicyFunded    PolicyStatus = "funded"
	PolicyExhausted PolicyStatus = "exhausted"
	PolicyCancelled PolicyStatus = "cancelled"
)

// PolicyIntent 是 policy 上允许的状态迁移意图。
type PolicyIntent string

const (
	PolicyIntentFund    PolicyIntent = "fund"
	PolicyIntentExhaust PolicyIntent = "exhaust"
	PolicyIntentCancel  PolicyIntent = "cancel"
)

// CriterionWeight 是单条验收标准在 Agent 份额中的权重（bps）。
type CriterionWeight struct {
	CriterionID string `json:"criterion_id"`
	WeightBps   int    `json:"weight_bps"`
}

// Policy 是任务级的不可变奖励契约。字段一旦创建就不再变化，唯一会动的是
// 状态与出资额；需要改金额或权重只能创建一条新的 policy（doc §10）。
type Policy struct {
	ID                      string
	ResourceTenantID        string
	TaskID                  string
	PolicyHash              string
	Currency                Currency
	SettlementProvider      string
	GrossAmountMinor        int64
	FundedAmountMinor       int64
	CriterionWeights        []CriterionWeight
	MaintainerShareBps      int
	ReviewerPoolShareBps    int
	PlatformFeeBps          int
	DisputeReserveBps       int
	QualityMultiplierMinBps int
	QualityMultiplierMaxBps int
	ChallengePeriod         time.Duration
	ExpiresAt               time.Time
	Status                  PolicyStatus
	CreatedAt               time.Time
	FundedAt                *time.Time
	ExhaustedAt             *time.Time
	CancelledAt             *time.Time
}

type NewPolicyParams struct {
	ID                      string
	ResourceTenantID        string
	TaskID                  string
	Currency                Currency
	SettlementProvider      string
	GrossAmountMinor        int64
	CriterionWeights        []CriterionWeight
	MaintainerShareBps      int
	ReviewerPoolShareBps    int
	PlatformFeeBps          int
	DisputeReserveBps       int
	QualityMultiplierMinBps int
	QualityMultiplierMaxBps int
	ChallengePeriod         time.Duration
	ExpiresAt               time.Time
	CreatedAt               time.Time
}

// policyDocument 是参与 policy_hash 计算的字段白名单。
//
// 刻意不含 resource_tenant_id：公开面绝不泄露 sponsor 租户，而摘要要能被
// 第三方用公开字段独立复算。字段顺序无关，canonical JSON 按键排序。
type policyDocument struct {
	PolicyID                string            `json:"policy_id"`
	TaskID                  string            `json:"task_id"`
	Currency                string            `json:"currency"`
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
	ExpiresAt               string            `json:"expires_at"`
}

// NewPolicy 校验并构造一条 unfunded 的 policy，并**自行计算** policy_hash。
// 调用方传入的任何摘要都会被忽略——摘要必须由字段唯一决定。
func NewPolicy(params NewPolicyParams) (*Policy, error) {
	for _, field := range []struct{ name, value string }{
		{"id", params.ID},
		{"resource_tenant_id", params.ResourceTenantID},
		{"task_id", params.TaskID},
		{"settlement_provider", params.SettlementProvider},
	} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	if !ValidCurrency(params.Currency) {
		return nil, invalid("currency")
	}
	if params.GrossAmountMinor <= 0 || params.GrossAmountMinor > MaxAmountMinor {
		return nil, invalid("gross_amount_minor")
	}
	for _, bps := range map[string]int{
		"maintainer_share_bps":    params.MaintainerShareBps,
		"reviewer_pool_share_bps": params.ReviewerPoolShareBps,
		"platform_fee_bps":        params.PlatformFeeBps,
		"dispute_reserve_bps":     params.DisputeReserveBps,
	} {
		if !ValidBps(bps) {
			return nil, invalid("share_bps")
		}
	}
	if params.PlatformFeeBps+params.DisputeReserveBps > BasisPointsScale {
		return nil, invalid("platform_fee_bps")
	}
	if params.MaintainerShareBps+params.ReviewerPoolShareBps > BasisPointsScale {
		return nil, invalid("maintainer_share_bps")
	}
	// quality_multiplier 的区间在这里冻结，结果出来后不得调整（doc §5.1）。
	if params.QualityMultiplierMinBps < 0 || params.QualityMultiplierMaxBps > 2*BasisPointsScale ||
		params.QualityMultiplierMinBps > params.QualityMultiplierMaxBps {
		return nil, invalid("quality_multiplier_bps")
	}
	if params.ChallengePeriod < 0 {
		return nil, invalid("challenge_period")
	}
	if params.CreatedAt.IsZero() {
		return nil, invalid("created_at")
	}
	if params.ExpiresAt.IsZero() || !params.ExpiresAt.After(params.CreatedAt) {
		return nil, invalid("expires_at")
	}
	weights, err := normalizeWeights(params.CriterionWeights)
	if err != nil {
		return nil, err
	}

	policy := &Policy{
		ID: params.ID, ResourceTenantID: params.ResourceTenantID, TaskID: params.TaskID,
		Currency: params.Currency, SettlementProvider: params.SettlementProvider,
		GrossAmountMinor: params.GrossAmountMinor, CriterionWeights: weights,
		MaintainerShareBps: params.MaintainerShareBps, ReviewerPoolShareBps: params.ReviewerPoolShareBps,
		PlatformFeeBps: params.PlatformFeeBps, DisputeReserveBps: params.DisputeReserveBps,
		QualityMultiplierMinBps: params.QualityMultiplierMinBps,
		QualityMultiplierMaxBps: params.QualityMultiplierMaxBps,
		ChallengePeriod:         params.ChallengePeriod, ExpiresAt: params.ExpiresAt.UTC(),
		Status: PolicyUnfunded, CreatedAt: params.CreatedAt.UTC(),
	}
	hash, err := policy.ComputePolicyHash()
	if err != nil {
		return nil, err
	}
	policy.PolicyHash = hash
	return policy, nil
}

// normalizeWeights 校验并按 criterion_id 排序权重，让同一组权重在任何调用
// 顺序下都产生同一个摘要。
func normalizeWeights(weights []CriterionWeight) ([]CriterionWeight, error) {
	result := make([]CriterionWeight, 0, len(weights))
	seen := make(map[string]struct{}, len(weights))
	total := 0
	for _, weight := range weights {
		if weight.CriterionID == "" || strings.TrimSpace(weight.CriterionID) != weight.CriterionID {
			return nil, invalid("criterion_id")
		}
		if _, duplicate := seen[weight.CriterionID]; duplicate {
			return nil, invalid("criterion_id")
		}
		if !ValidBps(weight.WeightBps) {
			return nil, invalid("weight_bps")
		}
		seen[weight.CriterionID] = struct{}{}
		total += weight.WeightBps
		result = append(result, weight)
	}
	if total > BasisPointsScale {
		return nil, invalid("criterion_weights_bps")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CriterionID < result[j].CriterionID })
	return result, nil
}

// ComputePolicyHash 返回 policy 的规范化 sha256 摘要（小写 hex）。
func (p Policy) ComputePolicyHash() (string, error) {
	weights := p.CriterionWeights
	if weights == nil {
		weights = []CriterionWeight{}
	}
	document := policyDocument{
		PolicyID: p.ID, TaskID: p.TaskID, Currency: string(p.Currency),
		SettlementProvider: p.SettlementProvider, GrossAmountMinor: p.GrossAmountMinor,
		CriterionWeightsBps: weights, MaintainerShareBps: p.MaintainerShareBps,
		ReviewerPoolShareBps: p.ReviewerPoolShareBps, PlatformFeeBps: p.PlatformFeeBps,
		DisputeReserveBps:       p.DisputeReserveBps,
		QualityMultiplierMinBps: p.QualityMultiplierMinBps,
		QualityMultiplierMaxBps: p.QualityMultiplierMaxBps,
		ChallengePeriodSeconds:  int64(p.ChallengePeriod / time.Second),
		ExpiresAt:               p.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Apply 执行 policy 状态迁移。状态机留在领域层，应用层不得绕过。
func (p *Policy) Apply(intent PolicyIntent, fundedAmountMinor int64, now time.Time) error {
	if now.IsZero() {
		return invalid("now")
	}
	moment := now.UTC()
	switch intent {
	case PolicyIntentFund:
		if p.Status != PolicyUnfunded {
			return ErrStateConflict
		}
		if fundedAmountMinor <= 0 || fundedAmountMinor > p.GrossAmountMinor {
			return invalid("funded_amount_minor")
		}
		p.Status = PolicyFunded
		p.FundedAmountMinor = fundedAmountMinor
		p.FundedAt = &moment
		return nil
	case PolicyIntentExhaust:
		if p.Status != PolicyFunded {
			return ErrStateConflict
		}
		p.Status = PolicyExhausted
		p.ExhaustedAt = &moment
		return nil
	case PolicyIntentCancel:
		// 已 funded 的 policy 也可以取消，但只有在没有活跃锁的前提下——
		// 那个前提由存储层在同一事务里检查，领域层只表达状态可达性。
		if p.Status != PolicyUnfunded && p.Status != PolicyFunded {
			return ErrStateConflict
		}
		p.Status = PolicyCancelled
		p.CancelledAt = &moment
		return nil
	default:
		return invalid("intent")
	}
}

// Snapshot 把 Claim 时刻的 policy 冻结成一份快照。锁定之后 Issue 如何更新
// 都不会影响本次执行的金额、权重与挑战期（doc §10）。
func (p Policy) Snapshot() PolicySnapshot {
	weights := make([]CriterionWeight, len(p.CriterionWeights))
	copy(weights, p.CriterionWeights)
	return PolicySnapshot{
		PolicyID: p.ID, PolicyHash: p.PolicyHash, TaskID: p.TaskID,
		Currency: p.Currency, SettlementProvider: p.SettlementProvider,
		GrossAmountMinor: p.GrossAmountMinor, CriterionWeightsBps: weights,
		MaintainerShareBps: p.MaintainerShareBps, ReviewerPoolShareBps: p.ReviewerPoolShareBps,
		PlatformFeeBps: p.PlatformFeeBps, DisputeReserveBps: p.DisputeReserveBps,
		QualityMultiplierMinBps: p.QualityMultiplierMinBps,
		QualityMultiplierMaxBps: p.QualityMultiplierMaxBps,
		ChallengePeriodSeconds:  int64(p.ChallengePeriod / time.Second),
		ExpiresAt:               p.ExpiresAt.UTC(),
	}
}

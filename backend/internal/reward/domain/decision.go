package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	canonicaljson "github.com/gibson042/canonicaljson-go"
)

// CriterionOutcome 是决策里逐条验收标准的判定结果。
//
// Verified 与 Passed 分开：规格里存在但没有任何结果的 criterion 是
// Verified=false，任何判定中都不得当成通过——这是整条链路的核心不变量。
type CriterionOutcome struct {
	CriterionID string `json:"criterion_id"`
	Required    bool   `json:"required"`
	WeightBps   int    `json:"weight_bps"`
	Verified    bool   `json:"verified"`
	Passed      bool   `json:"passed"`
}

// Allocation 是一次分配的完整结果，各项之和必然闭合到 gross。
type Allocation struct {
	GrossAmountMinor        int64
	PlatformFeeMinor        int64
	DisputeReserveMinor     int64
	NetAmountMinor          int64
	AgentAmountMinor        int64
	MaintainerAmountMinor   int64
	ReviewerPoolAmountMinor int64
	// UnallocatedAmountMinor 收纳所有向下取整的余数与未达成的 Agent 份额，
	// 让整数分账必然闭合。它随退款回到 sponsor。
	UnallocatedAmountMinor int64
}

type AllocateParams struct {
	Snapshot PolicySnapshot
	// GrossAmountMinor 是本次锁定实际占用的金额，通常等于快照里的 gross。
	GrossAmountMinor int64
	Outcomes         []CriterionOutcome
	// AllRequiredPassed 来自 contribution 领域的 CriterionCoverage，
	// 是 Agent 份额的硬门禁：为 false 时份额必须为 0（doc §10）。
	AllRequiredPassed bool
	// QualityMultiplierBps 必须落在快照冻结的区间内，超出直接报错，
	// 不做截断——静默截断等于事后调整了乘数。
	QualityMultiplierBps int
}

// Allocate 按 doc §5.1 的模型分配奖励：
//
//	net   = gross − platform_fee − dispute_reserve
//	agent = net_after_shares × Σ(criterion_weight × verified) × quality_multiplier
//
// 任一 required criterion 未通过或未验证 ⇒ agent 份额为 0。
func Allocate(params AllocateParams) (Allocation, error) {
	snapshot := params.Snapshot
	gross := params.GrossAmountMinor
	if gross <= 0 || gross > MaxAmountMinor || gross > snapshot.GrossAmountMinor {
		return Allocation{}, invalid("gross_amount_minor")
	}
	if !ValidBps(snapshot.PlatformFeeBps) || !ValidBps(snapshot.DisputeReserveBps) ||
		snapshot.PlatformFeeBps+snapshot.DisputeReserveBps > BasisPointsScale {
		return Allocation{}, invalid("platform_fee_bps")
	}
	if !ValidBps(snapshot.MaintainerShareBps) || !ValidBps(snapshot.ReviewerPoolShareBps) ||
		snapshot.MaintainerShareBps+snapshot.ReviewerPoolShareBps > BasisPointsScale {
		return Allocation{}, invalid("maintainer_share_bps")
	}
	if params.QualityMultiplierBps < snapshot.QualityMultiplierMinBps ||
		params.QualityMultiplierBps > snapshot.QualityMultiplierMaxBps {
		return Allocation{}, invalid("quality_multiplier_bps")
	}

	platformFee := ApplyBps(gross, snapshot.PlatformFeeBps)
	disputeReserve := ApplyBps(gross, snapshot.DisputeReserveBps)
	net := gross - platformFee - disputeReserve
	maintainer := ApplyBps(net, snapshot.MaintainerShareBps)
	reviewerPool := ApplyBps(net, snapshot.ReviewerPoolShareBps)
	agentPool := net - maintainer - reviewerPool

	agent := int64(0)
	if params.AllRequiredPassed {
		verifiedWeight, err := verifiedWeightBps(params.Outcomes)
		if err != nil {
			return Allocation{}, err
		}
		agent = ApplyBps(ApplyBps(agentPool, verifiedWeight), params.QualityMultiplierBps)
		// 乘数上限可能 > 10000，因此显式钳到可分配上限，绝不超发。
		if agent > agentPool {
			agent = agentPool
		}
	}

	allocation := Allocation{
		GrossAmountMinor: gross, PlatformFeeMinor: platformFee,
		DisputeReserveMinor: disputeReserve, NetAmountMinor: net,
		AgentAmountMinor: agent, MaintainerAmountMinor: maintainer,
		ReviewerPoolAmountMinor: reviewerPool,
		UnallocatedAmountMinor:  net - agent - maintainer - reviewerPool,
	}
	if allocation.UnallocatedAmountMinor < 0 {
		return Allocation{}, ErrStateConflict
	}
	return allocation, nil
}

// verifiedWeightBps 汇总已验证且通过的 criterion 权重。未验证的 criterion
// 不贡献任何权重。
func verifiedWeightBps(outcomes []CriterionOutcome) (int, error) {
	total := 0
	for _, outcome := range outcomes {
		if !ValidBps(outcome.WeightBps) {
			return 0, invalid("weight_bps")
		}
		if outcome.Verified && outcome.Passed {
			total += outcome.WeightBps
		}
	}
	if total > BasisPointsScale {
		return 0, invalid("criterion_weights_bps")
	}
	return total, nil
}

// Decision 是一次可被第三方验证的奖励决策。
//
// 它只携带公开可验证的摘要与金额：task_spec_hash、contribution_hash、
// recipient_ref。私有 Issue、代码与评审内容不出现在任何字段里（doc §10）。
type Decision struct {
	DecisionHash           string
	ResourceTenantID       string
	LockID                 string
	PolicyHash             string
	TaskSpecHash           string
	ContributionHash       string
	AlgorithmVersion       string
	Currency               Currency
	Allocation             Allocation
	QualityMultiplierBps   int
	CriterionResults       []CriterionOutcome
	RequiredCriteriaPassed bool
	RecipientRef           string
	ChallengeDeadline      time.Time
	Signature              string
	SignatureAlgorithm     string
	DecidedAt              time.Time
}

type NewDecisionParams struct {
	ResourceTenantID       string
	LockID                 string
	PolicyHash             string
	TaskSpecHash           string
	ContributionHash       string
	AlgorithmVersion       string
	Currency               Currency
	Allocation             Allocation
	QualityMultiplierBps   int
	CriterionResults       []CriterionOutcome
	RequiredCriteriaPassed bool
	RecipientRef           string
	ChallengeDeadline      time.Time
	DecidedAt              time.Time
	Signer                 Signer
}

// decisionDocument 是参与 decision_hash 计算的字段白名单。
//
// 同样不含 resource_tenant_id：匿名第三方要能用公开字段独立复算摘要，
// 而 sponsor 租户必须保持不可见。
type decisionDocument struct {
	LockID                 string             `json:"lock_id"`
	PolicyHash             string             `json:"policy_hash"`
	TaskSpecHash           string             `json:"task_spec_hash"`
	ContributionHash       string             `json:"contribution_hash"`
	AlgorithmVersion       string             `json:"algorithm_version"`
	Currency               string             `json:"currency"`
	GrossAmountMinor       int64              `json:"gross_amount_minor"`
	PlatformFeeMinor       int64              `json:"platform_fee_minor"`
	DisputeReserveMinor    int64              `json:"dispute_reserve_minor"`
	NetAmountMinor         int64              `json:"net_amount_minor"`
	AgentAmountMinor       int64              `json:"agent_amount_minor"`
	MaintainerAmountMinor  int64              `json:"maintainer_amount_minor"`
	ReviewerPoolMinor      int64              `json:"reviewer_pool_amount_minor"`
	UnallocatedAmountMinor int64              `json:"unallocated_amount_minor"`
	QualityMultiplierBps   int                `json:"quality_multiplier_bps"`
	CriterionResults       []CriterionOutcome `json:"criterion_results"`
	RequiredCriteriaPassed bool               `json:"required_criteria_passed"`
	RecipientRef           string             `json:"recipient_ref"`
	ChallengeDeadline      string             `json:"challenge_deadline"`
	DecidedAt              string             `json:"decided_at"`
}

// Signer 对 decision 摘要签名。第一版用对称密钥（HMAC），
// 后续换成 JWS/COSE 或 EAS 时只需替换实现。
type Signer interface {
	Algorithm() string
	Sign(decisionHash string) (string, error)
}

// NewDecision 校验并构造决策，**自行计算** decision_hash 与签名。
func NewDecision(params NewDecisionParams) (*Decision, error) {
	for _, field := range []struct{ name, value string }{
		{"resource_tenant_id", params.ResourceTenantID},
		{"lock_id", params.LockID},
		{"algorithm_version", params.AlgorithmVersion},
	} {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalid(field.name)
		}
	}
	for _, field := range []struct{ name, value string }{
		{"policy_hash", params.PolicyHash},
		{"task_spec_hash", params.TaskSpecHash},
		{"contribution_hash", params.ContributionHash},
		{"recipient_ref", params.RecipientRef},
	} {
		if !isHex64(field.value) {
			return nil, invalid(field.name)
		}
	}
	if !ValidCurrency(params.Currency) {
		return nil, invalid("currency")
	}
	if params.DecidedAt.IsZero() || params.ChallengeDeadline.IsZero() ||
		params.ChallengeDeadline.Before(params.DecidedAt) {
		return nil, invalid("challenge_deadline")
	}
	if params.Signer == nil {
		return nil, invalid("signer")
	}
	allocation := params.Allocation
	if allocation.GrossAmountMinor <= 0 ||
		allocation.GrossAmountMinor != allocation.PlatformFeeMinor+allocation.DisputeReserveMinor+allocation.NetAmountMinor ||
		allocation.NetAmountMinor != allocation.AgentAmountMinor+allocation.MaintainerAmountMinor+
			allocation.ReviewerPoolAmountMinor+allocation.UnallocatedAmountMinor {
		return nil, invalid("allocation")
	}
	// doc §10 的硬门禁在领域层再挡一次，数据库 CHECK 是最后一道。
	if !params.RequiredCriteriaPassed && allocation.AgentAmountMinor != 0 {
		return nil, ErrStateConflict
	}

	outcomes := make([]CriterionOutcome, len(params.CriterionResults))
	copy(outcomes, params.CriterionResults)
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].CriterionID < outcomes[j].CriterionID })

	decision := &Decision{
		ResourceTenantID: params.ResourceTenantID, LockID: params.LockID,
		PolicyHash: params.PolicyHash, TaskSpecHash: params.TaskSpecHash,
		ContributionHash: params.ContributionHash, AlgorithmVersion: params.AlgorithmVersion,
		Currency: params.Currency, Allocation: allocation,
		QualityMultiplierBps: params.QualityMultiplierBps, CriterionResults: outcomes,
		RequiredCriteriaPassed: params.RequiredCriteriaPassed,
		RecipientRef:           params.RecipientRef,
		ChallengeDeadline:      params.ChallengeDeadline.UTC(),
		DecidedAt:              params.DecidedAt.UTC(),
	}
	hash, err := decision.ComputeDecisionHash()
	if err != nil {
		return nil, err
	}
	signature, err := params.Signer.Sign(hash)
	if err != nil {
		return nil, err
	}
	decision.DecisionHash = hash
	decision.Signature = signature
	decision.SignatureAlgorithm = params.Signer.Algorithm()
	return decision, nil
}

// ComputeDecisionHash 返回决策的规范化 sha256 摘要（小写 hex）。
func (d Decision) ComputeDecisionHash() (string, error) {
	outcomes := d.CriterionResults
	if outcomes == nil {
		outcomes = []CriterionOutcome{}
	}
	document := decisionDocument{
		LockID: d.LockID, PolicyHash: d.PolicyHash, TaskSpecHash: d.TaskSpecHash,
		ContributionHash: d.ContributionHash, AlgorithmVersion: d.AlgorithmVersion,
		Currency:               string(d.Currency),
		GrossAmountMinor:       d.Allocation.GrossAmountMinor,
		PlatformFeeMinor:       d.Allocation.PlatformFeeMinor,
		DisputeReserveMinor:    d.Allocation.DisputeReserveMinor,
		NetAmountMinor:         d.Allocation.NetAmountMinor,
		AgentAmountMinor:       d.Allocation.AgentAmountMinor,
		MaintainerAmountMinor:  d.Allocation.MaintainerAmountMinor,
		ReviewerPoolMinor:      d.Allocation.ReviewerPoolAmountMinor,
		UnallocatedAmountMinor: d.Allocation.UnallocatedAmountMinor,
		QualityMultiplierBps:   d.QualityMultiplierBps,
		CriterionResults:       outcomes,
		RequiredCriteriaPassed: d.RequiredCriteriaPassed,
		RecipientRef:           d.RecipientRef,
		ChallengeDeadline:      d.ChallengeDeadline.UTC().Format(time.RFC3339Nano),
		DecidedAt:              d.DecidedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := canonicaljson.Marshal(document)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// signingKeyLabel 用于从部署密钥派生出奖励决定专用的签名密钥。
//
// 调用方传入的往往是一个身兼多职的部署级密钥（当前是 CURSOR_SECRET，它同时
// 为分页游标签名）。直接拿它做 HMAC 等于跨用途复用密钥：游标是发给每一个
// 匿名调用方的低价值凭据，而奖励决定签名是资金完整性控制，两者一旦共用密钥，
// 前者泄露即等同于后者被攻破，并可能构成跨协议伪造。派生一次即可切断这层耦合。
const signingKeyLabel = "agentguild/reward-decision/v1"

// HMACSigner 是第一版签名实现。它不需要任何外部依赖，验证方持有同一派生密钥
// 即可确认"这笔钱对应哪一个任务和哪一组证据"。
type HMACSigner struct {
	secret []byte
}

// NewHMACSigner 从部署密钥派生签名密钥，而不是直接使用它。
func NewHMACSigner(secret []byte) (*HMACSigner, error) {
	if len(secret) < 32 {
		return nil, invalid("signing_secret")
	}
	derive := hmac.New(sha256.New, secret)
	if _, err := derive.Write([]byte(signingKeyLabel)); err != nil {
		return nil, err
	}
	return &HMACSigner{secret: derive.Sum(nil)}, nil
}

func (s *HMACSigner) Algorithm() string { return "hmac-sha256" }

func (s *HMACSigner) Sign(decisionHash string) (string, error) {
	if !isHex64(decisionHash) {
		return "", invalid("decision_hash")
	}
	mac := hmac.New(sha256.New, s.secret)
	if _, err := mac.Write([]byte(decisionHash)); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Verify 供验证方复算签名。
func (s *HMACSigner) Verify(decisionHash, signature string) bool {
	expected, err := s.Sign(decisionHash)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(expected), []byte(signature))
}

var _ Signer = (*HMACSigner)(nil)

func isHex64(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

// DefaultAlgorithmVersion 是奖励分配算法的首个版本号。它被冻结在每条
// decision 上，让历史决策永远能用当时的算法复算（doc §4.5 的同一原则）。
const DefaultAlgorithmVersion = "2026-09-09-reward-v1"

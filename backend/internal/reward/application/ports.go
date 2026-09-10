package application

import (
	"context"
	"time"

	contributiondomain "agentguild.dev/agentguild/backend/internal/contribution/domain"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
)

// Store 是奖励模块唯一的持久化入口。所有命令都在一个事务里完成：
// 资金账本与状态机必须原子推进，中间态一旦落库就无法对账。
type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

type Tx interface {
	Escrow() EscrowRepository
	Policies() PolicyRepository
	Locks() LockRepository
	Decisions() DecisionRepository
	Disputes() DisputeRepository
	Receipts() ReceiptRepository
	Destinations() DestinationRepository
	Now(context.Context) (time.Time, error)
}

type EscrowRepository interface {
	// EnsureAccount 幂等地创建账户行，让首次充值不必区分"新建/更新"。
	EnsureAccount(context.Context, string, domain.Currency) error
	Get(context.Context, string, domain.Currency) (*domain.EscrowAccount, error)
	// ApplyEntry 以 idempotency_key 幂等地记账。返回 false 表示该分录已存在，
	// 余额未被二次改变——这是重复回调、重复支付、重复退款的共同兜底。
	ApplyEntry(context.Context, domain.EscrowEntry) (bool, error)
}

type PolicyRepository interface {
	Insert(context.Context, *domain.Policy) error
	GetByID(context.Context, string, string) (*domain.Policy, error)
	// GetActiveByTask 返回任务当前可领取的 policy（unfunded 或 funded）。
	GetActiveByTask(context.Context, string, string) (*domain.Policy, error)
	// GetFundedByTaskForUpdate 在 Claim 事务里取行锁，避免并发 claim 超额锁定。
	GetFundedByTaskForUpdate(context.Context, string, string) (*domain.Policy, error)
	Save(context.Context, *domain.Policy) error
	// LockedAmount 汇总该 policy 上尚未释放/退款的锁定额。
	LockedAmount(context.Context, string, string) (int64, error)
}

type LockRepository interface {
	Insert(context.Context, *domain.Lock) error
	GetByID(context.Context, string, string) (*domain.Lock, error)
	GetByExecution(context.Context, string, string) (*domain.Lock, error)
	Save(context.Context, *domain.Lock) error
	// ListByStatus 供 worker 扫描待处理的锁。
	ListByStatus(context.Context, domain.LockStatus, int) ([]domain.Lock, error)
	// ListExpired 返回超过 expires_at 且仍占用资金的锁。
	ListExpired(context.Context, time.Time, int) ([]domain.Lock, error)
}

type DecisionRepository interface {
	Insert(context.Context, *domain.Decision) error
	GetByLock(context.Context, string, string) (*domain.Decision, error)
	GetByHash(context.Context, string) (*domain.Decision, error)
	// ListReleasable 返回挑战期已过、锁仍为 releasable 的决策。
	ListReleasable(context.Context, time.Time, int) ([]domain.Decision, error)
}

type DisputeRepository interface {
	Insert(context.Context, *domain.Dispute) error
	GetByLock(context.Context, string, string) (*domain.Dispute, error)
	GetByID(context.Context, string, string) (*domain.Dispute, error)
	Save(context.Context, *domain.Dispute) error
	AppendEvent(context.Context, DisputeEvent) error
}

type DisputeEvent struct {
	DisputeID  string
	EventType  string
	ActorID    string
	Payload    map[string]string
	OccurredAt time.Time
}

type ReceiptRepository interface {
	// Append 幂等：同一 (provider, reference, state) 重复投递返回 false。
	Append(context.Context, *domain.Receipt) (bool, error)
	ListByDecision(context.Context, string) ([]domain.Receipt, error)
}

type DestinationRepository interface {
	InsertChallenge(context.Context, *domain.Challenge) error
	// ConsumeChallenge 一次性消费 nonce；已消费或过期返回 ErrStateConflict。
	ConsumeChallenge(context.Context, string, string, string, string, time.Time) error
	Insert(context.Context, *domain.Destination) error
	GetActive(context.Context, string) (*domain.Destination, error)
	ListByAgent(context.Context, string) ([]domain.Destination, error)
	Save(context.Context, *domain.Destination) error
}

// EvidenceSource 提供生成决策所需的公开证据摘要。
//
// 它刻意不返回 Issue 正文、diff 或评审内容：decision 只携带可验证的 hash，
// 不得泄露私有内容（doc §10）。
type EvidenceSource interface {
	DecisionEvidence(context.Context, string, string) (DecisionEvidence, error)
}

type DecisionEvidence struct {
	TaskSpecHash     string
	ContributionHash string
	// Spec 是任务规格里的 criterion 全集，Latest 是它们的最新验证结果。
	Spec   []contributiondomain.SpecCriterion
	Latest []contributiondomain.CriterionResult
}

// Coverage 把证据降解为奖励释放门禁使用的汇总。
func (e DecisionEvidence) Coverage() contributiondomain.CriterionCoverage {
	return contributiondomain.SummarizeCriteria(e.Spec, e.Latest)
}

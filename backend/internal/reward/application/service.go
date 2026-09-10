package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/reward/domain"
	"agentguild.dev/agentguild/backend/internal/settlement"
)

const (
	// defaultLockTTL 是锁在无人决策时自动过期的兜底时长。
	defaultLockTTL = 30 * 24 * time.Hour
	// defaultChallengeTTL 是收款目的地 nonce 的有效期。
	defaultChallengeTTL = 10 * time.Minute
)

type Options struct {
	Store    Store
	Provider settlement.Provider
	Evidence EvidenceSource
	Signer   domain.Signer
	// AlgorithmVersion 冻结在每条 decision 上，让历史决策能用当时的算法复算。
	AlgorithmVersion       string
	CurrencyAllowlist      []domain.Currency
	DefaultChallengePeriod time.Duration
	LockTTL                time.Duration
	ChallengeTTL           time.Duration
	Now                    func() time.Time
	NewID                  func() string
}

type Service struct {
	store                  Store
	provider               settlement.Provider
	evidence               EvidenceSource
	signer                 domain.Signer
	algorithmVersion       string
	allowlist              map[domain.Currency]struct{}
	defaultChallengePeriod time.Duration
	lockTTL                time.Duration
	challengeTTL           time.Duration
	now                    func() time.Time
	newID                  func() string
}

func NewService(options Options) (*Service, error) {
	if options.Store == nil {
		return nil, invalid("store")
	}
	if options.Provider == nil {
		return nil, invalid("settlement_provider")
	}
	if options.Signer == nil {
		return nil, invalid("signer")
	}
	if options.AlgorithmVersion == "" {
		return nil, invalid("algorithm_version")
	}
	if len(options.CurrencyAllowlist) == 0 {
		options.CurrencyAllowlist = []domain.Currency{domain.CurrencyUSDC, domain.CurrencyUSD}
	}
	allowlist := make(map[domain.Currency]struct{}, len(options.CurrencyAllowlist))
	for _, currency := range options.CurrencyAllowlist {
		if !domain.ValidCurrency(currency) {
			return nil, invalid("currency")
		}
		allowlist[currency] = struct{}{}
	}
	if options.DefaultChallengePeriod < 0 {
		return nil, invalid("challenge_period")
	}
	if options.LockTTL <= 0 {
		options.LockTTL = defaultLockTTL
	}
	if options.ChallengeTTL <= 0 {
		options.ChallengeTTL = defaultChallengeTTL
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	return &Service{
		store: options.Store, provider: options.Provider, evidence: options.Evidence,
		signer: options.Signer, algorithmVersion: options.AlgorithmVersion,
		allowlist: allowlist, defaultChallengePeriod: options.DefaultChallengePeriod,
		lockTTL: options.LockTTL, challengeTTL: options.ChallengeTTL,
		now: options.Now, newID: options.NewID,
	}, nil
}

// ---------------------------------------------------------------------------
// Sponsor escrow
// ---------------------------------------------------------------------------

type TopUpEscrow struct {
	RequestID   string
	Currency    string
	AmountMinor int64
}

// TopUpEscrow 为 sponsor 租户注入托管余额。RequestID 是幂等键：
// 同一次充值被重复投递时余额不会二次增加（doc §10）。
func (s *Service) TopUpEscrow(ctx context.Context, principal auth.Principal, command TopUpEscrow) (Envelope[EscrowView], error) {
	tenantID, err := s.sponsorTenant(principal)
	if err != nil {
		return Envelope[EscrowView]{}, err
	}
	if command.RequestID == "" {
		return Envelope[EscrowView]{}, invalid("request_id")
	}
	currency, err := s.currency(command.Currency)
	if err != nil {
		return Envelope[EscrowView]{}, err
	}
	if command.AmountMinor <= 0 || command.AmountMinor > domain.MaxAmountMinor {
		return Envelope[EscrowView]{}, invalid("amount_minor")
	}

	var view EscrowView
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		if err := tx.Escrow().EnsureAccount(ctx, tenantID, currency); err != nil {
			return err
		}
		available, locked, err := domain.Deltas(domain.EntryTopup, command.AmountMinor)
		if err != nil {
			return err
		}
		if _, err := tx.Escrow().ApplyEntry(ctx, domain.EscrowEntry{
			TenantID: tenantID, Currency: currency, EntryType: domain.EntryTopup,
			AmountMinor: command.AmountMinor, AvailableDelta: available, LockedDelta: locked,
			ReferenceKind: "topup", ReferenceID: command.RequestID,
			IdempotencyKey: "topup:" + command.RequestID, CreatedAt: now,
		}); err != nil {
			return err
		}
		account, err := tx.Escrow().Get(ctx, tenantID, currency)
		if err != nil {
			return err
		}
		view = EscrowView{
			Currency: string(account.Currency), AvailableMinor: account.AvailableMinor,
			LockedMinor: account.LockedMinor,
		}
		return nil
	})
	if err != nil {
		return Envelope[EscrowView]{}, err
	}
	return Envelope[EscrowView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// GetEscrow 返回 sponsor 自己的余额。
func (s *Service) GetEscrow(ctx context.Context, principal auth.Principal, currencyCode string) (Envelope[EscrowView], error) {
	tenantID, err := s.sponsorTenant(principal)
	if err != nil {
		return Envelope[EscrowView]{}, err
	}
	currency, err := s.currency(currencyCode)
	if err != nil {
		return Envelope[EscrowView]{}, err
	}
	var view EscrowView
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		account, err := tx.Escrow().Get(ctx, tenantID, currency)
		if err != nil {
			return err
		}
		view = EscrowView{
			Currency: string(account.Currency), AvailableMinor: account.AvailableMinor,
			LockedMinor: account.LockedMinor,
		}
		return nil
	})
	if err != nil {
		return Envelope[EscrowView]{}, err
	}
	return Envelope[EscrowView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// ---------------------------------------------------------------------------
// RewardPolicy
// ---------------------------------------------------------------------------

type CreateRewardPolicy struct {
	RequestID               string
	TaskID                  string
	Currency                string
	GrossAmountMinor        int64
	CriterionWeights        []domain.CriterionWeight
	MaintainerShareBps      int
	ReviewerPoolShareBps    int
	PlatformFeeBps          int
	DisputeReserveBps       int
	QualityMultiplierMinBps int
	QualityMultiplierMaxBps int
	ChallengePeriod         time.Duration
	ExpiresAt               time.Time
}

// CreateRewardPolicy 创建一条 unfunded 的奖励契约。
//
// 创建后字段不可变：改金额、criterion 权重或挑战期只能新建一条 policy，
// 因此已被 Claim 锁定的契约永远不会被 Issue 更新改写（doc §10）。
func (s *Service) CreateRewardPolicy(ctx context.Context, principal auth.Principal, command CreateRewardPolicy) (Envelope[PolicyView], error) {
	tenantID, err := s.sponsorTenant(principal)
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	if command.TaskID == "" {
		return Envelope[PolicyView]{}, invalid("task_id")
	}
	currency, err := s.currency(command.Currency)
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	challengePeriod := command.ChallengePeriod
	if challengePeriod == 0 {
		challengePeriod = s.defaultChallengePeriod
	}
	qualityMin, qualityMax := command.QualityMultiplierMinBps, command.QualityMultiplierMaxBps
	if qualityMin == 0 && qualityMax == 0 {
		qualityMin, qualityMax = domain.BasisPointsScale, domain.BasisPointsScale
	}

	var view PolicyView
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		expiresAt := command.ExpiresAt
		if expiresAt.IsZero() {
			expiresAt = now.Add(s.lockTTL)
		}
		policy, err := domain.NewPolicy(domain.NewPolicyParams{
			ID: s.newID(), ResourceTenantID: tenantID, TaskID: command.TaskID,
			Currency: currency, SettlementProvider: s.provider.Name(),
			GrossAmountMinor: command.GrossAmountMinor, CriterionWeights: command.CriterionWeights,
			MaintainerShareBps: command.MaintainerShareBps, ReviewerPoolShareBps: command.ReviewerPoolShareBps,
			PlatformFeeBps: command.PlatformFeeBps, DisputeReserveBps: command.DisputeReserveBps,
			QualityMultiplierMinBps: qualityMin, QualityMultiplierMaxBps: qualityMax,
			ChallengePeriod: challengePeriod, ExpiresAt: expiresAt, CreatedAt: now,
		})
		if err != nil {
			return err
		}
		if err := tx.Policies().Insert(ctx, policy); err != nil {
			return err
		}
		view = policyView(*policy)
		return nil
	})
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	return Envelope[PolicyView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

type FundRewardPolicy struct {
	PolicyID    string
	AmountMinor int64
}

// FundRewardPolicy 把 policy 从 unfunded 推进到 funded。
//
// 这里只校验余额充足而**不预留**资金：真正的预留发生在 Claim 时刻，
// 那才是"这笔钱被某个 Execution 占用"的时点。重复调用返回状态冲突。
func (s *Service) FundRewardPolicy(ctx context.Context, principal auth.Principal, command FundRewardPolicy) (Envelope[PolicyView], error) {
	tenantID, err := s.sponsorTenant(principal)
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	if command.PolicyID == "" {
		return Envelope[PolicyView]{}, invalid("policy_id")
	}
	var view PolicyView
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		policy, err := tx.Policies().GetByID(ctx, tenantID, command.PolicyID)
		if err != nil {
			return err
		}
		amount := command.AmountMinor
		if amount == 0 {
			amount = policy.GrossAmountMinor
		}
		account, err := tx.Escrow().Get(ctx, tenantID, policy.Currency)
		if err != nil {
			return err
		}
		if account.AvailableMinor < amount {
			return domain.ErrInsufficientEscrow
		}
		if err := policy.Apply(domain.PolicyIntentFund, amount, now); err != nil {
			return err
		}
		if err := tx.Policies().Save(ctx, policy); err != nil {
			return err
		}
		view = policyView(*policy)
		return nil
	})
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	return Envelope[PolicyView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// CancelRewardPolicy 取消尚未被锁定的契约。存在活跃锁时返回状态冲突：
// 已经承诺给某次执行的资金不能被 sponsor 单方面撤回。
func (s *Service) CancelRewardPolicy(ctx context.Context, principal auth.Principal, policyID string) (Envelope[PolicyView], error) {
	tenantID, err := s.sponsorTenant(principal)
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	if policyID == "" {
		return Envelope[PolicyView]{}, invalid("policy_id")
	}
	var view PolicyView
	var serverTime time.Time
	err = s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		policy, err := tx.Policies().GetByID(ctx, tenantID, policyID)
		if err != nil {
			return err
		}
		locked, err := tx.Policies().LockedAmount(ctx, tenantID, policyID)
		if err != nil {
			return err
		}
		if locked > 0 {
			return domain.ErrStateConflict
		}
		if err := policy.Apply(domain.PolicyIntentCancel, 0, now); err != nil {
			return err
		}
		if err := tx.Policies().Save(ctx, policy); err != nil {
			return err
		}
		view = policyView(*policy)
		return nil
	})
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	return Envelope[PolicyView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// GetTaskReward 返回某个公共任务当前的奖励契约，匿名可读。
func (s *Service) GetTaskReward(ctx context.Context, resourceTenantID, taskID string) (Envelope[PolicyView], error) {
	if resourceTenantID == "" || taskID == "" {
		return Envelope[PolicyView]{}, invalid("task_id")
	}
	var view PolicyView
	var serverTime time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		policy, err := tx.Policies().GetActiveByTask(ctx, resourceTenantID, taskID)
		if err != nil {
			return err
		}
		view = policyView(*policy)
		return nil
	})
	if err != nil {
		return Envelope[PolicyView]{}, err
	}
	return Envelope[PolicyView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

// GetExecutionReward 返回某次执行的锁定与决策摘要。
func (s *Service) GetExecutionReward(ctx context.Context, resourceTenantID, executionID string) (Envelope[LockView], error) {
	if resourceTenantID == "" || executionID == "" {
		return Envelope[LockView]{}, invalid("execution_id")
	}
	var view LockView
	var serverTime time.Time
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		serverTime = now
		lock, err := tx.Locks().GetByExecution(ctx, resourceTenantID, executionID)
		if err != nil {
			return err
		}
		decisionHash := ""
		if decision, err := tx.Decisions().GetByLock(ctx, resourceTenantID, lock.ID); err == nil {
			decisionHash = decision.DecisionHash
		} else if !isNotFound(err) {
			return err
		}
		view = lockView(*lock, decisionHash)
		return nil
	})
	if err != nil {
		return Envelope[LockView]{}, err
	}
	return Envelope[LockView]{Data: view, Meta: Meta{ServerTime: serverTime}}, nil
}

func (s *Service) sponsorTenant(principal auth.Principal) (string, error) {
	if principal.Type != auth.PrincipalTypeHuman || principal.TenantID == "" {
		return "", domain.ErrForbidden
	}
	return principal.TenantID, nil
}

func (s *Service) currency(code string) (domain.Currency, error) {
	currency := domain.Currency(code)
	if code == "" {
		currency = domain.CurrencyUSDC
	}
	if _, allowed := s.allowlist[currency]; !allowed {
		return "", invalid("currency")
	}
	return currency, nil
}

func invalid(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func isNotFound(err error) bool {
	return domain.CodeOf(err) == "not_found"
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

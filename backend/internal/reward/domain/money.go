package domain

import "sort"

// Currency 是结算币种。第一版只支持 allowlist 内的稳定币与法币，
// 不发行任何原生代币（doc §5.1）。
type Currency string

const (
	CurrencyUSDC Currency = "USDC"
	CurrencyUSD  Currency = "USD"
)

// BasisPointsScale 是 basis points 的分母：10000 bps = 100%。
//
// 分成比例一律用整数 bps 而不是小数：浮点没有唯一的 JSON 表示，
// 会让 policy_hash / decision_hash 无法被第三方跨语言复算（doc §10）。
const BasisPointsScale = 10000

// MaxAmountMinor 是单笔金额上限（minor units）。它存在的唯一目的是让
// amount * bps 的中间结果不会溢出 int64：1e12 * 20000 = 2e16 « 9.2e18。
const MaxAmountMinor int64 = 1_000_000_000_000

// Money 是金额的唯一表示：整数 minor units + 币种，禁止浮点。
type Money struct {
	AmountMinor int64    `json:"amount_minor"`
	Currency    Currency `json:"currency"`
}

func NewMoney(amountMinor int64, currency Currency) (Money, error) {
	if !ValidCurrency(currency) {
		return Money{}, invalid("currency")
	}
	if amountMinor < 0 || amountMinor > MaxAmountMinor {
		return Money{}, invalid("amount_minor")
	}
	return Money{AmountMinor: amountMinor, Currency: currency}, nil
}

func (m Money) IsZero() bool { return m.AmountMinor == 0 }

// Add 返回新的 Money，绝不就地修改。币种不一致直接报错而不是静默换算。
func (m Money) Add(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, invalid("currency")
	}
	return NewMoney(m.AmountMinor+other.AmountMinor, m.Currency)
}

func (m Money) Sub(other Money) (Money, error) {
	if m.Currency != other.Currency {
		return Money{}, invalid("currency")
	}
	return NewMoney(m.AmountMinor-other.AmountMinor, m.Currency)
}

func ValidCurrency(currency Currency) bool {
	return currency == CurrencyUSDC || currency == CurrencyUSD
}

// ParseCurrencyAllowlist 把配置里的 CSV 解析成去重排序后的币种集合。
// 出现 allowlist 之外的 code 直接报错，避免部署方误配出无法结算的币种。
func ParseCurrencyAllowlist(values []string) ([]Currency, error) {
	seen := make(map[Currency]struct{}, len(values))
	for _, raw := range values {
		currency := Currency(raw)
		if !ValidCurrency(currency) {
			return nil, invalid("currency")
		}
		seen[currency] = struct{}{}
	}
	if len(seen) == 0 {
		return []Currency{CurrencyUSDC, CurrencyUSD}, nil
	}
	result := make([]Currency, 0, len(seen))
	for currency := range seen {
		result = append(result, currency)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

// ApplyBps 返回 amount 的 bps 份额，向下取整。
//
// 一律向下取整、余数留在 unallocated 里，是为了让分账在整数域内必然闭合：
// 任何"四舍五入"都可能让各份额之和超过总额。
func ApplyBps(amountMinor int64, bps int) int64 {
	if amountMinor <= 0 || bps <= 0 {
		return 0
	}
	return amountMinor * int64(bps) / BasisPointsScale
}

// ValidBps 判断单个比例是否落在 [0, 10000]。
func ValidBps(bps int) bool { return bps >= 0 && bps <= BasisPointsScale }

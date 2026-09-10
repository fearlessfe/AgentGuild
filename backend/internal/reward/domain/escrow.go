package domain

import "time"

// EntryType 是 escrow 分录的类型。
//
// 资金只在 available 与 locked 两个池之间搬运，或由 topup 从外部注入：
// 任何其他形状的变动都无法通过数据库的守恒 CHECK。
type EntryType string

const (
	EntryTopup   EntryType = "topup"
	EntryLock    EntryType = "lock"
	EntryRelease EntryType = "release"
	EntryRefund  EntryType = "refund"
)

// EscrowAccount 是某个 sponsor 租户在某个币种下的托管余额。
type EscrowAccount struct {
	TenantID       string
	Currency       Currency
	AvailableMinor int64
	LockedMinor    int64
	UpdatedAt      time.Time
}

// EscrowEntry 是一次余额变动的不可变分录。
type EscrowEntry struct {
	ID             int64
	TenantID       string
	Currency       Currency
	EntryType      EntryType
	AmountMinor    int64
	AvailableDelta int64
	LockedDelta    int64
	ReferenceKind  string
	ReferenceID    string
	IdempotencyKey string
	CreatedAt      time.Time
}

// Deltas 返回某类分录对两个池的影响。它与迁移里的守恒 CHECK 一一对应，
// 两处必须同时修改才能生效——这是刻意的双重保险。
func Deltas(entryType EntryType, amountMinor int64) (availableDelta, lockedDelta int64, err error) {
	if amountMinor <= 0 {
		return 0, 0, invalid("amount_minor")
	}
	switch entryType {
	case EntryTopup:
		return amountMinor, 0, nil
	case EntryLock:
		return -amountMinor, amountMinor, nil
	case EntryRelease:
		return 0, -amountMinor, nil
	case EntryRefund:
		return amountMinor, -amountMinor, nil
	default:
		return 0, 0, invalid("entry_type")
	}
}

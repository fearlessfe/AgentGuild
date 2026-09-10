package domain

import "errors"

// Error 是奖励模块的领域错误，形状与其他模块保持一致，便于 transport 层
// 统一映射错误码。
type Error struct {
	Code    string
	Message string
	Field   string
}

func (e Error) Error() string { return e.Message }

func (e Error) Is(target error) bool {
	switch other := target.(type) {
	case Error:
		return e.Code == other.Code
	case *Error:
		return other != nil && e.Code == other.Code
	default:
		return false
	}
}

var (
	ErrInvalidArgument = &Error{Code: "invalid_argument", Message: "reward argument is invalid"}
	ErrNotFound        = &Error{Code: "not_found", Message: "reward resource not found"}
	ErrForbidden       = &Error{Code: "forbidden", Message: "reward operation is not authorized"}
	ErrStateConflict   = &Error{Code: "state_conflict", Message: "reward state transition is not allowed"}
	// ErrInsufficientEscrow 表示 sponsor 的可用托管余额不足以支撑本次锁定。
	// 它会让 Claim 整体失败、任务保持 open：不允许无资金背书的 claim。
	ErrInsufficientEscrow = &Error{Code: "insufficient_escrow", Message: "sponsor escrow balance is insufficient"}
	// ErrPolicyImmutable 表示试图修改一条已被 Claim 锁定的 policy。
	ErrPolicyImmutable = &Error{Code: "policy_immutable", Message: "reward policy is immutable once locked"}
)

func invalid(field string) error {
	return &Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func CodeOf(err error) string {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	return ""
}

func FieldOf(err error) string {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Field
	}
	return ""
}

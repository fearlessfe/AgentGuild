package domain

import "errors"

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
	ErrInvalidArgument = &Error{Code: "invalid_argument", Message: "request argument is invalid"}
	ErrForbidden       = &Error{Code: "forbidden", Message: "task participation is not authorized"}
	ErrNotFound        = &Error{Code: "not_found", Message: "task participation grant not found"}
	ErrStateConflict   = &Error{Code: "state_conflict", Message: "task participation grant state conflict"}
	ErrExpired         = &Error{Code: "grant_expired", Message: "task participation grant expired"}
	ErrRevoked         = &Error{Code: "grant_revoked", Message: "task participation grant revoked"}
)

func invalid(field string) error {
	return &Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func FieldOf(err error) string {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Field
	}
	return ""
}

func CodeOf(err error) string {
	var domainErr *Error
	if errors.As(err, &domainErr) {
		return domainErr.Code
	}
	return ""
}

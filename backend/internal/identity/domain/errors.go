package domain

import (
	"errors"
	"time"
)

type Error struct {
	Code       string
	Message    string
	Field      string
	RetryAfter time.Duration
}

func (e Error) Error() string {
	return e.Message
}

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
	ErrForbidden       = &Error{Code: "forbidden", Message: "actor is not allowed to perform this action"}
	ErrNotFound        = &Error{Code: "not_found", Message: "resource not found"}
	ErrStateConflict   = &Error{Code: "state_conflict", Message: "state transition is not allowed"}
	ErrTokenExpired    = &Error{Code: "token_expired", Message: "activation token is expired or invalid"}
	ErrTokenRevoked    = &Error{Code: "token_revoked", Message: "token has been revoked"}
	ErrRateLimited     = &Error{Code: "rate_limited", Message: "rate limit exceeded"}
)

func invalidArgument(field string) error {
	return &Error{
		Code:    "invalid_argument",
		Message: field + " is invalid",
		Field:   field,
	}
}

func CodeOf(err error) string {
	if domainErr := errorOf(err); domainErr != nil {
		return domainErr.Code
	}
	return ""
}

func FieldOf(err error) string {
	if domainErr := errorOf(err); domainErr != nil {
		return domainErr.Field
	}
	return ""
}

func RetryAfterOf(err error) time.Duration {
	if domainErr := errorOf(err); domainErr != nil {
		return domainErr.RetryAfter
	}
	return 0
}

func errorOf(err error) *Error {
	var pointer *Error
	if errors.As(err, &pointer) {
		return pointer
	}
	var value Error
	if errors.As(err, &value) {
		return &value
	}
	return nil
}

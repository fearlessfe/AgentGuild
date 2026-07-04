package domain

import (
	"errors"
	"time"
)

// Error is a domain-level error with a stable code.
type Error struct {
	Code       string
	Message    string
	Field      string
	RetryAfter time.Duration
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
	ErrInvalidArgument  = &Error{Code: "invalid_argument", Message: "request argument is invalid"}
	ErrStateConflict    = &Error{Code: "state_conflict", Message: "state transition is not allowed"}
	ErrImmutableResource = &Error{Code: "immutable_resource", Message: "version content is immutable after persistence"}
	ErrNoChange         = &Error{Code: "no_change", Message: "configuration has not changed"}
	ErrNotFound         = &Error{Code: "not_found", Message: "resource not found"}
	ErrForbidden        = &Error{Code: "forbidden", Message: "actor is not allowed to perform this action"}
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

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
	ErrInvalidArgument     = &Error{Code: "invalid_argument", Message: "request argument is invalid"}
	ErrPublicationRejected = &Error{Code: "publication_rejected", Message: "task did not pass the public publication gates"}
	ErrStateConflict       = &Error{Code: "state_conflict", Message: "public task state transition is not allowed"}
	ErrNotFound            = &Error{Code: "not_found", Message: "public task not found"}
	ErrForbidden           = &Error{Code: "forbidden", Message: "public task participation is not authorized"}
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

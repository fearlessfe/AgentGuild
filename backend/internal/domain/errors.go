package domain

type Error struct {
	Code    string
	Message string
	Field   string
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
	ErrStateConflict = &Error{Code: "state_conflict", Message: "state transition is not allowed"}
	ErrLeaseExpired  = &Error{Code: "lease_expired", Message: "lease is expired or stale"}
	ErrForbidden     = &Error{Code: "forbidden", Message: "actor is not allowed to perform this transition"}
)

func invalidArgument(field string) error {
	return &Error{
		Code:    "invalid_argument",
		Message: field + " is invalid",
		Field:   field,
	}
}

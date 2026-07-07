package domain

import (
	"errors"
	"strings"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

const (
	DedupeUpdate = "update"
	DedupeSkip   = "skip"
)

type Rule struct {
	ID              string
	TenantID        string
	Repo            string
	IncludeLabels   []string
	ExcludeLabels   []string
	IssueState      string
	TaskType        string
	DefaultPriority string
	DedupeStrategy  string
	Enabled         bool
	LastSyncedAt    time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

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

var ErrInvalidArgument = &Error{Code: "invalid_argument", Message: "request argument is invalid"}

func NewRule(
	id, tenantID, repo, taskType string,
	includeLabels, excludeLabels []string,
	issueState, defaultPriority, dedupeStrategy string,
	enabled bool,
	lastSyncedAt, createdAt, updatedAt time.Time,
) (*Rule, error) {
	if !validRepo(repo) {
		return nil, invalidArgument("repo")
	}
	if !validIssueState(issueState) {
		return nil, invalidArgument("issue_state")
	}
	if taskType == "" {
		return nil, invalidArgument("task_type")
	}
	if !validDedupeStrategy(dedupeStrategy) {
		return nil, invalidArgument("dedupe_strategy")
	}

	return &Rule{
		ID:              id,
		TenantID:        tenantID,
		Repo:            repo,
		IncludeLabels:   append([]string(nil), includeLabels...),
		ExcludeLabels:   append([]string(nil), excludeLabels...),
		IssueState:      issueState,
		TaskType:        taskType,
		DefaultPriority: defaultPriority,
		DedupeStrategy:  dedupeStrategy,
		Enabled:         enabled,
		LastSyncedAt:    lastSyncedAt,
		CreatedAt:       createdAt,
		UpdatedAt:       updatedAt,
	}, nil
}

func (r Rule) Matches(issue git.Issue) bool {
	if r.IssueState != "all" && issue.State != r.IssueState {
		return false
	}

	labels := make(map[string]struct{}, len(issue.Labels))
	for _, label := range issue.Labels {
		labels[label] = struct{}{}
	}

	for _, label := range r.ExcludeLabels {
		if _, ok := labels[label]; ok {
			return false
		}
	}

	if len(r.IncludeLabels) == 0 {
		return true
	}
	for _, label := range r.IncludeLabels {
		if _, ok := labels[label]; ok {
			return true
		}
	}
	return false
}

func invalidArgument(field string) error {
	return &Error{
		Code:    "invalid_argument",
		Message: field + " is invalid",
		Field:   field,
	}
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

func validRepo(repo string) bool {
	parts := strings.Split(repo, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

func validIssueState(state string) bool {
	switch state {
	case "open", "closed", "all":
		return true
	default:
		return false
	}
}

func validDedupeStrategy(strategy string) bool {
	switch strategy {
	case DedupeUpdate, DedupeSkip:
		return true
	default:
		return false
	}
}

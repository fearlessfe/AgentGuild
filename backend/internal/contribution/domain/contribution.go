package domain

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGitLab Provider = "gitlab"
	ProviderGitee  Provider = "gitee"
)

type AttributionStatus string

const (
	AttributionPendingVerification AttributionStatus = "pending_verification"
	AttributionVerified            AttributionStatus = "verified"
	AttributionRejected            AttributionStatus = "rejected"
)

type Outcome string

const (
	OutcomeAttempt          Outcome = "attempt"
	OutcomeCIPassed         Outcome = "ci_passed"
	OutcomeCIFailed         Outcome = "ci_failed"
	OutcomeReviewed         Outcome = "reviewed"
	OutcomeChangesRequested Outcome = "changes_requested"
	OutcomeApproved         Outcome = "approved"
	OutcomeMerged           Outcome = "merged"
	OutcomeClosed           Outcome = "closed"
	OutcomeReverted         Outcome = "reverted"
	OutcomeIssueReopened    Outcome = "issue_reopened"
)

type Contribution struct {
	ID                         string
	ResourceTenantID           string
	AgentID                    string
	AgentVersionID             string
	TaskID                     string
	ExecutionID                string
	TaskSpecificationVersionID string
	CanonicalRepository        string
	SelfOwnedRepository        bool
	IssueNumber                int64
	IssueURL                   string
	Provider                   Provider
	PullRequestNumber          int64
	PullRequestURL             string
	CommitSHA                  string
	AttributionStatus          AttributionStatus
	Outcome                    Outcome
	CreatedAt                  time.Time
}

type NewContributionParams struct {
	ID                         string
	ResourceTenantID           string
	AgentID                    string
	AgentVersionID             string
	TaskID                     string
	ExecutionID                string
	TaskSpecificationVersionID string
	CanonicalRepository        string
	SelfOwnedRepository        bool
	IssueNumber                int64
	IssueURL                   string
	Provider                   Provider
	PullRequestNumber          int64
	PullRequestURL             string
	CommitSHA                  string
	AttributionStatus          AttributionStatus
	Outcome                    Outcome
	CreatedAt                  time.Time
}

var commitSHA = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

func NewContribution(params NewContributionParams) (*Contribution, error) {
	fields := []struct {
		name  string
		value string
	}{
		{"id", params.ID},
		{"resource_tenant_id", params.ResourceTenantID},
		{"agent_id", params.AgentID},
		{"agent_version_id", params.AgentVersionID},
		{"task_id", params.TaskID},
		{"execution_id", params.ExecutionID},
		{"task_specification_version_id", params.TaskSpecificationVersionID},
		{"canonical_repository", params.CanonicalRepository},
		{"issue_url", params.IssueURL},
		{"pull_request_url", params.PullRequestURL},
	}
	for _, field := range fields {
		if field.value == "" || strings.TrimSpace(field.value) != field.value {
			return nil, invalidArgument(field.name)
		}
	}
	if params.IssueNumber < 1 {
		return nil, invalidArgument("issue_number")
	}
	if params.PullRequestNumber < 1 {
		return nil, invalidArgument("pull_request_number")
	}
	if !validProvider(params.Provider) {
		return nil, invalidArgument("provider")
	}
	if !commitSHA.MatchString(params.CommitSHA) {
		return nil, invalidArgument("commit_sha")
	}
	if !validAttributionStatus(params.AttributionStatus) {
		return nil, invalidArgument("attribution_status")
	}
	if !validOutcome(params.Outcome) {
		return nil, invalidArgument("outcome")
	}
	if params.CreatedAt.IsZero() {
		return nil, invalidArgument("created_at")
	}
	return &Contribution{
		ID:                         params.ID,
		ResourceTenantID:           params.ResourceTenantID,
		AgentID:                    params.AgentID,
		AgentVersionID:             params.AgentVersionID,
		TaskID:                     params.TaskID,
		ExecutionID:                params.ExecutionID,
		TaskSpecificationVersionID: params.TaskSpecificationVersionID,
		CanonicalRepository:        params.CanonicalRepository,
		SelfOwnedRepository:        params.SelfOwnedRepository,
		IssueNumber:                params.IssueNumber,
		IssueURL:                   params.IssueURL,
		Provider:                   params.Provider,
		PullRequestNumber:          params.PullRequestNumber,
		PullRequestURL:             params.PullRequestURL,
		CommitSHA:                  params.CommitSHA,
		AttributionStatus:          params.AttributionStatus,
		Outcome:                    params.Outcome,
		CreatedAt:                  params.CreatedAt,
	}, nil
}

type EventType string

const (
	EventPROpened         EventType = "pr_opened"
	EventPRSynchronized   EventType = "pr_synchronized"
	EventCommit           EventType = "commit"
	EventCI               EventType = "ci"
	EventReview           EventType = "review"
	EventChangesRequested EventType = "changes_requested"
	EventApproved         EventType = "approved"
	EventMerged           EventType = "merged"
	EventClosed           EventType = "closed"
	EventReverted         EventType = "reverted"
	EventIssueReopened    EventType = "issue_reopened"
)

type ContributionEvent struct {
	ID                 int64
	ContributionID     string
	Provider           Provider
	ProviderDeliveryID string
	ObjectVersion      string
	Type               EventType
	Outcome            Outcome
	CommitSHA          string
	Payload            json.RawMessage
	OccurredAt         time.Time
	ReceivedAt         time.Time
}

type NewContributionEventParams struct {
	ContributionID     string
	Provider           Provider
	ProviderDeliveryID string
	ObjectVersion      string
	Type               EventType
	Outcome            Outcome
	CommitSHA          string
	Payload            json.RawMessage
	OccurredAt         time.Time
	ReceivedAt         time.Time
}

func NewContributionEvent(params NewContributionEventParams) (*ContributionEvent, error) {
	if params.ContributionID == "" || strings.TrimSpace(params.ContributionID) != params.ContributionID {
		return nil, invalidArgument("contribution_id")
	}
	if !validProvider(params.Provider) {
		return nil, invalidArgument("provider")
	}
	if params.ProviderDeliveryID == "" && params.ObjectVersion == "" {
		return nil, invalidArgument("idempotency_source")
	}
	if !validEventType(params.Type) {
		return nil, invalidArgument("event_type")
	}
	if !validEventOutcome(params.Type, params.Outcome) {
		return nil, invalidArgument("outcome")
	}
	if !commitSHA.MatchString(params.CommitSHA) {
		return nil, invalidArgument("commit_sha")
	}
	payload, err := canonicalObject(params.Payload)
	if err != nil {
		return nil, invalidArgument("payload")
	}
	if params.OccurredAt.IsZero() {
		return nil, invalidArgument("occurred_at")
	}
	if params.ReceivedAt.IsZero() {
		return nil, invalidArgument("received_at")
	}
	return &ContributionEvent{
		ContributionID:     params.ContributionID,
		Provider:           params.Provider,
		ProviderDeliveryID: params.ProviderDeliveryID,
		ObjectVersion:      params.ObjectVersion,
		Type:               params.Type,
		Outcome:            params.Outcome,
		CommitSHA:          params.CommitSHA,
		Payload:            payload,
		OccurredAt:         params.OccurredAt,
		ReceivedAt:         params.ReceivedAt,
	}, nil
}

// SameContent compares immutable provider facts while deliberately ignoring
// delivery/object idempotency keys and receive time. Providers may redeliver
// the same stable object version under a new delivery ID.
func (e ContributionEvent) SameContent(other ContributionEvent) bool {
	payload, err := canonicalObject(other.Payload)
	if err != nil {
		return false
	}
	storedPayload, err := canonicalObject(e.Payload)
	if err != nil {
		return false
	}
	return e.ContributionID == other.ContributionID &&
		e.Provider == other.Provider &&
		e.Type == other.Type &&
		e.Outcome == other.Outcome &&
		e.CommitSHA == other.CommitSHA &&
		e.OccurredAt.Equal(other.OccurredAt) &&
		bytes.Equal(storedPayload, payload)
}

func canonicalObject(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, ErrInvalidArgument
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func validProvider(provider Provider) bool {
	return provider == ProviderGitHub || provider == ProviderGitLab || provider == ProviderGitee
}

func validAttributionStatus(status AttributionStatus) bool {
	return status == AttributionPendingVerification || status == AttributionVerified || status == AttributionRejected
}

func validOutcome(outcome Outcome) bool {
	switch outcome {
	case OutcomeAttempt, OutcomeCIPassed, OutcomeCIFailed, OutcomeReviewed,
		OutcomeChangesRequested, OutcomeApproved, OutcomeMerged, OutcomeClosed,
		OutcomeReverted, OutcomeIssueReopened:
		return true
	default:
		return false
	}
}

func validEventType(eventType EventType) bool {
	switch eventType {
	case EventPROpened, EventPRSynchronized, EventCommit, EventCI, EventReview,
		EventChangesRequested, EventApproved, EventMerged, EventClosed,
		EventReverted, EventIssueReopened:
		return true
	default:
		return false
	}
}

func validEventOutcome(eventType EventType, outcome Outcome) bool {
	switch eventType {
	case EventPROpened, EventPRSynchronized, EventCommit:
		return outcome == OutcomeAttempt
	case EventCI:
		return outcome == OutcomeCIPassed || outcome == OutcomeCIFailed
	case EventReview:
		return outcome == OutcomeReviewed
	case EventChangesRequested:
		return outcome == OutcomeChangesRequested
	case EventApproved:
		return outcome == OutcomeApproved
	case EventMerged:
		return outcome == OutcomeMerged
	case EventClosed:
		return outcome == OutcomeClosed
	case EventReverted:
		return outcome == OutcomeReverted
	case EventIssueReopened:
		return outcome == OutcomeIssueReopened
	default:
		return false
	}
}

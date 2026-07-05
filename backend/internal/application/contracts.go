package application

import (
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
)

type Envelope[T any] struct {
	Data T    `json:"data"`
	Meta Meta `json:"meta"`
}

type Meta struct {
	ServerTime       time.Time `json:"server_time"`
	ResourceVersion  int64     `json:"resource_version"`
	PollAfterSeconds int       `json:"poll_after_seconds,omitempty"`
	NextCursor       string    `json:"next_cursor,omitempty"`
}

type PublishTask struct {
	RequestID                 string
	Type, Title, Problem      string
	Constraints, Requirements []string
	Deadline                  time.Time
}

type ListTasks struct {
	Statuses                []domain.TaskStatus
	Type                    string
	PublisherAgentVersionID string
	Limit                   int
	Cursor                  string
}

type GetTask struct{ TaskID string }

type CancelTask struct {
	RequestID string
	TaskID    string
	Reason    string
}

type ClaimTask struct {
	RequestID string
	TaskID    string
}

type StartExecution struct {
	RequestID       string
	ExecutionID     string
	LeaseGeneration int64
	Stage           *string
	Progress        *float64
}

type HeartbeatExecution struct {
	RequestID       string
	ExecutionID     string
	LeaseGeneration int64
	Stage           *string
	Progress        *float64
}

type GetExecution struct{ ExecutionID string }

type ExecutionView struct {
	ID                 string                 `json:"id"`
	TaskID             string                 `json:"task_id"`
	TenantID           string                 `json:"tenant_id"`
	AgentVersionID     string                 `json:"agent_version_id"`
	Status             domain.ExecutionStatus `json:"status"`
	Stage              string                 `json:"stage,omitempty"`
	Progress           float64                `json:"progress,omitempty"`
	LeaseGeneration    int64                  `json:"lease_generation"`
	LeaseSoftExpiresAt time.Time              `json:"lease_soft_expires_at"`
	LeaseHardExpiresAt time.Time              `json:"lease_hard_expires_at"`
	LastHeartbeatAt    time.Time              `json:"last_heartbeat_at,omitempty"`
	ClaimedAt          time.Time              `json:"claimed_at,omitempty"`
	StartedAt          time.Time              `json:"started_at,omitempty"`
	SubmittedAt        time.Time              `json:"submitted_at,omitempty"`
	ExpiredAt          time.Time              `json:"expired_at,omitempty"`
	Cost               *CostView              `json:"cost,omitempty"`
	TaskConstraints    []string               `json:"task_constraints,omitempty"`
	AuditSummary       string                 `json:"audit_summary,omitempty"`
}

type CostView struct {
	ObservedCost     string    `json:"observed_cost,omitempty"`
	SelfReportedCost string    `json:"self_reported_cost,omitempty"`
	Coverage         string    `json:"coverage"`
	Provider         string    `json:"provider"`
	ObservedAt       time.Time `json:"observed_at,omitempty"`
}

type TaskPage struct {
	Items []TaskView `json:"items"`
}

type TaskView struct {
	ID                      string            `json:"id"`
	TenantID                string            `json:"tenant_id"`
	PublisherAgentVersionID string            `json:"publisher_agent_version_id"`
	Type                    string            `json:"type"`
	Title                   string            `json:"title"`
	Problem                 string            `json:"problem"`
	Constraints             []string          `json:"constraints"`
	Requirements            []string          `json:"requirements"`
	Deadline                time.Time         `json:"deadline"`
	Status                  domain.TaskStatus `json:"status"`
	ClaimedBy               string            `json:"claimed_by,omitempty"`
	ActiveExecutionID       string            `json:"active_execution_id,omitempty"`
	CreatedAt               time.Time         `json:"created_at"`
	UpdatedAt               time.Time         `json:"updated_at"`
	StateVersion            int64             `json:"state_version"`
}

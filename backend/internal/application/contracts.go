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
}

type HeartbeatExecution struct {
	RequestID       string
	ExecutionID     string
	LeaseGeneration int64
}

type GetExecution struct{ ExecutionID string }

type ExecutionView struct {
	ID                 string                 `json:"id"`
	TaskID             string                 `json:"task_id"`
	TenantID           string                 `json:"tenant_id"`
	AgentVersionID     string                 `json:"agent_version_id"`
	Status             domain.ExecutionStatus `json:"status"`
	LeaseGeneration    int64                  `json:"lease_generation"`
	LeaseSoftExpiresAt time.Time              `json:"lease_soft_expires_at"`
	LeaseHardExpiresAt time.Time              `json:"lease_hard_expires_at"`
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

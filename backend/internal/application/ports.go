package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"github.com/shopspring/decimal"
)

type Store interface {
	WithTx(context.Context, func(Tx) error) error
}

type Tx interface {
	TaskRepository
	ExecutionRepository
	IdempotencyRepository
	EventRepository
	Now(context.Context) (time.Time, error)
}

type TaskRecord struct {
	ID                      string
	TenantID                string
	PublisherAgentVersionID string
	Type                    string
	Title                   string
	Problem                 string
	Constraints             []byte
	Requirements            []byte
	Deadline                time.Time
	Status                  domain.TaskStatus
	ClaimedBy               string
	StateVersion            int64
	ActiveExecutionID       string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type TaskRepository interface {
	InsertTask(context.Context, TaskRecord) error
	GetTask(context.Context, string, string) (*TaskRecord, error)
	ListTaskRecords(context.Context, TaskListQuery) ([]TaskRecord, error)
	UpdateTask(context.Context, TaskRecord, int64, string) (bool, error)
	ClaimTask(context.Context, string, string, int64, string) (bool, error)
}

type TaskListQuery struct {
	TenantID                string
	Statuses                []domain.TaskStatus
	Type                    string
	PublisherAgentVersionID string
	AfterCreatedAt          time.Time
	AfterID                 string
	Limit                   int
}

type ExecutionRepository interface {
	InsertExecution(context.Context, *domain.Execution, []byte) error
	GetExecution(context.Context, string, string) (*domain.Execution, int64, error)
	GetExecutionForUpdate(context.Context, string, string) (*domain.Execution, int64, error)
	ListActiveExecutions(context.Context, string, string) ([]ExecutionRecord, error)
	UpdateExecution(context.Context, *domain.Execution, int64) (bool, error)
	UpdateOwnedExecution(context.Context, *domain.Execution, int64, string, int64) (bool, error)
	GetExecutionUsage(context.Context, string, string) (*UsageView, error)
}

type ExecutionRecord struct {
	Execution    *domain.Execution
	StateVersion int64
}

type UsageView struct {
	ObservedCost     *decimal.Decimal
	SelfReportedCost *decimal.Decimal
	Coverage         string
	Provider         string
	ObservedAt       time.Time
}

type IdempotencyKey struct {
	TenantID  string
	ActorID   string
	Operation string
	RequestID string
}

type IdempotencyRecord struct {
	Key          IdempotencyKey
	RequestHash  [32]byte
	ResponseCode *int
	ResponseBody []byte
	ExpiresAt    time.Time
	OwnerToken   string
	Acquired     bool
	Completed    bool
}

type IdempotencyRepository interface {
	AcquireIdempotency(context.Context, IdempotencyKey, [32]byte, time.Time) (*IdempotencyRecord, error)
	CompleteIdempotency(context.Context, IdempotencyKey, string, int, []byte) error
}

type TaskEvent struct {
	TenantID    string
	TaskID      string
	ExecutionID string
	ActorType   string
	ActorID     string
	Intent      string
	FromState   string
	ToState     string
	Reason      string
	Payload     []byte
	CreatedAt   time.Time
}

type OutboxEvent struct {
	TenantID      string
	ID            string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       []byte
	AvailableAt   time.Time
}

type EventRepository interface {
	AppendTaskEvent(context.Context, TaskEvent) error
	AppendOutboxEvent(context.Context, OutboxEvent) error
	ListTaskEvents(context.Context, string, string, int64, int) ([]TaskEventSummary, error)
	GetLatestExecutionEvent(context.Context, string, string) (TaskEventSummary, error)
}

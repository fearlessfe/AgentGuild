package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
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

type TaskRepository interface {
	InsertTask(context.Context, *domain.Task) error
	GetTask(context.Context, string, string) (*domain.Task, int64, error)
	UpdateTask(context.Context, *domain.Task, int64, string, time.Time) (bool, error)
	ClaimTask(context.Context, string, string, int64, string, time.Time) (bool, error)
}

type ExecutionRepository interface {
	InsertExecution(context.Context, *domain.Execution, []byte) error
	GetExecution(context.Context, string, string) (*domain.Execution, int64, error)
	UpdateExecution(context.Context, *domain.Execution, int64, time.Time) (bool, error)
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
}

type IdempotencyRepository interface {
	LockIdempotency(context.Context, IdempotencyKey, [32]byte, time.Time) (*IdempotencyRecord, error)
	SaveIdempotencyResponse(context.Context, IdempotencyKey, int, []byte, time.Time) error
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
}

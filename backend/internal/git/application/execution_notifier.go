package application

import (
	"context"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
)

// ExecutionStateCommand describes a state transition requested by the git module.
type ExecutionStateCommand struct {
	TenantID    string
	ExecutionID string
	Intent      domain.Intent
	Actor       domain.Actor
}

// ExecutionNotifier notifies the task lifecycle module of state transitions
// that originate from git delivery and validation. The concrete implementation
// is supplied by the caller (normally the core application service).
type ExecutionNotifier interface {
	Notify(ctx context.Context, cmd ExecutionStateCommand, now time.Time) error
}

// NopExecutionNotifier is a no-op notifier for tests and local development.
type NopExecutionNotifier struct{}

// Notify does nothing and returns nil.
func (NopExecutionNotifier) Notify(context.Context, ExecutionStateCommand, time.Time) error { return nil }

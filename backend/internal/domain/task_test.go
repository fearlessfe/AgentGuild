package domain_test

import (
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestTaskRejectsProgressBeforeClaim(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	task := domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour))

	err := task.Apply(
		domain.IntentStart,
		domain.Actor{Type: domain.ActorAgent, ID: "agent-1"},
		now,
	)

	if !errors.Is(err, domain.ErrStateConflict) {
		t.Fatalf("Apply() error = %v, want %v", err, domain.ErrStateConflict)
	}
	if task.Status != domain.TaskOpen {
		t.Fatalf("task status = %q, want %q", task.Status, domain.TaskOpen)
	}
}

func TestTaskLegalTransitions(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	publisher := domain.Actor{Type: domain.ActorPublisher, ID: "publisher-1"}
	agent := domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}

	tests := []struct {
		name       string
		task       *domain.Task
		intent     domain.Intent
		actor      domain.Actor
		wantStatus domain.TaskStatus
	}{
		{
			name:       "publisher publishes draft",
			task:       domain.NewDraftTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour)),
			intent:     domain.IntentPublish,
			actor:      publisher,
			wantStatus: domain.TaskOpen,
		},
		{
			name:       "agent claims open task",
			task:       domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour)),
			intent:     domain.IntentClaim,
			actor:      agent,
			wantStatus: domain.TaskClaimed,
		},
		{
			name: "claiming agent starts task",
			task: func() *domain.Task {
				task := domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
				if err := task.Apply(domain.IntentClaim, agent, now); err != nil {
					t.Fatalf("claim fixture: %v", err)
				}
				return task
			}(),
			intent:     domain.IntentStart,
			actor:      agent,
			wantStatus: domain.TaskInProgress,
		},
		{
			name:       "publisher cancels open task",
			task:       domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour)),
			intent:     domain.IntentCancel,
			actor:      publisher,
			wantStatus: domain.TaskCancelled,
		},
		{
			name:       "system expires open task after deadline",
			task:       domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(-time.Second)),
			intent:     domain.IntentExpire,
			actor:      domain.SystemActor(),
			wantStatus: domain.TaskExpired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.task.Apply(tt.intent, tt.actor, now); err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if tt.task.Status != tt.wantStatus {
				t.Fatalf("task status = %q, want %q", tt.task.Status, tt.wantStatus)
			}
		})
	}
}

func TestTaskRejectsUnauthorizedActorWithoutMutation(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	task := domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour))

	err := task.Apply(
		domain.IntentCancel,
		domain.Actor{Type: domain.ActorPublisher, ID: "publisher-2"},
		now,
	)

	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Apply() error = %v, want %v", err, domain.ErrForbidden)
	}
	if task.Status != domain.TaskOpen {
		t.Fatalf("task status = %q, want %q", task.Status, domain.TaskOpen)
	}
}

func TestTaskCompletesOnlyForClaimingAgent(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	agent := domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}
	task := domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
	if err := task.Apply(domain.IntentClaim, agent, now); err != nil {
		t.Fatalf("claim fixture: %v", err)
	}
	if err := task.Apply(domain.IntentStart, agent, now); err != nil {
		t.Fatalf("start fixture: %v", err)
	}

	err := task.Apply(domain.IntentComplete, agent, now)

	if err != nil {
		t.Fatalf("Apply(complete) error = %v", err)
	}
	if task.Status != domain.TaskCompleted {
		t.Fatalf("task status = %q, want %q", task.Status, domain.TaskCompleted)
	}
}

func TestTaskTerminalStatesNeverTransition(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	publisher := domain.Actor{Type: domain.ActorPublisher, ID: "publisher-1"}

	for _, status := range []domain.TaskStatus{
		domain.TaskCompleted,
		domain.TaskCancelled,
		domain.TaskExpired,
	} {
		t.Run(string(status), func(t *testing.T) {
			task := domain.NewTask("task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
			task.Status = status

			err := task.Apply(domain.IntentCancel, publisher, now)

			if !errors.Is(err, domain.ErrStateConflict) {
				t.Fatalf("Apply() error = %v, want %v", err, domain.ErrStateConflict)
			}
			if task.Status != status {
				t.Fatalf("task status = %q, want unchanged %q", task.Status, status)
			}
		})
	}
}

func TestDomainErrorCarriesStableDetails(t *testing.T) {
	var err error = domain.Error{
		Code:    "invalid_argument",
		Message: "deadline is required",
		Field:   "deadline",
	}

	if err.Error() != "deadline is required" {
		t.Fatalf("Error() = %q", err.Error())
	}
	if !errors.Is(err, domain.Error{Code: "invalid_argument"}) {
		t.Fatal("errors.Is() should match errors with the same code")
	}
}

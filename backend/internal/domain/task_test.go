package domain_test

import (
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestTaskRejectsProgressBeforeClaim(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour))

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

func TestTaskConstructorRejectsMissingDeadline(t *testing.T) {
	task, err := domain.NewTask("task-1", "tenant-1", "publisher-1", time.Time{})

	if task != nil {
		t.Fatalf("NewTask() = %+v, want nil for missing deadline", task)
	}
	assertInvalidArgument(t, err, "deadline")
}

func TestTaskConstructorRejectsEmptyIdentity(t *testing.T) {
	deadline := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		id          string
		tenantID    string
		publisherID string
		field       string
	}{
		{name: "empty task ID", tenantID: "tenant-1", publisherID: "publisher-1", field: "id"},
		{name: "empty tenant ID", id: "task-1", publisherID: "publisher-1", field: "tenant_id"},
		{name: "empty publisher ID", id: "task-1", tenantID: "tenant-1", field: "publisher_id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := domain.NewTask(tt.id, tt.tenantID, tt.publisherID, deadline)
			if task != nil {
				t.Fatalf("NewTask() = %+v, want nil", task)
			}
			assertInvalidArgument(t, err, tt.field)
		})
	}
}

func TestTaskRejectsClaimAtOrAfterDeadlineWithoutMutation(t *testing.T) {
	deadline := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	agent := domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}

	for _, claimAt := range []time.Time{deadline, deadline.Add(time.Nanosecond)} {
		t.Run(claimAt.Sub(deadline).String(), func(t *testing.T) {
			task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", deadline)

			err := task.Apply(domain.IntentClaim, agent, claimAt)

			if !errors.Is(err, domain.ErrStateConflict) {
				t.Fatalf("Apply(claim) error = %v, want %v", err, domain.ErrStateConflict)
			}
			if task.Status != domain.TaskOpen || task.ClaimedBy != "" {
				t.Fatalf("task mutated after rejected claim: %+v", task)
			}
		})
	}
}

func TestTaskExpiresAtDeadline(t *testing.T) {
	deadline := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", deadline)

	if err := task.Apply(domain.IntentExpire, domain.SystemActor(), deadline); err != nil {
		t.Fatalf("Apply(expire at deadline) error = %v", err)
	}
	if task.Status != domain.TaskExpired {
		t.Fatalf("status = %q, want %q", task.Status, domain.TaskExpired)
	}
}

func TestTaskRejectsProgressAtDeadlineWithoutMutation(t *testing.T) {
	deadline := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	beforeDeadline := deadline.Add(-time.Minute)
	publisher := domain.Actor{Type: domain.ActorPublisher, ID: "publisher-1"}
	agent := domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}

	tests := []struct {
		name   string
		task   *domain.Task
		intent domain.Intent
		actor  domain.Actor
	}{
		{
			name:   "publish",
			task:   domain.NewDraftTask("task-1", "tenant-1", "publisher-1", deadline),
			intent: domain.IntentPublish,
			actor:  publisher,
		},
		{
			name: "start",
			task: func() *domain.Task {
				task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", deadline)
				if err := task.Apply(domain.IntentClaim, agent, beforeDeadline); err != nil {
					t.Fatalf("claim fixture: %v", err)
				}
				return task
			}(),
			intent: domain.IntentStart,
			actor:  agent,
		},
		{
			name: "complete",
			task: func() *domain.Task {
				task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", deadline)
				if err := task.Apply(domain.IntentClaim, agent, beforeDeadline); err != nil {
					t.Fatalf("claim fixture: %v", err)
				}
				if err := task.Apply(domain.IntentStart, agent, beforeDeadline); err != nil {
					t.Fatalf("start fixture: %v", err)
				}
				return task
			}(),
			intent: domain.IntentComplete,
			actor:  agent,
		},
		{
			name:   "cancel",
			task:   mustNewTask(t, "task-1", "tenant-1", "publisher-1", deadline),
			intent: domain.IntentCancel,
			actor:  publisher,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := *tt.task

			err := tt.task.Apply(tt.intent, tt.actor, deadline)

			if !errors.Is(err, domain.ErrStateConflict) {
				t.Fatalf("Apply() error = %v, want %v", err, domain.ErrStateConflict)
			}
			if *tt.task != before {
				t.Fatalf("task mutated: got %+v, want %+v", *tt.task, before)
			}
		})
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
			task:       mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour)),
			intent:     domain.IntentClaim,
			actor:      agent,
			wantStatus: domain.TaskClaimed,
		},
		{
			name: "claiming agent starts task",
			task: func() *domain.Task {
				task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
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
			task:       mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour)),
			intent:     domain.IntentCancel,
			actor:      publisher,
			wantStatus: domain.TaskCancelled,
		},
		{
			name:       "system expires open task after deadline",
			task:       mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(-time.Second)),
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
	task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour))

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
	task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
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
			task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
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

func TestTaskIllegalTransitionMatrixDoesNotMutate(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	publisher := domain.Actor{Type: domain.ActorPublisher, ID: "publisher-1"}
	agent := domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}

	tests := []struct {
		name   string
		status domain.TaskStatus
		intent domain.Intent
		actor  domain.Actor
	}{
		{name: "draft cannot be claimed", status: domain.TaskDraft, intent: domain.IntentClaim, actor: agent},
		{name: "open cannot be completed", status: domain.TaskOpen, intent: domain.IntentComplete, actor: agent},
		{name: "claimed cannot be published", status: domain.TaskClaimed, intent: domain.IntentPublish, actor: publisher},
		{name: "in progress cannot be claimed", status: domain.TaskInProgress, intent: domain.IntentClaim, actor: agent},
		{name: "completed cannot be cancelled", status: domain.TaskCompleted, intent: domain.IntentCancel, actor: publisher},
		{name: "cancelled cannot be published", status: domain.TaskCancelled, intent: domain.IntentPublish, actor: publisher},
		{name: "expired cannot be started", status: domain.TaskExpired, intent: domain.IntentStart, actor: agent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
			task.Status = tt.status
			task.ClaimedBy = "agent-1"
			before := *task

			err := task.Apply(tt.intent, tt.actor, now)

			if !errors.Is(err, domain.ErrStateConflict) {
				t.Fatalf("Apply() error = %v, want %v", err, domain.ErrStateConflict)
			}
			if *task != before {
				t.Fatalf("task mutated: got %+v, want %+v", *task, before)
			}
		})
	}
}

func TestTaskExplicitIntentMatrix(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	deadline := now.Add(time.Hour)
	publisher := domain.Actor{Type: domain.ActorPublisher, ID: "publisher-1"}
	agent := domain.Actor{Type: domain.ActorAgent, ID: "agent-1"}

	operations := []struct {
		name    string
		intent  domain.Intent
		actor   domain.Actor
		at      time.Time
		allowed map[domain.TaskStatus]domain.TaskStatus
	}{
		{
			name:   "publish",
			intent: domain.IntentPublish,
			actor:  publisher,
			at:     now,
			allowed: map[domain.TaskStatus]domain.TaskStatus{
				domain.TaskDraft: domain.TaskOpen,
			},
		},
		{
			name:   "claim",
			intent: domain.IntentClaim,
			actor:  agent,
			at:     now,
			allowed: map[domain.TaskStatus]domain.TaskStatus{
				domain.TaskOpen: domain.TaskClaimed,
			},
		},
		{
			name:   "cancel",
			intent: domain.IntentCancel,
			actor:  publisher,
			at:     now,
			allowed: map[domain.TaskStatus]domain.TaskStatus{
				domain.TaskOpen:       domain.TaskCancelled,
				domain.TaskClaimed:    domain.TaskCancelled,
				domain.TaskInProgress: domain.TaskCancelled,
			},
		},
		{
			name:   "start",
			intent: domain.IntentStart,
			actor:  agent,
			at:     now,
			allowed: map[domain.TaskStatus]domain.TaskStatus{
				domain.TaskClaimed: domain.TaskInProgress,
			},
		},
		{
			name:   "complete",
			intent: domain.IntentComplete,
			actor:  agent,
			at:     now,
			allowed: map[domain.TaskStatus]domain.TaskStatus{
				domain.TaskInProgress: domain.TaskCompleted,
			},
		},
		{
			name:   "expire",
			intent: domain.IntentExpire,
			actor:  domain.SystemActor(),
			at:     deadline,
			allowed: map[domain.TaskStatus]domain.TaskStatus{
				domain.TaskOpen:       domain.TaskExpired,
				domain.TaskClaimed:    domain.TaskExpired,
				domain.TaskInProgress: domain.TaskExpired,
			},
		},
	}
	statuses := []domain.TaskStatus{
		domain.TaskDraft,
		domain.TaskOpen,
		domain.TaskClaimed,
		domain.TaskInProgress,
		domain.TaskCompleted,
		domain.TaskCancelled,
		domain.TaskExpired,
	}

	for _, operation := range operations {
		for _, status := range statuses {
			t.Run(operation.name+"/"+string(status), func(t *testing.T) {
				task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", deadline)
				task.Status = status
				task.ClaimedBy = "agent-1"
				before := *task

				err := task.Apply(operation.intent, operation.actor, operation.at)
				wantStatus, allowed := operation.allowed[status]
				if !allowed {
					if !errors.Is(err, domain.ErrStateConflict) {
						t.Fatalf("Apply() error = %v, want %v", err, domain.ErrStateConflict)
					}
					if *task != before {
						t.Fatalf("task mutated: got %+v, want %+v", *task, before)
					}
					return
				}
				if err != nil {
					t.Fatalf("Apply() error = %v", err)
				}
				if task.Status != wantStatus {
					t.Fatalf("status = %q, want %q", task.Status, wantStatus)
				}
			})
		}
	}
}

func TestTaskRejectsEmptyActorIdentityWithoutMutation(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	task := mustNewTask(t, "task-1", "tenant-1", "publisher-1", now.Add(time.Hour))
	before := *task

	err := task.Apply(domain.IntentClaim, domain.Actor{Type: domain.ActorAgent}, now)

	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Apply() error = %v, want %v", err, domain.ErrForbidden)
	}
	if *task != before {
		t.Fatalf("task mutated: got %+v, want %+v", *task, before)
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

func assertInvalidArgument(t *testing.T, err error, field string) {
	t.Helper()
	if !errors.Is(err, domain.Error{Code: "invalid_argument"}) {
		t.Fatalf("error = %v, want invalid_argument", err)
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error type = %T, want *domain.Error", err)
	}
	if domainErr.Field != field {
		t.Fatalf("error field = %q, want %q", domainErr.Field, field)
	}
}

func mustNewTask(t *testing.T, id, tenantID, publisherID string, deadline time.Time) *domain.Task {
	t.Helper()
	task, err := domain.NewTask(id, tenantID, publisherID, deadline)
	if err != nil {
		t.Fatalf("NewTask() error = %v", err)
	}
	return task
}

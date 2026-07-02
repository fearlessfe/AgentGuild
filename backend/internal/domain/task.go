package domain

import "time"

type TaskStatus string

const (
	TaskDraft      TaskStatus = "draft"
	TaskOpen       TaskStatus = "open"
	TaskClaimed    TaskStatus = "claimed"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
	TaskCancelled  TaskStatus = "cancelled"
	TaskExpired    TaskStatus = "expired"
)

type Intent uint8

const (
	IntentPublish Intent = iota
	IntentClaim
	IntentCancel
	IntentStart
	IntentComplete
	IntentHeartbeat
	IntentExpire
	IntentAccept
)

type ActorType string

const (
	ActorPublisher ActorType = "publisher"
	ActorAgent     ActorType = "agent"
	ActorSystem    ActorType = "system"
)

type Actor struct {
	Type ActorType
	ID   string
}

type Task struct {
	ID          string
	TenantID    string
	PublisherID string
	Deadline    time.Time
	Status      TaskStatus
	ClaimedBy   string
}

func NewTask(id, tenantID, publisherID string, deadline time.Time) *Task {
	return &Task{
		ID:          id,
		TenantID:    tenantID,
		PublisherID: publisherID,
		Deadline:    deadline,
		Status:      TaskOpen,
	}
}

func NewDraftTask(id, tenantID, publisherID string, deadline time.Time) *Task {
	task := NewTask(id, tenantID, publisherID, deadline)
	task.Status = TaskDraft
	return task
}

func SystemActor() Actor {
	return Actor{Type: ActorSystem, ID: "system"}
}

func (t *Task) Apply(intent Intent, actor Actor, now time.Time) error {
	switch intent {
	case IntentPublish:
		if t.Status != TaskDraft {
			return ErrStateConflict
		}
		if actor.Type != ActorPublisher || actor.ID != t.PublisherID {
			return ErrForbidden
		}
		t.Status = TaskOpen
		return nil
	case IntentClaim:
		if t.Status != TaskOpen {
			return ErrStateConflict
		}
		if actor.Type != ActorAgent {
			return ErrForbidden
		}
		t.ClaimedBy = actor.ID
		t.Status = TaskClaimed
		return nil
	case IntentStart:
		if t.Status != TaskClaimed {
			return ErrStateConflict
		}
		if actor.Type != ActorAgent || actor.ID != t.ClaimedBy {
			return ErrForbidden
		}
		t.Status = TaskInProgress
		return nil
	case IntentCancel:
		if t.Status != TaskOpen && t.Status != TaskClaimed && t.Status != TaskInProgress {
			return ErrStateConflict
		}
		if actor.Type != ActorPublisher || actor.ID != t.PublisherID {
			return ErrForbidden
		}
		t.Status = TaskCancelled
		return nil
	case IntentComplete:
		if t.Status != TaskInProgress {
			return ErrStateConflict
		}
		if actor.Type != ActorAgent || actor.ID != t.ClaimedBy {
			return ErrForbidden
		}
		t.Status = TaskCompleted
		return nil
	case IntentExpire:
		if t.Status != TaskOpen && t.Status != TaskClaimed && t.Status != TaskInProgress {
			return ErrStateConflict
		}
		if actor.Type != ActorSystem {
			return ErrForbidden
		}
		if !now.After(t.Deadline) {
			return ErrStateConflict
		}
		t.Status = TaskExpired
		return nil
	default:
		return ErrStateConflict
	}
}

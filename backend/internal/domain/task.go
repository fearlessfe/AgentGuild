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

const SystemIssuePublisherID = "system-issue-sync"

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
	IntentReject
	IntentRequestRevision
	IntentSubmitForReview
	IntentSubmit
	IntentStartValidation
	IntentFailValidation
	IntentMarkReviewing
)

type ActorType string

const (
	ActorPublisher ActorType = "publisher"
	ActorAgent     ActorType = "agent"
	ActorSystem    ActorType = "system"
	ActorReviewer  ActorType = "reviewer"
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

func NewTask(id, tenantID, publisherID string, deadline time.Time) (*Task, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if publisherID == "" {
		return nil, invalidArgument("publisher_id")
	}
	if deadline.IsZero() {
		return nil, invalidArgument("deadline")
	}
	return &Task{
		ID:          id,
		TenantID:    tenantID,
		PublisherID: publisherID,
		Deadline:    deadline,
		Status:      TaskOpen,
	}, nil
}

func NewDraftTask(id, tenantID, publisherID string, deadline time.Time) (*Task, error) {
	task, err := NewTask(id, tenantID, publisherID, deadline)
	if err != nil {
		return nil, err
	}
	task.Status = TaskDraft
	return task, nil
}

func SystemActor() Actor {
	return Actor{Type: ActorSystem, ID: "system"}
}

func (t *Task) Apply(intent Intent, actor Actor, now time.Time) error {
	if actor.ID == "" {
		return ErrForbidden
	}
	switch intent {
	case IntentPublish, IntentClaim, IntentStart, IntentCancel:
		if !now.Before(t.Deadline) {
			return ErrStateConflict
		}
	}
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
		if (actor.Type != ActorAgent || actor.ID != t.ClaimedBy) && actor.Type != ActorReviewer {
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
		if now.Before(t.Deadline) {
			return ErrStateConflict
		}
		t.Status = TaskExpired
		return nil
	default:
		return ErrStateConflict
	}
}

package application

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

const idempotencyTTL = 24 * time.Hour

func (s *Service) PublishTask(ctx context.Context, principal auth.Principal, command PublishTask) (Envelope[TaskView], error) {
	var result Envelope[TaskView]
	if err := s.policy.Require(principal, "tasks:publish"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		if err := s.requireLiveAgent(ctx, tx, principal); err != nil {
			return err
		}
		if err := s.checkRateLimit(ctx, principal); err != nil {
			return err
		}
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		key, record, err := acquire(ctx, tx, principal, "task_publish", command.RequestID, command, now)
		if err != nil {
			return err
		}
		if record.Completed {
			return json.Unmarshal(record.ResponseBody, &result)
		}
		if !record.Acquired {
			return conflict("idempotency request is already in progress")
		}
		if !now.Before(command.Deadline) {
			return invalid("deadline")
		}

		task, err := domain.NewTask(s.newID(), principal.TenantID, principal.AgentVersionID, command.Deadline)
		if err != nil {
			return err
		}
		constraints, err := json.Marshal(command.Constraints)
		if err != nil {
			return err
		}
		requirements, err := json.Marshal(command.Requirements)
		if err != nil {
			return err
		}
		recordTask := TaskRecord{ID: task.ID, TenantID: task.TenantID, PublisherAgentVersionID: task.PublisherID, Type: command.Type, Title: command.Title, Problem: command.Problem, Constraints: constraints, Requirements: requirements, Deadline: task.Deadline, Status: task.Status, CreatedAt: now, UpdatedAt: now}
		if err := tx.InsertTask(ctx, recordTask); err != nil {
			return err
		}
		view, err := taskView(recordTask)
		if err != nil {
			return err
		}
		result = Envelope[TaskView]{Data: view, Meta: Meta{ServerTime: now, ResourceVersion: recordTask.StateVersion}}
		if err := appendEvents(ctx, tx, principal, recordTask, "publish", "draft", string(task.Status), ""); err != nil {
			return err
		}
		return complete(ctx, tx, key, record.OwnerToken, result)
	})
	return result, err
}

func (s *Service) CancelTask(ctx context.Context, principal auth.Principal, command CancelTask) (Envelope[TaskView], error) {
	var result Envelope[TaskView]
	if err := s.policy.Require(principal, "tasks:cancel"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		if err := s.requireLiveAgent(ctx, tx, principal); err != nil {
			return err
		}
		if err := s.checkRateLimit(ctx, principal); err != nil {
			return err
		}
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		key, idem, err := acquire(ctx, tx, principal, "task_cancel", command.RequestID, command, now)
		if err != nil {
			return err
		}
		if idem.Completed {
			return json.Unmarshal(idem.ResponseBody, &result)
		}
		if !idem.Acquired {
			return conflict("idempotency request is already in progress")
		}
		record, err := tx.GetTask(ctx, principal.TenantID, command.TaskID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return notFound()
			}
			return err
		}
		if record.PublisherAgentVersionID != principal.AgentVersionID {
			return notFound()
		}
		task := &domain.Task{ID: record.ID, TenantID: record.TenantID, PublisherID: record.PublisherAgentVersionID, Deadline: record.Deadline, Status: record.Status, ClaimedBy: record.ClaimedBy}
		from := task.Status
		actor := domain.Actor{Type: domain.ActorPublisher, ID: principal.AgentVersionID}
		if err := task.Apply(domain.IntentCancel, actor, now); err != nil {
			return err
		}
		executions, err := tx.ListActiveExecutions(ctx, principal.TenantID, record.ID)
		if err != nil {
			return err
		}
		for _, active := range executions {
			if err := active.Execution.Cancel(actor, now); err != nil {
				return err
			}
			updated, err := tx.UpdateExecution(ctx, active.Execution, active.StateVersion)
			if err != nil {
				return err
			}
			if !updated {
				return conflict("execution changed concurrently")
			}
		}
		record.Status = task.Status
		updated, err := tx.UpdateTask(ctx, *record, record.StateVersion, "")
		if err != nil {
			return err
		}
		if !updated {
			return conflict("task changed concurrently")
		}
		record.StateVersion++
		record.ActiveExecutionID = ""
		record.UpdatedAt = now
		view, err := taskView(*record)
		if err != nil {
			return err
		}
		result = Envelope[TaskView]{Data: view, Meta: Meta{ServerTime: now, ResourceVersion: record.StateVersion}}
		if err := appendEvents(ctx, tx, principal, *record, "cancel", string(from), string(task.Status), command.Reason); err != nil {
			return err
		}
		return complete(ctx, tx, key, idem.OwnerToken, result)
	})
	return result, err
}

func acquire(ctx context.Context, tx Tx, principal auth.Principal, operation, requestID string, request any, now time.Time) (IdempotencyKey, *IdempotencyRecord, error) {
	if requestID == "" {
		return IdempotencyKey{}, nil, invalid("request_id")
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return IdempotencyKey{}, nil, err
	}
	key := IdempotencyKey{TenantID: principal.TenantID, ActorID: principal.AgentVersionID, Operation: operation, RequestID: requestID}
	record, err := tx.AcquireIdempotency(ctx, key, sha256.Sum256(payload), now.Add(idempotencyTTL))
	return key, record, err
}

func complete[T any](ctx context.Context, tx Tx, key IdempotencyKey, owner string, response Envelope[T]) error {
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return tx.CompleteIdempotency(ctx, key, owner, 200, body)
}

func appendEvents(ctx context.Context, tx Tx, principal auth.Principal, task TaskRecord, intent, from, to, reason string) error {
	payload, err := json.Marshal(map[string]string{"task_id": task.ID})
	if err != nil {
		return err
	}
	if err := tx.AppendTaskEvent(ctx, TaskEvent{TenantID: task.TenantID, TaskID: task.ID, ActorType: string(domain.ActorPublisher), ActorID: principal.AgentVersionID, Intent: intent, FromState: from, ToState: to, Reason: reason, Payload: payload, CreatedAt: task.UpdatedAt}); err != nil {
		return err
	}
	eventType := "task." + intent + "ed"
	if intent == "cancel" {
		eventType = "task.cancelled"
	}
	return tx.AppendOutboxEvent(ctx, OutboxEvent{TenantID: task.TenantID, ID: task.ID + ":" + intent + ":" + randomID(), EventType: eventType, AggregateType: "task", AggregateID: task.ID, Payload: payload, AvailableAt: task.UpdatedAt})
}

func invalid(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}
func conflict(message string) error { return &domain.Error{Code: "state_conflict", Message: message} }
func notFound() error               { return &domain.Error{Code: "not_found", Message: "resource not found"} }

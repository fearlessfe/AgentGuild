package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

func (s *Service) ClaimTask(ctx context.Context, principal auth.Principal, command ClaimTask) (Envelope[ExecutionView], error) {
	var result Envelope[ExecutionView]
	if err := s.policy.Require(principal, "tasks:claim"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		key, idem, err := acquire(ctx, tx, principal, "task_claim", command.RequestID, command, now)
		if err != nil {
			return err
		}
		if idem.Completed {
			return json.Unmarshal(idem.ResponseBody, &result)
		}
		if !idem.Acquired {
			return conflict("idempotency request is already in progress")
		}
		taskRecord, err := tx.GetTask(ctx, principal.TenantID, command.TaskID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return notFound()
			}
			return err
		}
		task := &domain.Task{ID: taskRecord.ID, TenantID: taskRecord.TenantID, PublisherID: taskRecord.PublisherAgentVersionID, Deadline: taskRecord.Deadline, Status: taskRecord.Status, ClaimedBy: taskRecord.ClaimedBy}
		if err := task.Apply(domain.IntentClaim, domain.Actor{Type: domain.ActorAgent, ID: principal.AgentVersionID}, now); err != nil {
			return err
		}
		execution, err := domain.NewLeasedExecution(s.newID(), task.ID, task.TenantID, principal.AgentVersionID, now, 1)
		if err != nil {
			return err
		}
		claimed, err := tx.ClaimTask(ctx, task.TenantID, task.ID, taskRecord.StateVersion, execution.ID)
		if err != nil {
			return err
		}
		if !claimed {
			return conflict("task changed concurrently")
		}
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return err
		}
		hash := sha256.Sum256(secret)
		if err := tx.InsertExecution(ctx, execution, hash[:]); err != nil {
			return err
		}
		result = executionEnvelope(execution, now, 0)
		if err := appendExecutionEvents(ctx, tx, principal, execution, "claim", string(taskRecord.Status), string(task.Status), now); err != nil {
			return err
		}
		return complete(ctx, tx, key, idem.OwnerToken, result)
	})
	return result, err
}

func (s *Service) StartExecution(ctx context.Context, principal auth.Principal, command StartExecution) (Envelope[ExecutionView], error) {
	return s.mutateExecution(ctx, principal, "execution_start", command.RequestID, command.ExecutionID, command.LeaseGeneration, command, "start", func(execution *domain.Execution, now time.Time, generation int64) error {
		return execution.Start(now, generation)
	})
}

func (s *Service) HeartbeatExecution(ctx context.Context, principal auth.Principal, command HeartbeatExecution) (Envelope[ExecutionView], error) {
	return s.mutateExecution(ctx, principal, "execution_heartbeat", command.RequestID, command.ExecutionID, command.LeaseGeneration, command, "heartbeat", func(execution *domain.Execution, now time.Time, generation int64) error {
		_, err := execution.Heartbeat(now, generation)
		return err
	})
}

type executionMutation func(*domain.Execution, time.Time, int64) error

func (s *Service) mutateExecution(ctx context.Context, principal auth.Principal, operation, requestID, executionID string, generation int64, request any, intent string, mutate executionMutation) (Envelope[ExecutionView], error) {
	var result Envelope[ExecutionView]
	if err := s.policy.Require(principal, "tasks:execute"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		key, idem, err := acquire(ctx, tx, principal, operation, requestID, request, now)
		if err != nil {
			return err
		}
		if idem.Completed {
			return json.Unmarshal(idem.ResponseBody, &result)
		}
		if !idem.Acquired {
			return conflict("idempotency request is already in progress")
		}
		execution, version, err := tx.GetExecutionForUpdate(ctx, principal.TenantID, executionID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return notFound()
			}
			return err
		}
		if execution.AgentID != principal.AgentVersionID {
			return notFound()
		}
		taskRecord, err := tx.GetTask(ctx, principal.TenantID, execution.TaskID)
		if err != nil {
			return err
		}
		if !now.Before(taskRecord.Deadline) {
			return domain.ErrLeaseExpired
		}
		from := execution.Status
		if err := mutate(execution, now, generation); err != nil {
			return err
		}
		var startedTask *TaskRecord
		if intent == "start" {
			task := &domain.Task{ID: taskRecord.ID, TenantID: taskRecord.TenantID, PublisherID: taskRecord.PublisherAgentVersionID, Deadline: taskRecord.Deadline, Status: taskRecord.Status, ClaimedBy: taskRecord.ClaimedBy}
			if err := task.Apply(domain.IntentStart, domain.Actor{Type: domain.ActorAgent, ID: principal.AgentVersionID}, now); err != nil {
				return err
			}
			taskRecord.Status = task.Status
			startedTask = taskRecord
		}
		updated, err := tx.UpdateOwnedExecution(ctx, execution, version, principal.AgentVersionID, generation)
		if err != nil {
			return err
		}
		if !updated {
			return domain.ErrLeaseExpired
		}
		if startedTask != nil {
			updated, err := tx.UpdateTask(ctx, *startedTask, startedTask.StateVersion, execution.ID)
			if err != nil {
				return err
			}
			if !updated {
				return conflict("task changed concurrently")
			}
		}
		result = executionEnvelope(execution, now, version+1)
		if err := appendExecutionEvents(ctx, tx, principal, execution, intent, string(from), string(execution.Status), now); err != nil {
			return err
		}
		return complete(ctx, tx, key, idem.OwnerToken, result)
	})
	return result, err
}

func (s *Service) GetExecution(ctx context.Context, principal auth.Principal, query GetExecution) (Envelope[ExecutionView], error) {
	var result Envelope[ExecutionView]
	if err := s.policy.Require(principal, "tasks:execute"); err != nil {
		return result, err
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		execution, version, err := tx.GetExecution(ctx, principal.TenantID, query.ExecutionID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return notFound()
			}
			return err
		}
		if execution.AgentID != principal.AgentVersionID {
			return notFound()
		}
		result = executionEnvelope(execution, now, version)
		return nil
	})
	return result, err
}

func executionEnvelope(execution *domain.Execution, now time.Time, version int64) Envelope[ExecutionView] {
	return Envelope[ExecutionView]{Data: ExecutionView{ID: execution.ID, TaskID: execution.TaskID, TenantID: execution.TenantID, AgentVersionID: execution.AgentID, Status: execution.Status, LeaseGeneration: execution.Lease.Generation, LeaseSoftExpiresAt: execution.Lease.SoftExpiry, LeaseHardExpiresAt: execution.Lease.HardExpiry}, Meta: Meta{ServerTime: now, ResourceVersion: version}}
}

func appendExecutionEvents(ctx context.Context, tx Tx, principal auth.Principal, execution *domain.Execution, intent, from, to string, now time.Time) error {
	payload, err := json.Marshal(map[string]string{"task_id": execution.TaskID, "execution_id": execution.ID})
	if err != nil {
		return err
	}
	if err := tx.AppendTaskEvent(ctx, TaskEvent{TenantID: execution.TenantID, TaskID: execution.TaskID, ExecutionID: execution.ID, ActorType: string(domain.ActorAgent), ActorID: principal.AgentVersionID, Intent: intent, FromState: from, ToState: to, Payload: payload, CreatedAt: now}); err != nil {
		return err
	}
	return tx.AppendOutboxEvent(ctx, OutboxEvent{TenantID: execution.TenantID, ID: execution.ID + ":" + intent + ":" + randomID(), EventType: "execution." + intent + "ed", AggregateType: "execution", AggregateID: execution.ID, Payload: payload, AvailableAt: now})
}

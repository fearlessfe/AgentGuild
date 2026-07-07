package application

import (
	"context"
	"encoding/json"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

type PublishSystemTask struct {
	TenantID     string
	RequestID    string
	Type         string
	Title        string
	Problem      string
	Constraints  []string
	Requirements []string
	Deadline     time.Time
}

func (s *Service) PublishSystemTask(ctx context.Context, command PublishSystemTask) (TaskView, error) {
	var result TaskView
	principal := systemTaskPrincipal(command.TenantID)
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		key, record, err := acquire(ctx, tx, principal, "task_publish", command.RequestID, command, now)
		if err != nil {
			return err
		}
		if record.Completed {
			var envelope Envelope[TaskView]
			if err := json.Unmarshal(record.ResponseBody, &envelope); err != nil {
				return err
			}
			result = envelope.Data
			return nil
		}
		if !record.Acquired {
			return conflict("idempotency request is already in progress")
		}
		if !now.Before(command.Deadline) {
			return invalid("deadline")
		}

		task, err := domain.NewTask(s.newID(), command.TenantID, domain.SystemIssuePublisherID, command.Deadline)
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
		recordTask := TaskRecord{
			ID:                      task.ID,
			TenantID:                task.TenantID,
			PublisherAgentVersionID: domain.SystemIssuePublisherID,
			Type:                    command.Type,
			Title:                   command.Title,
			Problem:                 command.Problem,
			Constraints:             constraints,
			Requirements:            requirements,
			Deadline:                task.Deadline,
			Status:                  task.Status,
			CreatedAt:               now,
			UpdatedAt:               now,
		}
		if err := tx.InsertTask(ctx, recordTask); err != nil {
			return err
		}
		view, err := taskView(recordTask)
		if err != nil {
			return err
		}
		result = view
		envelope := Envelope[TaskView]{Data: view, Meta: Meta{ServerTime: now, ResourceVersion: recordTask.StateVersion}}
		if err := appendEvents(ctx, tx, principal, recordTask, "publish", "draft", string(task.Status), ""); err != nil {
			return err
		}
		return complete(ctx, tx, key, record.OwnerToken, envelope)
	})
	return result, err
}

func (s *Service) CancelSystemTask(ctx context.Context, tenantID, taskID, reason string) (TaskView, error) {
	var result TaskView
	principal := systemTaskPrincipal(tenantID)
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		record, err := tx.GetTask(ctx, tenantID, taskID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return notFound()
			}
			return err
		}
		task := &domain.Task{ID: record.ID, TenantID: record.TenantID, PublisherID: record.PublisherAgentVersionID, Deadline: record.Deadline, Status: record.Status, ClaimedBy: record.ClaimedBy}
		from := task.Status
		actor := domain.Actor{Type: domain.ActorPublisher, ID: domain.SystemIssuePublisherID}
		if err := task.Apply(domain.IntentCancel, actor, now); err != nil {
			return err
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
		result = view
		return appendEvents(ctx, tx, principal, *record, "cancel", string(from), string(task.Status), reason)
	})
	return result, err
}

func (s *Service) UpdateSystemTaskContent(ctx context.Context, tenantID, taskID string, title, problem string, constraints, requirements []string) error {
	return s.store.WithTx(ctx, func(tx Tx) error {
		record, err := tx.GetTask(ctx, tenantID, taskID)
		if err != nil {
			if domain.CodeOf(err) == "not_found" {
				return notFound()
			}
			return err
		}
		if record.Status != domain.TaskOpen && record.Status != domain.TaskDraft {
			return nil
		}
		marshaledConstraints, err := json.Marshal(constraints)
		if err != nil {
			return err
		}
		marshaledRequirements, err := json.Marshal(requirements)
		if err != nil {
			return err
		}
		record.Title = title
		record.Problem = problem
		record.Constraints = marshaledConstraints
		record.Requirements = marshaledRequirements
		updated, err := tx.UpdateTask(ctx, *record, record.StateVersion, "")
		if err != nil {
			return err
		}
		if !updated {
			return conflict("task changed concurrently")
		}
		return nil
	})
}

func systemTaskPrincipal(tenantID string) auth.Principal {
	return auth.Principal{TenantID: tenantID, AgentVersionID: domain.SystemIssuePublisherID}
}

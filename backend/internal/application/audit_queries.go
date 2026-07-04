package application

import (
	"context"
	"strconv"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

// TaskEventSummary 是审计事件的脱敏摘要，不包含原始 payload。
type TaskEventSummary struct {
	ID          int64     `json:"id"`
	TenantID    string    `json:"tenant_id"`
	TaskID      string    `json:"task_id"`
	ExecutionID string    `json:"execution_id,omitempty"`
	ActorType   string    `json:"actor_type"`
	ActorID     string    `json:"actor_id"`
	Intent      string    `json:"intent"`
	FromState   string    `json:"from_state"`
	ToState     string    `json:"to_state"`
	CreatedAt   time.Time `json:"created_at"`
}

// ListTaskEvents 查询调用者可见 Task 的审计事件摘要。
func (s *Service) ListTaskEvents(ctx context.Context, principal auth.Principal, query ListTaskEvents) (Envelope[TaskEventPage], error) {
	var result Envelope[TaskEventPage]
	if err := s.policy.Require(principal, "tasks:read"); err != nil {
		return result, err
	}
	if err := s.requireLiveAgent(ctx, principal); err != nil {
		return result, err
	}
	if err := s.checkRateLimit(ctx, principal); err != nil {
		return result, err
	}
	if query.TaskID == "" {
		return result, invalid("task_id")
	}
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return result, invalid("limit")
	}
	err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		// 校验任务存在且属于当前租户；不暴露无权限资源是否存在。
		if _, err := tx.GetTask(ctx, principal.TenantID, query.TaskID); err != nil {
			if domain.CodeOf(err) == "not_found" {
				return notFound()
			}
			return err
		}
		events, err := tx.ListTaskEvents(ctx, principal.TenantID, query.TaskID, query.AfterID, limit+1)
		if err != nil {
			return err
		}
		page := TaskEventPage{Events: events}
		hasMore := len(page.Events) > limit
		if hasMore {
			page.Events = page.Events[:limit]
			last := page.Events[len(page.Events)-1]
			page.NextCursor = encodeIDCursor(last.ID)
		}
		result = Envelope[TaskEventPage]{Data: page, Meta: Meta{ServerTime: now}}
		return nil
	})
	return result, err
}

// ListTaskEvents 查询参数。
type ListTaskEvents struct {
	TaskID  string
	Limit   int
	AfterID int64 // 基于事件自增 ID 的游标
}

// TaskEventPage 是审计事件分页结果。
type TaskEventPage struct {
	Events     []TaskEventSummary `json:"events"`
	NextCursor string             `json:"next_cursor,omitempty"`
}

func encodeIDCursor(id int64) string {
	// 简单可解码游标；实际生产应签名。
	return strconv.FormatInt(id, 10)
}

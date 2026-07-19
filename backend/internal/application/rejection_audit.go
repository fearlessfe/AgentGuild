package application

import (
	"context"
	"encoding/json"
	"log/slog"

	"agentguild.dev/agentguild/backend/internal/domain"
)

// RejectedIntentPrefix 是被拒绝迁移审计事件的 intent 前缀。
// 拒绝事件复用 task_events 现有列写入：intent 以该前缀区分正常迁移事件，
// to_state 等于 from_state（迁移未发生），reason 记录领域错误码。
const RejectedIntentPrefix = "reject:"

// rejectionAudit 描述一次被领域状态机拒绝的迁移尝试。
type rejectionAudit struct {
	taskID      string
	executionID string
	actorType   domain.ActorType
	actorID     string
	intent      string
	fromState   string
	reason      string
}

// auditRejection 在独立事务中追加拒绝审计事件，使审计不被业务事务回滚影响。
// 审计写入失败不掩盖原始拒绝错误，仅记录告警日志；拒绝事件不写入 outbox，
// 避免污染下游消费（遥测、声望投影）的正常事件流。
func auditRejection(ctx context.Context, store Store, tenantID string, rejection rejectionAudit) {
	err := store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		fields := map[string]string{"task_id": rejection.taskID}
		if rejection.executionID != "" {
			fields["execution_id"] = rejection.executionID
		}
		payload, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		return tx.AppendTaskEvent(ctx, TaskEvent{
			TenantID:    tenantID,
			TaskID:      rejection.taskID,
			ExecutionID: rejection.executionID,
			ActorType:   string(rejection.actorType),
			ActorID:     rejection.actorID,
			Intent:      RejectedIntentPrefix + rejection.intent,
			FromState:   rejection.fromState,
			ToState:     rejection.fromState,
			Reason:      rejection.reason,
			Payload:     payload,
			CreatedAt:   now,
		})
	})
	if err != nil {
		slog.Warn("rejection audit event append failed",
			"tenant", tenantID, "task", rejection.taskID, "execution", rejection.executionID,
			"actor", rejection.actorID, "intent", rejection.intent, "reason", rejection.reason,
			"error", err)
	}
}

// intentName 将领域意图映射为审计事件使用的稳定字符串。
func intentName(intent domain.Intent) string {
	switch intent {
	case domain.IntentPublish:
		return "publish"
	case domain.IntentClaim:
		return "claim"
	case domain.IntentCancel:
		return "cancel"
	case domain.IntentStart:
		return "start"
	case domain.IntentComplete:
		return "complete"
	case domain.IntentHeartbeat:
		return "heartbeat"
	case domain.IntentExpire:
		return "expire"
	case domain.IntentAccept:
		return "accept"
	case domain.IntentReject:
		return "reject"
	case domain.IntentRequestRevision:
		return "request_revision"
	case domain.IntentSubmitForReview:
		return "submit_for_review"
	case domain.IntentSubmit:
		return "submit"
	case domain.IntentStartValidation:
		return "start_validation"
	case domain.IntentFailValidation:
		return "fail_validation"
	case domain.IntentMarkReviewing:
		return "mark_reviewing"
	default:
		return "unknown"
	}
}

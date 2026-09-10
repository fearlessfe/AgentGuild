package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/contribution/domain"
)

type CriterionRepository interface {
	// Record 追加一条验证事实，返回 (事实, 是否首次插入, 错误)。
	Record(context.Context, *domain.CriterionResult) (*domain.CriterionResult, bool, error)
	ListLatest(ctx context.Context, tenantID, executionID string) ([]domain.CriterionResult, error)
	ListHistory(ctx context.Context, tenantID, executionID string) ([]domain.CriterionResult, error)
}

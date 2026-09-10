// Package criteria 把公共任务规格里的验收标准、验证作业的终态与 contribution
// 的 criterion 账本粘接起来。它是三个模块唯一的交汇点，让 git、publictask 与
// contribution 彼此保持独立。
package criteria

import (
	"context"
	"encoding/json"
	"errors"

	contributionapp "agentguild.dev/agentguild/backend/internal/contribution/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ProjectionCriteriaSource 从公共任务投影中读取某个 Execution 所属任务的
// 验收标准。私有（非公开发布）任务没有投影，返回空规格。
type ProjectionCriteriaSource struct {
	pool *pgxpool.Pool
}

func NewProjectionCriteriaSource(pool *pgxpool.Pool) *ProjectionCriteriaSource {
	return &ProjectionCriteriaSource{pool: pool}
}

func (s *ProjectionCriteriaSource) CriteriaForExecution(ctx context.Context, tenantID, executionID string) (contributionapp.TaskCriteria, error) {
	var taskID string
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT projection.task_id, projection.acceptance_criteria
		FROM executions AS execution
		JOIN public_task_projections AS projection
		  ON projection.resource_tenant_id = execution.tenant_id
		 AND projection.task_id = execution.task_id
		WHERE execution.tenant_id=$1 AND execution.id=$2`, tenantID, executionID).Scan(&taskID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return contributionapp.TaskCriteria{}, nil
	}
	if err != nil {
		return contributionapp.TaskCriteria{}, err
	}

	var criteria []publictaskdomain.AcceptanceCriterion
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &criteria); err != nil {
			return contributionapp.TaskCriteria{}, err
		}
	}
	return contributionapp.TaskCriteria{TaskID: taskID, Criteria: MapCriteria(criteria)}, nil
}

// MapCriteria 把公共任务规格中的验收标准降解为 contribution 模块使用的最小
// 投影，并在此处判定它是否可被自动验证。
func MapCriteria(criteria []publictaskdomain.AcceptanceCriterion) []contributionapp.CriterionSpec {
	specs := make([]contributionapp.CriterionSpec, 0, len(criteria))
	for _, criterion := range criteria {
		ref, automated := criterion.AutomatedVerifier()
		if automated && !knownVerifierRef(ref) {
			// 分析器是不可信输入：指向未知验证步骤的标准不能被自动判定。
			ref, automated = "", false
		}
		specs = append(specs, contributionapp.CriterionSpec{
			ID:           criterion.ID,
			Critical:     criterion.Critical,
			VerifierKind: criterion.VerifierKind,
			VerifierRef:  ref,
			Automated:    automated,
		})
	}
	return specs
}

// knownVerifierRef 直接以 git 领域的 DefaultValidationSteps 为准。复制一份
// 步骤名会随交付实现演进而悄悄漂移，那正是这里要避免的。
func knownVerifierRef(ref string) bool {
	for _, step := range gitdomain.DefaultValidationSteps {
		if string(step) == ref {
			return true
		}
	}
	return false
}

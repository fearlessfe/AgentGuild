package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/reward/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Resolver 把外部可见的标识翻译成"资源属于哪个租户"。
//
// 奖励的三个入口（公共任务、锁、争议）都用全局唯一 ID 寻址，而应用服务
// 全部按 resource_tenant_id 取数。翻译必须发生在授权之前：先查出资源真正
// 归属哪个租户，再判断调用方是否有权访问它——反过来（拿调用方租户去查）
// 会让跨租户的全局 Agent 永远查不到自己的锁。
//
// 翻译结果本身不对外暴露，只用于内部授权与取数。
type Resolver struct {
	pool *pgxpool.Pool
}

func NewResolver(pool *pgxpool.Pool) *Resolver { return &Resolver{pool: pool} }

// PublicTaskRef 是公共任务在奖励侧的寻址信息。
type PublicTaskRef struct {
	ResourceTenantID string
	TaskID           string
}

// PublicTask 解析已发布的公共任务。未发布或已撤销的任务一律 not_found：
// 匿名访问者不得通过奖励端点探测尚未公开的任务。
func (r *Resolver) PublicTask(ctx context.Context, publicTaskID string) (PublicTaskRef, error) {
	if publicTaskID == "" {
		return PublicTaskRef{}, domain.ErrNotFound
	}
	var ref PublicTaskRef
	err := r.pool.QueryRow(ctx, `
		SELECT resource_tenant_id, task_id
		FROM public_task_projections
		WHERE id=$1 AND status='published'`, publicTaskID,
	).Scan(&ref.ResourceTenantID, &ref.TaskID)
	if err != nil {
		return PublicTaskRef{}, translateLookup(err)
	}
	return ref, nil
}

// LockRef 是奖励锁的寻址信息。ExecutionID 用于给全局 Agent 做 grant 授权。
type LockRef struct {
	ResourceTenantID string
	LockID           string
	ExecutionID      string
	AgentID          string
}

func (r *Resolver) Lock(ctx context.Context, lockID string) (LockRef, error) {
	if lockID == "" {
		return LockRef{}, domain.ErrNotFound
	}
	ref := LockRef{LockID: lockID}
	err := r.pool.QueryRow(ctx, `
		SELECT resource_tenant_id, execution_id, agent_id
		FROM reward_locks WHERE id=$1`, lockID,
	).Scan(&ref.ResourceTenantID, &ref.ExecutionID, &ref.AgentID)
	if err != nil {
		return LockRef{}, translateLookup(err)
	}
	return ref, nil
}

// Dispute 解析争议所在的租户与锁。
func (r *Resolver) Dispute(ctx context.Context, disputeID string) (LockRef, error) {
	if disputeID == "" {
		return LockRef{}, domain.ErrNotFound
	}
	var ref LockRef
	err := r.pool.QueryRow(ctx, `
		SELECT d.resource_tenant_id, d.lock_id, l.execution_id, l.agent_id
		FROM reward_disputes d
		JOIN reward_locks l
		  ON l.resource_tenant_id=d.resource_tenant_id AND l.id=d.lock_id
		WHERE d.id=$1`, disputeID,
	).Scan(&ref.ResourceTenantID, &ref.LockID, &ref.ExecutionID, &ref.AgentID)
	if err != nil {
		return LockRef{}, translateLookup(err)
	}
	return ref, nil
}

func translateLookup(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

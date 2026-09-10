package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// ClaimParticipation 是 Claim 参与者能看到的最小上下文。
//
// 它刻意只暴露参与者真正需要的标识与时刻，不传整个 claimTarget：
// Claim 事务的内部结构不应成为其他模块的依赖面。
type ClaimParticipation struct {
	ResourceTenantID string
	TaskID           string
	ExecutionID      string
	AgentID          string
	AgentVersionID   string
	Deadline         time.Time
	Now              time.Time
}

// ClaimParticipant 让其他模块在 Claim 的**同一个事务**里追加写入。
//
// Claim() 是一个原子事务：奖励锁定必须与任务状态、Execution、grant 一起
// 提交或一起回滚。任何返回的错误都会让整个 Claim 失败、任务保持 open。
type ClaimParticipant interface {
	OnClaim(ctx context.Context, tx pgx.Tx, participation ClaimParticipation) error
}

// Option 配置 Repository。
type Option func(*Repository)

// WithClaimParticipants 注册 Claim 事务的参与者，按注册顺序调用。
func WithClaimParticipants(participants ...ClaimParticipant) Option {
	return func(r *Repository) {
		r.claimParticipants = append(r.claimParticipants, participants...)
	}
}

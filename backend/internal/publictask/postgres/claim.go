package postgres

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	core "agentguild.dev/agentguild/backend/internal/domain"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	publictaskapp "agentguild.dev/agentguild/backend/internal/publictask/application"
	publictaskdomain "agentguild.dev/agentguild/backend/internal/publictask/domain"
	"github.com/jackc/pgx/v5"
)

const publicClaimIdempotencyTTL = 24 * time.Hour

type claimTarget struct {
	ResourceTenantID           string
	TaskID                     string
	TaskSpecificationVersionID string
	ProjectionStatus           string
	PublisherAgentVersionID    string
	Deadline                   time.Time
	TaskStatus                 core.TaskStatus
	TaskStateVersion           int64
}

// Claim performs the complete public Claim transition in one short database
// transaction. No sponsor tenant identifier is present in its response.
func (r *Repository) Claim(ctx context.Context, request publictaskapp.PublicClaimRequest) (publictaskapp.Envelope[publictaskapp.PublicClaimView], error) {
	if request.PublicTaskID == "" || request.AgentID == "" || request.AgentVersionID == "" ||
		request.RequestID == "" || request.ExecutionID == "" || request.GrantID == "" ||
		request.OutboxEventID == "" || request.GrantTTL <= 0 {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, publictaskdomain.ErrInvalidArgument
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	now, err := claimDatabaseNow(ctx, tx)
	if err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	target, err := lockClaimTarget(ctx, tx, request.PublicTaskID)
	if err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if err := requireActiveIdentity(ctx, tx, request.AgentID, request.AgentVersionID); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}

	replay, result, err := acquirePublicClaimIdempotency(ctx, tx, *target, request, now)
	if err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if replay {
		if err := tx.Commit(ctx); err != nil {
			return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
		}
		return result, nil
	}
	if target.ProjectionStatus != string(publictaskdomain.StatusPublished) {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, publictaskdomain.ErrForbidden
	}

	task := &core.Task{
		ID: target.TaskID, TenantID: target.ResourceTenantID,
		PublisherID: target.PublisherAgentVersionID, Deadline: target.Deadline,
		Status: target.TaskStatus,
	}
	if err := task.Apply(core.IntentClaim, core.Actor{Type: core.ActorAgent, ID: request.AgentVersionID}, now); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	execution, err := core.NewLeasedExecution(
		request.ExecutionID, target.TaskID, target.ResourceTenantID,
		request.AgentVersionID, now, 1,
	)
	if err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	grantExpiry := now.Add(request.GrantTTL)
	if target.Deadline.Before(grantExpiry) {
		grantExpiry = target.Deadline
	}
	grant, err := participationdomain.NewGrant(participationdomain.NewGrantParams{
		ID: request.GrantID, ResourceTenantID: target.ResourceTenantID,
		TaskID: target.TaskID, ExecutionID: execution.ID,
		AgentID: request.AgentID, AgentVersionID: request.AgentVersionID,
		Scopes: request.GrantScopes, ExpiresAt: grantExpiry, CreatedAt: now,
	})
	if err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}

	claimed, err := claimSponsorTask(ctx, tx, *target, execution.ID, now)
	if err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if !claimed {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, core.ErrStateConflict
	}
	if err := insertPublicExecution(ctx, tx, execution, request.AgentID, target.TaskSpecificationVersionID, now); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if err := insertClaimGrant(ctx, tx, grant); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if err := appendClaimGrantAudit(ctx, tx, grant, now); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if err := appendPublicClaimEvents(ctx, tx, *target, request, execution.ID, now); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	// 参与者在同一事务里追加写入（目前是奖励锁定）。任何失败都会让整个
	// Claim 回滚、任务保持 open——不允许无资金背书的 claim。
	if err := r.runClaimParticipants(ctx, tx, ClaimParticipation{
		ResourceTenantID: target.ResourceTenantID, TaskID: target.TaskID,
		ExecutionID: execution.ID, AgentID: request.AgentID,
		AgentVersionID: request.AgentVersionID, Deadline: target.Deadline, Now: now,
	}); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}

	result = publictaskapp.Envelope[publictaskapp.PublicClaimView]{
		Data: publictaskapp.PublicClaimView{
			PublicTaskID: request.PublicTaskID, ExecutionID: execution.ID,
			AgentID: request.AgentID, AgentVersionID: request.AgentVersionID,
			TaskSpecificationVersionID: target.TaskSpecificationVersionID,
			Status:                     string(execution.Status), LeaseGeneration: execution.Lease.Generation,
			LeaseSoftExpiresAt: execution.Lease.SoftExpiry,
			LeaseHardExpiresAt: execution.Lease.HardExpiry, ClaimedAt: now,
			Grant: publictaskapp.PublicParticipationGrant{
				ID: grant.ID, Scopes: append([]participationdomain.Scope(nil), grant.Scopes...),
				ExpiresAt: grant.ExpiresAt,
			},
		},
		Meta: publictaskapp.Meta{ServerTime: now, ResourceVersion: target.TaskStateVersion + 1},
	}
	if err := completePublicClaimIdempotency(ctx, tx, *target, request, result, now); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	return result, nil
}

// runClaimParticipants 按注册顺序调用参与者。它保持串行：参与者之间可能
// 存在写序依赖，并发调用同一个 pgx.Tx 也不安全。
func (r *Repository) runClaimParticipants(ctx context.Context, tx pgx.Tx, participation ClaimParticipation) error {
	for _, participant := range r.claimParticipants {
		if err := participant.OnClaim(ctx, tx, participation); err != nil {
			return err
		}
	}
	return nil
}

func lockClaimTarget(ctx context.Context, tx pgx.Tx, publicTaskID string) (*claimTarget, error) {
	var target claimTarget
	var taskStatus string
	err := tx.QueryRow(ctx, `
		SELECT p.resource_tenant_id, p.task_id, p.task_specification_version_id,
		       p.status, t.publisher_agent_version_id, t.deadline, t.status,
		       t.state_version
		FROM public_task_projections p
		JOIN tasks t ON t.tenant_id=p.resource_tenant_id AND t.id=p.task_id
		WHERE p.id=$1
		FOR UPDATE OF p, t`, publicTaskID,
	).Scan(
		&target.ResourceTenantID, &target.TaskID,
		&target.TaskSpecificationVersionID, &target.ProjectionStatus,
		&target.PublisherAgentVersionID, &target.Deadline, &taskStatus,
		&target.TaskStateVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, publictaskdomain.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	target.TaskStatus = core.TaskStatus(taskStatus)
	return &target, nil
}

func requireActiveIdentity(ctx context.Context, tx pgx.Tx, agentID, versionID string) error {
	var allowed bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM agent_identities a
			JOIN agent_identity_versions v ON v.agent_id=a.id
			WHERE a.id=$1 AND a.status='active'
			  AND v.id=$2 AND v.status='active'
		)`, agentID, versionID,
	).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return publictaskdomain.ErrForbidden
	}
	return nil
}

func acquirePublicClaimIdempotency(ctx context.Context, tx pgx.Tx, target claimTarget, request publictaskapp.PublicClaimRequest, now time.Time) (bool, publictaskapp.Envelope[publictaskapp.PublicClaimView], error) {
	keyArgs := []any{target.ResourceTenantID, request.AgentID, "public_task_claim", request.RequestID}
	tag, err := tx.Exec(ctx, `
		INSERT INTO idempotency_records (
			tenant_id, actor_id, operation, request_id, request_hash,
			owner_token, expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		ON CONFLICT (tenant_id, actor_id, operation, request_id) DO NOTHING`,
		keyArgs[0], keyArgs[1], keyArgs[2], keyArgs[3], request.RequestHash[:],
		request.ExecutionID, now.Add(publicClaimIdempotencyTTL), now,
	)
	if err != nil {
		return false, publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if tag.RowsAffected() == 1 {
		return false, publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, nil
	}
	var storedHash, responseBody []byte
	var responseCode *int
	err = tx.QueryRow(ctx, `
		SELECT request_hash, response_code, response_body
		FROM idempotency_records
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4
		FOR UPDATE`, keyArgs...,
	).Scan(&storedHash, &responseCode, &responseBody)
	if err != nil {
		return false, publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	if !bytes.Equal(storedHash, request.RequestHash[:]) {
		return false, publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, &publictaskdomain.Error{Code: "idempotency_mismatch", Message: "idempotency key was already used with a different request"}
	}
	if responseCode == nil {
		return false, publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, &publictaskdomain.Error{Code: "idempotency_in_progress", Message: "idempotency request is already in progress"}
	}
	var result publictaskapp.Envelope[publictaskapp.PublicClaimView]
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return false, publictaskapp.Envelope[publictaskapp.PublicClaimView]{}, err
	}
	return true, result, nil
}

func claimSponsorTask(ctx context.Context, tx pgx.Tx, target claimTarget, executionID string, now time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE tasks
		SET status='claimed', state_version=state_version+1,
		    active_execution_id=$4, updated_at=$5
		WHERE tenant_id=$1 AND id=$2 AND status='open'
		  AND state_version=$3 AND deadline>$5`,
		target.ResourceTenantID, target.TaskID, target.TaskStateVersion, executionID, now,
	)
	return tag.RowsAffected() == 1, err
}

func insertPublicExecution(ctx context.Context, tx pgx.Tx, execution *core.Execution, agentID, specificationVersionID string, now time.Time) error {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return err
	}
	hash := sha256.Sum256(secret)
	_, err := tx.Exec(ctx, `
		INSERT INTO executions (
			tenant_id, id, task_id, agent_id, agent_version_id,
			task_specification_version_id, status, lease_secret_hash,
			lease_generation, lease_soft_expires_at, lease_hard_expires_at,
			claimed_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12,$12)`,
		execution.TenantID, execution.ID, execution.TaskID, agentID,
		execution.AgentID, specificationVersionID, execution.Status, hash[:],
		execution.Lease.Generation, execution.Lease.SoftExpiry,
		execution.Lease.HardExpiry, now,
	)
	return writeError(err)
}

func insertClaimGrant(ctx context.Context, tx pgx.Tx, grant *participationdomain.Grant) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO task_participation_grants (
			id, resource_tenant_id, task_id, execution_id, agent_id,
			agent_version_id, scopes, status, expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		grant.ID, grant.ResourceTenantID, grant.TaskID, grant.ExecutionID,
		grant.AgentID, grant.AgentVersionID, claimScopeStrings(grant.Scopes),
		grant.Status, grant.ExpiresAt, grant.CreatedAt, grant.UpdatedAt,
	)
	return writeError(err)
}

func appendClaimGrantAudit(ctx context.Context, tx pgx.Tx, grant *participationdomain.Grant, now time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO task_participation_grant_events (
			grant_id, event_type, actor_type, actor_id, agent_id,
			agent_version_id, resource_tenant_id, task_id, execution_id,
			reason_code, metadata, created_at
		) VALUES ($1,'created','agent',$2,$2,$3,$4,$5,$6,'public_claim','{}'::jsonb,$7)`,
		grant.ID, grant.AgentID, grant.AgentVersionID, grant.ResourceTenantID,
		grant.TaskID, grant.ExecutionID, now,
	)
	return err
}

func appendPublicClaimEvents(ctx context.Context, tx pgx.Tx, target claimTarget, request publictaskapp.PublicClaimRequest, executionID string, now time.Time) error {
	payload, err := json.Marshal(map[string]string{
		"public_task_id": request.PublicTaskID, "task_id": target.TaskID,
		"execution_id":                  executionID,
		"task_specification_version_id": target.TaskSpecificationVersionID,
	})
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO task_events (
			tenant_id, task_id, execution_id, actor_type, actor_id,
			intent, from_state, to_state, payload, created_at
		) VALUES ($1,$2,$3,'agent',$4,'claim',$5,'claimed',$6,$7)`,
		target.ResourceTenantID, target.TaskID, executionID,
		request.AgentVersionID, string(target.TaskStatus), payload, now,
	); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (
			tenant_id, id, event_type, aggregate_type, aggregate_id,
			payload, available_at
		) VALUES ($1,$2,'execution.claimed','execution',$3,$4,$5)`,
		target.ResourceTenantID, request.OutboxEventID, executionID, payload, now,
	)
	return err
}

func completePublicClaimIdempotency(ctx context.Context, tx pgx.Tx, target claimTarget, request publictaskapp.PublicClaimRequest, result publictaskapp.Envelope[publictaskapp.PublicClaimView], now time.Time) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE idempotency_records
		SET response_code=200, response_body=$5, updated_at=$6
		WHERE tenant_id=$1 AND actor_id=$2 AND operation=$3 AND request_id=$4
		  AND owner_token=$7 AND response_code IS NULL`,
		target.ResourceTenantID, request.AgentID, "public_task_claim",
		request.RequestID, body, now, request.ExecutionID,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return &publictaskdomain.Error{Code: "idempotency_in_progress", Message: "idempotency request is already in progress"}
	}
	return nil
}

func claimDatabaseNow(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var now time.Time
	err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now, err
}

func claimScopeStrings(scopes []participationdomain.Scope) []string {
	result := make([]string, len(scopes))
	for index, scope := range scopes {
		result[index] = string(scope)
	}
	return result
}

var _ publictaskapp.ClaimStore = (*Repository)(nil)

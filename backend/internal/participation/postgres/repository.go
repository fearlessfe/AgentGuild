package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/participation/application"
	"agentguild.dev/agentguild/backend/internal/participation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) application.Repository { return &Repository{pool: pool} }

func (r *Repository) Insert(ctx context.Context, grant *domain.Grant, actorType domain.ActorType, actorID string) error {
	if grant == nil || actorID == "" {
		return domain.ErrInvalidArgument
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `
		INSERT INTO task_participation_grants (
			id, resource_tenant_id, task_id, execution_id, agent_id,
			agent_version_id, scopes, status, expires_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		grant.ID, grant.ResourceTenantID, grant.TaskID, grant.ExecutionID,
		grant.AgentID, grant.AgentVersionID, scopeStrings(grant.Scopes), grant.Status,
		grant.ExpiresAt, grant.CreatedAt, grant.UpdatedAt)
	if err != nil {
		return writeError(err)
	}
	if err := appendAudit(ctx, tx, auditForGrant(grant, domain.EventCreated, actorType, actorID, "", "created", grant.CreatedAt)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) GetByID(ctx context.Context, id string) (*domain.Grant, error) {
	grant, err := scanGrant(r.pool.QueryRow(ctx, selectGrant+` WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return grant, err
}

func (r *Repository) Authorize(ctx context.Context, request domain.AccessRequest) (*domain.Grant, error) {
	if err := domain.ValidateAccessRequest(request); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	grant, err := scanGrant(tx.QueryRow(ctx, selectGrant+`
		WHERE resource_tenant_id=$1 AND execution_id=$2
		FOR UPDATE`, request.ResourceTenantID, request.ExecutionID))
	if errors.Is(err, pgx.ErrNoRows) {
		event := auditForRequest(request, domain.EventDenied, "forbidden", now)
		if err := appendAudit(ctx, tx, event); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, domain.ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	authorizeErr := grant.Authorizes(request, now)
	if errors.Is(authorizeErr, domain.ErrExpired) && grant.Status == domain.StatusActive {
		if err := grant.Expire(now); err != nil {
			return nil, err
		}
		if err := updateGrant(ctx, tx, grant); err != nil {
			return nil, err
		}
		if err := appendAudit(ctx, tx, auditForGrant(grant, domain.EventExpired, domain.ActorSystem, "grant-authorizer", request.Scope, "expired", now)); err != nil {
			return nil, err
		}
	}
	eventType, reason := domain.EventAllowed, "allowed"
	if authorizeErr != nil {
		eventType, reason = domain.EventDenied, reasonCode(authorizeErr)
	}
	if err := appendAudit(ctx, tx, auditForRequestWithGrant(request, grant.ID, eventType, reason, now)); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if authorizeErr != nil {
		return nil, authorizeErr
	}
	return grant, nil
}

func (r *Repository) Renew(ctx context.Context, id, actorID string, newExpiry time.Time) (*domain.Grant, error) {
	if id == "" || actorID == "" || newExpiry.IsZero() {
		return nil, domain.ErrInvalidArgument
	}
	return r.mutate(ctx, id, func(grant *domain.Grant, now time.Time) (domain.AuditEvent, error) {
		if err := grant.Renew(newExpiry, actorID, now); err != nil {
			return domain.AuditEvent{}, err
		}
		return auditForGrant(grant, domain.EventRenewed, domain.ActorHuman, actorID, "", "renewed", now), nil
	})
}

func (r *Repository) Revoke(ctx context.Context, id, actorID, reason string) (*domain.Grant, error) {
	if id == "" || actorID == "" || reason == "" {
		return nil, domain.ErrInvalidArgument
	}
	return r.mutate(ctx, id, func(grant *domain.Grant, now time.Time) (domain.AuditEvent, error) {
		if err := grant.Revoke(actorID, reason, now); err != nil {
			return domain.AuditEvent{}, err
		}
		return auditForGrant(grant, domain.EventRevoked, domain.ActorHuman, actorID, "", reason, now), nil
	})
}

func (r *Repository) mutate(ctx context.Context, id string, apply func(*domain.Grant, time.Time) (domain.AuditEvent, error)) (*domain.Grant, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	grant, err := scanGrant(tx.QueryRow(ctx, selectGrant+` WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	event, err := apply(grant, now)
	if err != nil {
		return nil, err
	}
	if err := updateGrant(ctx, tx, grant); err != nil {
		return nil, err
	}
	if err := appendAudit(ctx, tx, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return grant, nil
}

func (r *Repository) ExpireDue(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		return 0, domain.ErrInvalidArgument
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now, err := databaseNow(ctx, tx)
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, selectGrant+`
		WHERE status='active' AND expires_at <= $1
		ORDER BY expires_at, id
		FOR UPDATE SKIP LOCKED LIMIT $2`, now, limit)
	if err != nil {
		return 0, err
	}
	var grants []*domain.Grant
	for rows.Next() {
		grant, err := scanGrant(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, grant := range grants {
		if err := grant.Expire(now); err != nil {
			return 0, err
		}
		if err := updateGrant(ctx, tx, grant); err != nil {
			return 0, err
		}
		if err := appendAudit(ctx, tx, auditForGrant(grant, domain.EventExpired, domain.ActorSystem, "grant-reaper", "", "expired", now)); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(grants), nil
}

func (r *Repository) ListAuditByTask(ctx context.Context, tenantID, taskID string, limit int) ([]domain.AuditEvent, error) {
	if tenantID == "" || taskID == "" || limit < 1 || limit > 1000 {
		return nil, domain.ErrInvalidArgument
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, COALESCE(grant_id, ''), event_type, actor_type, actor_id,
		       agent_id, agent_version_id, resource_tenant_id, task_id,
		       execution_id, COALESCE(scope, ''), reason_code, metadata, created_at
		FROM task_participation_grant_events
		WHERE resource_tenant_id=$1 AND task_id=$2
		ORDER BY created_at, id LIMIT $3`, tenantID, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.AuditEvent
	for rows.Next() {
		event, err := scanAudit(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}
	return events, rows.Err()
}

const selectGrant = `
	SELECT id, resource_tenant_id, task_id, execution_id, agent_id,
	       agent_version_id, scopes, status, expires_at, created_at, updated_at,
	       revoked_at, COALESCE(revocation_actor, ''), COALESCE(revocation_reason, '')
	FROM task_participation_grants`

type scanner interface{ Scan(...any) error }

func scanGrant(row scanner) (*domain.Grant, error) {
	var grant domain.Grant
	var scopes []string
	err := row.Scan(&grant.ID, &grant.ResourceTenantID, &grant.TaskID,
		&grant.ExecutionID, &grant.AgentID, &grant.AgentVersionID, &scopes,
		&grant.Status, &grant.ExpiresAt, &grant.CreatedAt, &grant.UpdatedAt,
		&grant.RevokedAt, &grant.RevocationActor, &grant.RevocationReason)
	grant.Scopes = make([]domain.Scope, len(scopes))
	for index, scope := range scopes {
		grant.Scopes[index] = domain.Scope(scope)
	}
	return &grant, err
}

func updateGrant(ctx context.Context, tx pgx.Tx, grant *domain.Grant) error {
	_, err := tx.Exec(ctx, `
		UPDATE task_participation_grants
		SET status=$2, expires_at=$3, updated_at=$4, revoked_at=$5,
		    revocation_actor=NULLIF($6,''), revocation_reason=NULLIF($7,'')
		WHERE id=$1`, grant.ID, grant.Status, grant.ExpiresAt, grant.UpdatedAt,
		grant.RevokedAt, grant.RevocationActor, grant.RevocationReason)
	return err
}

func appendAudit(ctx context.Context, tx pgx.Tx, event domain.AuditEvent) error {
	var grantID any
	if event.GrantID != "" {
		grantID = event.GrantID
	}
	metadata := event.Metadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO task_participation_grant_events (
			grant_id, event_type, actor_type, actor_id, agent_id, agent_version_id,
			resource_tenant_id, task_id, execution_id, scope, reason_code,
			metadata, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,$12,$13)`,
		grantID, event.Type, event.ActorType, event.ActorID, event.AgentID,
		event.AgentVersionID, event.ResourceTenantID, event.TaskID,
		event.ExecutionID, event.Scope, event.ReasonCode, metadata, event.CreatedAt)
	return err
}

func scanAudit(row scanner) (*domain.AuditEvent, error) {
	var event domain.AuditEvent
	err := row.Scan(&event.ID, &event.GrantID, &event.Type, &event.ActorType,
		&event.ActorID, &event.AgentID, &event.AgentVersionID,
		&event.ResourceTenantID, &event.TaskID, &event.ExecutionID,
		&event.Scope, &event.ReasonCode, &event.Metadata, &event.CreatedAt)
	return &event, err
}

func auditForGrant(grant *domain.Grant, eventType domain.EventType, actorType domain.ActorType, actorID string, scope domain.Scope, reason string, now time.Time) domain.AuditEvent {
	return domain.AuditEvent{GrantID: grant.ID, Type: eventType, ActorType: actorType,
		ActorID: actorID, AgentID: grant.AgentID, AgentVersionID: grant.AgentVersionID,
		ResourceTenantID: grant.ResourceTenantID, TaskID: grant.TaskID,
		ExecutionID: grant.ExecutionID, Scope: scope, ReasonCode: reason, CreatedAt: now}
}

func auditForRequest(request domain.AccessRequest, eventType domain.EventType, reason string, now time.Time) domain.AuditEvent {
	return domain.AuditEvent{Type: eventType, ActorType: request.ActorType,
		ActorID: request.ActorID, AgentID: request.AgentID, AgentVersionID: request.AgentVersionID,
		ResourceTenantID: request.ResourceTenantID, TaskID: request.TaskID,
		ExecutionID: request.ExecutionID, Scope: request.Scope, ReasonCode: reason, CreatedAt: now}
}

func auditForRequestWithGrant(request domain.AccessRequest, grantID string, eventType domain.EventType, reason string, now time.Time) domain.AuditEvent {
	event := auditForRequest(request, eventType, reason, now)
	event.GrantID = grantID
	return event
}

func databaseNow(ctx context.Context, tx pgx.Tx) (time.Time, error) {
	var now time.Time
	err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	return now, err
}

func reasonCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrExpired):
		return "expired"
	case errors.Is(err, domain.ErrRevoked):
		return "revoked"
	default:
		return "forbidden"
	}
}

func scopeStrings(scopes []domain.Scope) []string {
	result := make([]string, len(scopes))
	for index, scope := range scopes {
		result[index] = string(scope)
	}
	return result
}

func writeError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrStateConflict
	}
	return err
}

var _ application.Repository = (*Repository)(nil)

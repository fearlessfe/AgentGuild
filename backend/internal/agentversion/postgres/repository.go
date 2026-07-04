package postgres

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/agentversion/application"
	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type versionRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

// NewVersionRepository returns a VersionRepository backed by pool.
func NewVersionRepository(pool *pgxpool.Pool) application.VersionRepository {
	return &versionRepository{
		q:   pool,
		now: func(context.Context) (time.Time, error) { return time.Now(), nil },
	}
}

func (r *versionRepository) Create(ctx context.Context, tx application.Tx, version *domain.AgentVersion) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO agent_versions (
			id, tenant_id, agent_id, version_number, parent_version_id, status,
			runtime, model, capabilities, config_fingerprint, content_hash,
			environment_digest, prompt_ref, skill_refs, memory_ref, tool_refs,
			created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		version.ID, version.TenantID, version.AgentID, version.VersionNumber,
		nullString(version.ParentVersionID), string(version.Status),
		version.Runtime, version.Model, stringSlice(version.Capabilities),
		version.ConfigFingerprint, version.ContentHash,
		version.EnvironmentDigest, version.PromptRef,
		stringSlice(version.SkillRefs), version.MemoryRef,
		stringSlice(version.ToolRefs), version.CreatedBy,
		version.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrStateConflict
		}
		return err
	}
	version.Persisted = true
	return nil
}

func (r *versionRepository) UpdateStatus(ctx context.Context, tx application.Tx, version *domain.AgentVersion) error {
	var promotedAt, retiredAt any
	if version.PromotedAt != nil {
		promotedAt = *version.PromotedAt
	}
	if version.RetiredAt != nil {
		retiredAt = *version.RetiredAt
	}

	tag, err := tx.Exec(ctx, `
		UPDATE agent_versions
		SET status=$4,
		    promoted_at=COALESCE($5, promoted_at),
		    retired_at=COALESCE($6, retired_at)
		WHERE tenant_id=$1 AND agent_id=$2 AND id=$3`,
		version.TenantID, version.AgentID, version.ID,
		string(version.Status), promotedAt, retiredAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *versionRepository) GetByID(ctx context.Context, tenantID, agentID, versionID string) (*domain.AgentVersion, error) {
	var version domain.AgentVersion
	var parentID, envDigest, promptRef, memoryRef, createdBy sql.NullString
	var promotedAt, retiredAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, version_number, parent_version_id, status,
		       runtime, model, capabilities, config_fingerprint, content_hash,
		       environment_digest, prompt_ref, skill_refs, memory_ref, tool_refs,
		       created_by, created_at, promoted_at, retired_at
		FROM agent_versions
		WHERE tenant_id=$1 AND agent_id=$2 AND id=$3`,
		tenantID, agentID, versionID,
	).Scan(
		&version.ID, &version.TenantID, &version.AgentID, &version.VersionNumber,
		&parentID, &version.Status, &version.Runtime, &version.Model,
		&version.Capabilities, &version.ConfigFingerprint, &version.ContentHash,
		&envDigest, &promptRef, &version.SkillRefs, &memoryRef, &version.ToolRefs,
		&createdBy, &version.CreatedAt, &promotedAt, &retiredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	version.ParentVersionID = parentID.String
	version.EnvironmentDigest = envDigest.String
	version.PromptRef = promptRef.String
	version.MemoryRef = memoryRef.String
	version.CreatedBy = createdBy.String
	version.PromotedAt = promotedAt
	version.RetiredAt = retiredAt
	version.Persisted = true
	return &version, nil
}

func (r *versionRepository) ListByAgent(ctx context.Context, tenantID, agentID string) ([]domain.AgentVersion, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, agent_id, version_number, parent_version_id, status,
		       runtime, model, capabilities, config_fingerprint, content_hash,
		       environment_digest, prompt_ref, skill_refs, memory_ref, tool_refs,
		       created_by, created_at, promoted_at, retired_at
		FROM agent_versions
		WHERE tenant_id=$1 AND agent_id=$2
		ORDER BY version_number DESC`,
		tenantID, agentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []domain.AgentVersion
	for rows.Next() {
		version, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, *version)
	}
	return versions, rows.Err()
}

func (r *versionRepository) GetLatestByAgent(ctx context.Context, tenantID, agentID string) (*domain.AgentVersion, error) {
	var version domain.AgentVersion
	var parentID, envDigest, promptRef, memoryRef, createdBy sql.NullString
	var promotedAt, retiredAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, version_number, parent_version_id, status,
		       runtime, model, capabilities, config_fingerprint, content_hash,
		       environment_digest, prompt_ref, skill_refs, memory_ref, tool_refs,
		       created_by, created_at, promoted_at, retired_at
		FROM agent_versions
		WHERE tenant_id=$1 AND agent_id=$2
		ORDER BY version_number DESC
		LIMIT 1`,
		tenantID, agentID,
	).Scan(
		&version.ID, &version.TenantID, &version.AgentID, &version.VersionNumber,
		&parentID, &version.Status, &version.Runtime, &version.Model,
		&version.Capabilities, &version.ConfigFingerprint, &version.ContentHash,
		&envDigest, &promptRef, &version.SkillRefs, &memoryRef, &version.ToolRefs,
		&createdBy, &version.CreatedAt, &promotedAt, &retiredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	version.ParentVersionID = parentID.String
	version.EnvironmentDigest = envDigest.String
	version.PromptRef = promptRef.String
	version.MemoryRef = memoryRef.String
	version.CreatedBy = createdBy.String
	version.PromotedAt = promotedAt
	version.RetiredAt = retiredAt
	version.Persisted = true
	return &version, nil
}

func (r *versionRepository) GetActiveByAgent(ctx context.Context, tenantID, agentID string) (*domain.AgentVersion, error) {
	var version domain.AgentVersion
	var parentID, envDigest, promptRef, memoryRef, createdBy sql.NullString
	var promotedAt, retiredAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, version_number, parent_version_id, status,
		       runtime, model, capabilities, config_fingerprint, content_hash,
		       environment_digest, prompt_ref, skill_refs, memory_ref, tool_refs,
		       created_by, created_at, promoted_at, retired_at
		FROM agent_versions
		WHERE tenant_id=$1 AND agent_id=$2 AND status='active'
		ORDER BY version_number DESC
		LIMIT 1`,
		tenantID, agentID,
	).Scan(
		&version.ID, &version.TenantID, &version.AgentID, &version.VersionNumber,
		&parentID, &version.Status, &version.Runtime, &version.Model,
		&version.Capabilities, &version.ConfigFingerprint, &version.ContentHash,
		&envDigest, &promptRef, &version.SkillRefs, &memoryRef, &version.ToolRefs,
		&createdBy, &version.CreatedAt, &promotedAt, &retiredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	version.ParentVersionID = parentID.String
	version.EnvironmentDigest = envDigest.String
	version.PromptRef = promptRef.String
	version.MemoryRef = memoryRef.String
	version.CreatedBy = createdBy.String
	version.PromotedAt = promotedAt
	version.RetiredAt = retiredAt
	version.Persisted = true
	return &version, nil
}

func (r *versionRepository) LockAgent(ctx context.Context, tx application.Tx, tenantID, agentID string) error {
	var unused int
	err := tx.QueryRow(ctx, `
		SELECT 1 FROM agents
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE`,
		tenantID, agentID,
	).Scan(&unused)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func (r *versionRepository) UpdateAgentCurrentVersion(ctx context.Context, tx application.Tx, tenantID, agentID, versionID string) error {
	now, err := tx.Now(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE agents
		SET current_version_id=$3, updated_at=$4
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, agentID, nullString(versionID), now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *versionRepository) GetAgentOwner(ctx context.Context, tenantID, agentID string) (string, error) {
	var ownerID string
	err := r.q.QueryRow(ctx, `
		SELECT owner_id FROM agents
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, agentID,
	).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return ownerID, err
}

var _ application.VersionRepository = (*versionRepository)(nil)

type versionScanner interface {
	Scan(...any) error
}

func scanVersion(row versionScanner) (*domain.AgentVersion, error) {
	var version domain.AgentVersion
	var parentID, envDigest, promptRef, memoryRef, createdBy sql.NullString
	var promotedAt, retiredAt *time.Time
	if err := row.Scan(
		&version.ID, &version.TenantID, &version.AgentID, &version.VersionNumber,
		&parentID, &version.Status, &version.Runtime, &version.Model,
		&version.Capabilities, &version.ConfigFingerprint, &version.ContentHash,
		&envDigest, &promptRef, &version.SkillRefs, &memoryRef, &version.ToolRefs,
		&createdBy, &version.CreatedAt, &promotedAt, &retiredAt,
	); err != nil {
		return nil, err
	}
	version.ParentVersionID = parentID.String
	version.EnvironmentDigest = envDigest.String
	version.PromptRef = promptRef.String
	version.MemoryRef = memoryRef.String
	version.CreatedBy = createdBy.String
	version.PromotedAt = promotedAt
	version.RetiredAt = retiredAt
	version.Persisted = true
	return &version, nil
}

func stringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

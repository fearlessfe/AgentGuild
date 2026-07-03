package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type agentRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewAgentRepository(pool *pgxpool.Pool) application.AgentRepository {
	return &agentRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *agentRepository) Insert(ctx context.Context, agent *domain.Agent) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO agents (
			id, tenant_id, owner_id, owner_email, team, name, description, status,
			current_version_id, scopes, repo_scope, budget_cents, budget_currency,
			last_seen_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)`,
		agent.ID, agent.TenantID, agent.OwnerID, agent.OwnerEmail, nullString(agent.Team), agent.Name,
		nullString(agent.Description), agent.Status, nullString(agent.CurrentVersionID), stringSlice(agent.Scopes),
		stringSlice(agent.RepoScope), agent.BudgetCents, nullString(agent.BudgetCurrency), agent.LastSeenAt, now,
	)
	return err
}

func (r *agentRepository) GetByID(ctx context.Context, tenantID, agentID string) (*domain.Agent, error) {
	var agent domain.Agent
	var team, description, currentVersionID, budgetCurrency sql.NullString
	var lastSeenAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, owner_id, owner_email, team, name, description, status,
		       current_version_id, scopes, repo_scope, budget_cents, budget_currency,
		       last_seen_at, created_at, updated_at
		FROM agents
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, agentID,
	).Scan(
		&agent.ID, &agent.TenantID, &agent.OwnerID, &agent.OwnerEmail, &team,
		&agent.Name, &description, &agent.Status, &currentVersionID,
		&agent.Scopes, &agent.RepoScope, &agent.BudgetCents, &budgetCurrency,
		&lastSeenAt, &agent.CreatedAt, &agent.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	agent.Team = team.String
	agent.Description = description.String
	agent.CurrentVersionID = currentVersionID.String
	agent.BudgetCurrency = budgetCurrency.String
	agent.LastSeenAt = lastSeenAt
	return &agent, nil
}

func (r *agentRepository) List(ctx context.Context, query application.AgentListQuery) ([]domain.Agent, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, owner_id, owner_email, team, name, description, status,
		       current_version_id, scopes, repo_scope, budget_cents, budget_currency,
		       last_seen_at, created_at, updated_at
		FROM agents
		WHERE tenant_id=$1
		  AND ($2='' OR owner_id=$2)
		ORDER BY created_at DESC, id ASC
		LIMIT $3`,
		query.TenantID, query.OwnerID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []domain.Agent
	for rows.Next() {
		agent, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		agents = append(agents, *agent)
	}
	return agents, rows.Err()
}

func (r *agentRepository) Update(ctx context.Context, agent *domain.Agent) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	expectedStatus, conditional := expectedCurrentStatus(agent)
	if conditional {
		tag, err := r.q.Exec(ctx, `
		UPDATE agents
		SET owner_id=$3, owner_email=$4, team=$5, name=$6, description=$7, status=$8,
		    current_version_id=$9, scopes=$10, repo_scope=$11, budget_cents=$12,
		    budget_currency=$13, last_seen_at=$14, updated_at=$15
		WHERE tenant_id=$1 AND id=$2 AND status=$16`,
			agent.TenantID, agent.ID, agent.OwnerID, agent.OwnerEmail, nullString(agent.Team), agent.Name,
			nullString(agent.Description), agent.Status, nullString(agent.CurrentVersionID), stringSlice(agent.Scopes),
			stringSlice(agent.RepoScope), agent.BudgetCents, nullString(agent.BudgetCurrency), agent.LastSeenAt, now,
			expectedStatus,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrStateConflict
		}
		return nil
	}

	tag, err := r.q.Exec(ctx, `
		UPDATE agents
		SET owner_id=$3, owner_email=$4, team=$5, name=$6, description=$7, status=$8,
		    current_version_id=$9, scopes=$10, repo_scope=$11, budget_cents=$12,
		    budget_currency=$13, last_seen_at=$14, updated_at=$15
		WHERE tenant_id=$1 AND id=$2`,
		agent.TenantID, agent.ID, agent.OwnerID, agent.OwnerEmail, nullString(agent.Team), agent.Name,
		nullString(agent.Description), agent.Status, nullString(agent.CurrentVersionID), stringSlice(agent.Scopes),
		stringSlice(agent.RepoScope), agent.BudgetCents, nullString(agent.BudgetCurrency), agent.LastSeenAt, now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type versionRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewVersionRepository(pool *pgxpool.Pool) application.VersionRepository {
	return &versionRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *versionRepository) Insert(ctx context.Context, version *domain.AgentVersion) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO agent_versions (
			id, tenant_id, agent_id, version_number, runtime, model,
			capabilities, config_fingerprint, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		version.ID, version.TenantID, version.AgentID, version.VersionNumber,
		version.Runtime, version.Model, stringSlice(version.Capabilities), version.ConfigFingerprint,
		version.CreatedAt,
	)
	return err
}

func (r *versionRepository) GetByID(ctx context.Context, tenantID, versionID string) (*domain.AgentVersion, error) {
	var version domain.AgentVersion
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, version_number, runtime, model,
		       capabilities, config_fingerprint, created_at
		FROM agent_versions
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, versionID,
	).Scan(
		&version.ID, &version.TenantID, &version.AgentID, &version.VersionNumber,
		&version.Runtime, &version.Model, &version.Capabilities,
		&version.ConfigFingerprint, &version.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *versionRepository) ListByAgent(ctx context.Context, tenantID, agentID string) ([]domain.AgentVersion, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, tenant_id, agent_id, version_number, runtime, model,
		       capabilities, config_fingerprint, created_at
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
		var version domain.AgentVersion
		if err := rows.Scan(
			&version.ID, &version.TenantID, &version.AgentID, &version.VersionNumber,
			&version.Runtime, &version.Model, &version.Capabilities,
			&version.ConfigFingerprint, &version.CreatedAt,
		); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

type credentialRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewCredentialRepository(pool *pgxpool.Pool) application.CredentialRepository {
	return &credentialRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *credentialRepository) Insert(ctx context.Context, cred *domain.ActivationCredential) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO activation_credentials (
			id, tenant_id, agent_id, hash, status, expires_at, consumed_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		cred.ID, cred.TenantID, cred.AgentID, cred.Hash, cred.Status, cred.ExpiresAt,
		cred.ConsumedAt, cred.CreatedAt,
	)
	return err
}

func (r *credentialRepository) GetPending(ctx context.Context, tenantID, agentID string) (*domain.ActivationCredential, error) {
	return r.getByStatus(ctx, tenantID, agentID, domain.ActivationCredentialPending)
}

func (r *credentialRepository) GetPendingByPlaintext(ctx context.Context, token string) (*domain.ActivationCredential, error) {
	sum := sha256.Sum256([]byte(token))
	var cred domain.ActivationCredential
	var expiresAt, consumedAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, hash, status, expires_at, consumed_at, created_at
		FROM activation_credentials
		WHERE hash=$1 AND status=$2`,
		sum[:], domain.ActivationCredentialPending,
	).Scan(
		&cred.ID, &cred.TenantID, &cred.AgentID, &cred.Hash, &cred.Status,
		&expiresAt, &consumedAt, &cred.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cred.ExpiresAt = expiresAt
	cred.ConsumedAt = consumedAt
	return &cred, nil
}

func (r *credentialRepository) GetLatestByAgent(ctx context.Context, tenantID, agentID string) (*domain.ActivationCredential, error) {
	var cred domain.ActivationCredential
	var expiresAt, consumedAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, hash, status, expires_at, consumed_at, created_at
		FROM activation_credentials
		WHERE tenant_id=$1 AND agent_id=$2
		ORDER BY created_at DESC, id DESC
		LIMIT 1`,
		tenantID, agentID,
	).Scan(
		&cred.ID, &cred.TenantID, &cred.AgentID, &cred.Hash, &cred.Status,
		&expiresAt, &consumedAt, &cred.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cred.ExpiresAt = expiresAt
	cred.ConsumedAt = consumedAt
	return &cred, nil
}

func (r *credentialRepository) GetByID(ctx context.Context, tenantID, credID string) (*domain.ActivationCredential, error) {
	var cred domain.ActivationCredential
	var expiresAt, consumedAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, hash, status, expires_at, consumed_at, created_at
		FROM activation_credentials
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, credID,
	).Scan(
		&cred.ID, &cred.TenantID, &cred.AgentID, &cred.Hash, &cred.Status,
		&expiresAt, &consumedAt, &cred.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cred.ExpiresAt = expiresAt
	cred.ConsumedAt = consumedAt
	return &cred, nil
}

func (r *credentialRepository) getByStatus(ctx context.Context, tenantID, agentID, status string) (*domain.ActivationCredential, error) {
	var cred domain.ActivationCredential
	var expiresAt, consumedAt *time.Time
	err := r.q.QueryRow(ctx, `
		SELECT id, tenant_id, agent_id, hash, status, expires_at, consumed_at, created_at
		FROM activation_credentials
		WHERE tenant_id=$1 AND agent_id=$2 AND status=$3`,
		tenantID, agentID, status,
	).Scan(
		&cred.ID, &cred.TenantID, &cred.AgentID, &cred.Hash, &cred.Status,
		&expiresAt, &consumedAt, &cred.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cred.ExpiresAt = expiresAt
	cred.ConsumedAt = consumedAt
	return &cred, nil
}

func (r *credentialRepository) Save(ctx context.Context, cred *domain.ActivationCredential) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	if cred.Status == domain.ActivationCredentialConsumed {
		tag, err := r.q.Exec(ctx, `
			UPDATE activation_credentials
			SET status=$4, consumed_at=$1
			WHERE tenant_id=$2
			  AND agent_id=$3
			  AND id=$5
			  AND hash=$6
			  AND status='pending'
			  AND expires_at>$1`,
			now, cred.TenantID, cred.AgentID, cred.Status, cred.ID, cred.Hash,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrTokenExpired
		}
		return nil
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE activation_credentials
		SET hash=$3, status=$4, expires_at=$5, consumed_at=$6
		WHERE tenant_id=$1 AND id=$2`,
		cred.TenantID, cred.ID, cred.Hash, cred.Status, cred.ExpiresAt, cred.ConsumedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

var _ application.AgentRepository = (*agentRepository)(nil)
var _ application.VersionRepository = (*versionRepository)(nil)
var _ application.CredentialRepository = (*credentialRepository)(nil)

type agentScanner interface {
	Scan(...any) error
}

func scanAgent(row agentScanner) (*domain.Agent, error) {
	var agent domain.Agent
	var team, description, currentVersionID, budgetCurrency sql.NullString
	var lastSeenAt *time.Time
	if err := row.Scan(
		&agent.ID, &agent.TenantID, &agent.OwnerID, &agent.OwnerEmail, &team,
		&agent.Name, &description, &agent.Status, &currentVersionID,
		&agent.Scopes, &agent.RepoScope, &agent.BudgetCents, &budgetCurrency,
		&lastSeenAt, &agent.CreatedAt, &agent.UpdatedAt,
	); err != nil {
		return nil, err
	}
	agent.Team = team.String
	agent.Description = description.String
	agent.CurrentVersionID = currentVersionID.String
	agent.BudgetCurrency = budgetCurrency.String
	agent.LastSeenAt = lastSeenAt
	return &agent, nil
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

func expectedCurrentStatus(agent *domain.Agent) (string, bool) {
	for i := len(agent.Events) - 1; i >= 0; i-- {
		event := agent.Events[i]
		if event.AgentID == agent.ID && event.TenantID == agent.TenantID && event.ToState == agent.Status && event.FromState != "" {
			return event.FromState, true
		}
	}
	return "", false
}

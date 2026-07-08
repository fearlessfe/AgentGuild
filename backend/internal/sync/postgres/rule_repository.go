package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/sync/application"
	"agentguild.dev/agentguild/backend/internal/sync/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type RuleRepository struct {
	q queryer
}

func NewRuleRepository(pool *pgxpool.Pool) *RuleRepository {
	return &RuleRepository{q: pool}
}

func (r *RuleRepository) Create(ctx context.Context, rule *domain.Rule) error {
	includeLabels, excludeLabels, err := marshalLabels(rule)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
			INSERT INTO sync_rules (
				id, tenant_id, repo, include_labels, exclude_labels, issue_state,
				task_type, default_priority, dedupe_strategy, source_auth, enabled, last_synced_at,
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		rule.ID, rule.TenantID, rule.Repo, includeLabels, excludeLabels, rule.IssueState,
		rule.TaskType, rule.DefaultPriority, rule.DedupeStrategy, rule.SourceAuth, rule.Enabled,
		nullableTime(rule.LastSyncedAt), rule.CreatedAt, rule.UpdatedAt,
	)
	return err
}

func (r *RuleRepository) Get(ctx context.Context, tenantID, id string) (*domain.Rule, error) {
	rule, err := scanRule(r.q.QueryRow(ctx, `
			SELECT id, tenant_id, repo, include_labels, exclude_labels, issue_state,
			       task_type, default_priority, dedupe_strategy, source_auth, enabled, last_synced_at,
			       created_at, updated_at
		FROM sync_rules
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return rule, err
}

func (r *RuleRepository) List(ctx context.Context, tenantID string) ([]*domain.Rule, error) {
	rows, err := r.q.Query(ctx, `
			SELECT id, tenant_id, repo, include_labels, exclude_labels, issue_state,
			       task_type, default_priority, dedupe_strategy, source_auth, enabled, last_synced_at,
			       created_at, updated_at
		FROM sync_rules
		WHERE tenant_id=$1
		ORDER BY created_at ASC, id ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

func (r *RuleRepository) ListEnabledAllTenants(ctx context.Context) ([]*domain.Rule, error) {
	rows, err := r.q.Query(ctx, `
			SELECT id, tenant_id, repo, include_labels, exclude_labels, issue_state,
			       task_type, default_priority, dedupe_strategy, source_auth, enabled, last_synced_at,
			       created_at, updated_at
		FROM sync_rules
		WHERE enabled=true
		ORDER BY tenant_id ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRules(rows)
}

func (r *RuleRepository) Update(ctx context.Context, rule *domain.Rule) error {
	includeLabels, excludeLabels, err := marshalLabels(rule)
	if err != nil {
		return err
	}
	tag, err := r.q.Exec(ctx, `
			UPDATE sync_rules
			SET repo=$3, include_labels=$4, exclude_labels=$5, issue_state=$6,
			    task_type=$7, default_priority=$8, dedupe_strategy=$9, source_auth=$10,
			    enabled=$11, last_synced_at=$12, updated_at=$13
			WHERE tenant_id=$1 AND id=$2`,
		rule.TenantID, rule.ID, rule.Repo, includeLabels, excludeLabels,
		rule.IssueState, rule.TaskType, rule.DefaultPriority, rule.DedupeStrategy,
		rule.SourceAuth, rule.Enabled, nullableTime(rule.LastSyncedAt), rule.UpdatedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RuleRepository) Delete(ctx context.Context, tenantID, id string) error {
	tag, err := r.q.Exec(ctx, `DELETE FROM sync_rules WHERE tenant_id=$1 AND id=$2`, tenantID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *RuleRepository) TouchSynced(ctx context.Context, tenantID, id string, syncedAt time.Time) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE sync_rules
		SET last_synced_at=$3, updated_at=$3
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id, syncedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type ruleScanner interface {
	Scan(...any) error
}

func scanRules(rows pgx.Rows) ([]*domain.Rule, error) {
	var rules []*domain.Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func scanRule(row ruleScanner) (*domain.Rule, error) {
	var rule domain.Rule
	var includeLabels, excludeLabels []byte
	var lastSyncedAt sql.NullTime
	if err := row.Scan(
		&rule.ID,
		&rule.TenantID,
		&rule.Repo,
		&includeLabels,
		&excludeLabels,
		&rule.IssueState,
		&rule.TaskType,
		&rule.DefaultPriority,
		&rule.DedupeStrategy,
		&rule.SourceAuth,
		&rule.Enabled,
		&lastSyncedAt,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(includeLabels, &rule.IncludeLabels); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(excludeLabels, &rule.ExcludeLabels); err != nil {
		return nil, err
	}
	if lastSyncedAt.Valid {
		rule.LastSyncedAt = lastSyncedAt.Time
	}
	return &rule, nil
}

func marshalLabels(rule *domain.Rule) ([]byte, []byte, error) {
	includeLabels, err := json.Marshal(rule.IncludeLabels)
	if err != nil {
		return nil, nil, err
	}
	excludeLabels, err := json.Marshal(rule.ExcludeLabels)
	if err != nil {
		return nil, nil, err
	}
	return includeLabels, excludeLabels, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

var _ application.RuleRepository = (*RuleRepository)(nil)

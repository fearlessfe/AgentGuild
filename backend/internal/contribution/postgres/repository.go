package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/contribution/application"
	"agentguild.dev/agentguild/backend/internal/contribution/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) application.Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Insert(ctx context.Context, contribution *domain.Contribution) error {
	if contribution == nil {
		return domain.ErrInvalidArgument
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO contributions (
			id, resource_tenant_id, agent_id, agent_version_id, task_id,
			execution_id, task_specification_version_id, canonical_repository,
			self_owned_repository, issue_number, issue_url, provider, pull_request_number,
			pull_request_url, commit_sha, attribution_status, outcome, created_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18
		)`,
		contribution.ID, contribution.ResourceTenantID, contribution.AgentID,
		contribution.AgentVersionID, contribution.TaskID, contribution.ExecutionID,
		contribution.TaskSpecificationVersionID, contribution.CanonicalRepository,
		contribution.SelfOwnedRepository, contribution.IssueNumber, contribution.IssueURL, contribution.Provider,
		contribution.PullRequestNumber, contribution.PullRequestURL,
		contribution.CommitSHA, contribution.AttributionStatus, contribution.Outcome,
		contribution.CreatedAt,
	)
	return writeError(err)
}

func (r *Repository) GetByID(ctx context.Context, id string) (*domain.Contribution, error) {
	contribution, err := scanContribution(r.pool.QueryRow(ctx, `
		SELECT id, resource_tenant_id, agent_id, agent_version_id, task_id,
		       execution_id, task_specification_version_id, canonical_repository,
		       self_owned_repository, issue_number, issue_url, provider, pull_request_number,
		       pull_request_url, commit_sha, attribution_status, outcome, created_at
		FROM contributions
		WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return contribution, err
}

func (r *Repository) ListVerified(ctx context.Context) ([]domain.Contribution, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, resource_tenant_id, agent_id, agent_version_id, task_id,
		       execution_id, task_specification_version_id, canonical_repository,
		       self_owned_repository, issue_number, issue_url, provider, pull_request_number,
		       pull_request_url, commit_sha, attribution_status, outcome, created_at
		FROM contributions
		WHERE attribution_status='verified'
		ORDER BY agent_id, agent_version_id, created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	contributions := make([]domain.Contribution, 0)
	for rows.Next() {
		contribution, err := scanContribution(rows)
		if err != nil {
			return nil, err
		}
		contributions = append(contributions, *contribution)
	}
	return contributions, rows.Err()
}

func (r *Repository) AppendEvent(ctx context.Context, event *domain.ContributionEvent) (*domain.ContributionEvent, bool, error) {
	if event == nil {
		return nil, false, domain.ErrInvalidArgument
	}
	inserted, err := scanEvent(r.pool.QueryRow(ctx, `
		INSERT INTO contribution_events (
			contribution_id, provider, provider_delivery_id, object_version,
			event_type, outcome, commit_sha, payload, occurred_at, received_at
		) VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6,$7,$8,$9,$10)
		ON CONFLICT DO NOTHING
		RETURNING id, contribution_id, provider, COALESCE(provider_delivery_id, ''),
		          COALESCE(object_version, ''), event_type, outcome, commit_sha,
		          payload, occurred_at, received_at`,
		event.ContributionID, event.Provider, event.ProviderDeliveryID,
		event.ObjectVersion, event.Type, event.Outcome, event.CommitSHA,
		event.Payload, event.OccurredAt, event.ReceivedAt,
	))
	if err == nil {
		return inserted, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, writeError(err)
	}

	existing, err := r.findIdempotentEvent(ctx, event)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

func (r *Repository) ListEvents(ctx context.Context, contributionID string) ([]domain.ContributionEvent, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, contribution_id, provider, COALESCE(provider_delivery_id, ''),
		       COALESCE(object_version, ''), event_type, outcome, commit_sha,
		       payload, occurred_at, received_at
		FROM contribution_events
		WHERE contribution_id=$1
		ORDER BY occurred_at, id`, contributionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]domain.ContributionEvent, 0)
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, *event)
	}
	return events, rows.Err()
}

func (r *Repository) findIdempotentEvent(ctx context.Context, expected *domain.ContributionEvent) (*domain.ContributionEvent, error) {
	var byDelivery *domain.ContributionEvent
	if expected.ProviderDeliveryID != "" {
		var err error
		byDelivery, err = scanEvent(r.pool.QueryRow(ctx, `
			SELECT id, contribution_id, provider, COALESCE(provider_delivery_id, ''),
			       COALESCE(object_version, ''), event_type, outcome, commit_sha,
			       payload, occurred_at, received_at
			FROM contribution_events
			WHERE provider=$1 AND provider_delivery_id=$2`,
			expected.Provider, expected.ProviderDeliveryID))
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			byDelivery = nil
		}
	}

	var byObject *domain.ContributionEvent
	if expected.ObjectVersion != "" {
		var err error
		byObject, err = scanEvent(r.pool.QueryRow(ctx, `
			SELECT id, contribution_id, provider, COALESCE(provider_delivery_id, ''),
			       COALESCE(object_version, ''), event_type, outcome, commit_sha,
			       payload, occurred_at, received_at
			FROM contribution_events
			WHERE contribution_id=$1 AND provider=$2 AND event_type=$3 AND object_version=$4`,
			expected.ContributionID, expected.Provider, expected.Type, expected.ObjectVersion))
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			byObject = nil
		}
	}

	if byDelivery != nil && byObject != nil && byDelivery.ID != byObject.ID {
		return nil, domain.ErrStateConflict
	}
	if byDelivery != nil {
		if !byDelivery.SameContent(*expected) {
			return nil, domain.ErrStateConflict
		}
		if byDelivery.ObjectVersion != "" && expected.ObjectVersion != "" && byDelivery.ObjectVersion != expected.ObjectVersion {
			return nil, domain.ErrStateConflict
		}
		return byDelivery, nil
	}
	if byObject != nil {
		if !byObject.SameContent(*expected) {
			return nil, domain.ErrStateConflict
		}
		return byObject, nil
	}
	return nil, domain.ErrStateConflict
}

type scanner interface {
	Scan(...any) error
}

func scanContribution(row scanner) (*domain.Contribution, error) {
	var contribution domain.Contribution
	err := row.Scan(
		&contribution.ID, &contribution.ResourceTenantID, &contribution.AgentID,
		&contribution.AgentVersionID, &contribution.TaskID, &contribution.ExecutionID,
		&contribution.TaskSpecificationVersionID, &contribution.CanonicalRepository,
		&contribution.SelfOwnedRepository, &contribution.IssueNumber, &contribution.IssueURL, &contribution.Provider,
		&contribution.PullRequestNumber, &contribution.PullRequestURL,
		&contribution.CommitSHA, &contribution.AttributionStatus,
		&contribution.Outcome, &contribution.CreatedAt,
	)
	return &contribution, err
}

func scanEvent(row scanner) (*domain.ContributionEvent, error) {
	var event domain.ContributionEvent
	err := row.Scan(
		&event.ID, &event.ContributionID, &event.Provider,
		&event.ProviderDeliveryID, &event.ObjectVersion, &event.Type,
		&event.Outcome, &event.CommitSHA, &event.Payload,
		&event.OccurredAt, &event.ReceivedAt,
	)
	return &event, err
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

package postgres

import (
	"context"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type reviewerRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewReviewerRepository(pool *pgxpool.Pool) application.ReviewerRepository {
	return &reviewerRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *reviewerRepository) Insert(ctx context.Context, profile *domain.ReviewerProfile) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO reviewer_profiles (
			tenant_id, id, user_id, capabilities, current_load,
			is_active, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $7)`,
		profile.TenantID, profile.ID, profile.UserID, stringSlice(profile.Capabilities),
		profile.CurrentLoad, profile.IsActive, now,
	)
	return err
}

func (r *reviewerRepository) GetByID(ctx context.Context, tenantID, reviewerID string) (*domain.ReviewerProfile, error) {
	var profile domain.ReviewerProfile
	err := r.q.QueryRow(ctx, `
		SELECT tenant_id, id, user_id, capabilities, current_load,
		       is_active, created_at, updated_at
		FROM reviewer_profiles
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, reviewerID,
	).Scan(
		&profile.TenantID, &profile.ID, &profile.UserID, &profile.Capabilities,
		&profile.CurrentLoad, &profile.IsActive, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *reviewerRepository) ListActive(ctx context.Context, tenantID string, limit int) ([]domain.ReviewerProfile, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.q.Query(ctx, `
		SELECT tenant_id, id, user_id, capabilities, current_load,
		       is_active, created_at, updated_at
		FROM reviewer_profiles
		WHERE tenant_id=$1 AND is_active=true
		ORDER BY current_load ASC, created_at ASC
		LIMIT $2`,
		tenantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []domain.ReviewerProfile
	for rows.Next() {
		var profile domain.ReviewerProfile
		if err := rows.Scan(
			&profile.TenantID, &profile.ID, &profile.UserID, &profile.Capabilities,
			&profile.CurrentLoad, &profile.IsActive, &profile.CreatedAt, &profile.UpdatedAt,
		); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (r *reviewerRepository) IncrementLoad(ctx context.Context, tenantID, reviewerID string) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE reviewer_profiles
		SET current_load = current_load + 1, updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND is_active=true`,
		tenantID, reviewerID, now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		var active bool
		err := r.q.QueryRow(ctx, `
			SELECT is_active
			FROM reviewer_profiles
			WHERE tenant_id=$1 AND id=$2`,
			tenantID, reviewerID,
		).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) {
			return appdomain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if !active {
			return appdomain.ErrStateConflict
		}
	}
	return nil
}

func (r *reviewerRepository) DecrementLoad(ctx context.Context, tenantID, reviewerID string) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	tag, err := r.q.Exec(ctx, `
		UPDATE reviewer_profiles
		SET current_load = current_load - 1, updated_at=$3
		WHERE tenant_id=$1 AND id=$2 AND current_load > 0`,
		tenantID, reviewerID, now,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return appdomain.ErrNotFound
	}
	return nil
}

var _ application.ReviewerRepository = (*reviewerRepository)(nil)

// NewReviewerRepositoryFromTx returns a reviewer repository bound to an existing pgx transaction.
func NewReviewerRepositoryFromTx(tx pgx.Tx, now func(context.Context) (time.Time, error)) application.ReviewerRepository {
	return &reviewerRepository{q: tx, now: now}
}

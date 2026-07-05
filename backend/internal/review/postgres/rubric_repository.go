package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	appdomain "agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type rubricRepository struct {
	q   queryer
	now func(context.Context) (time.Time, error)
}

func NewRubricRepository(pool *pgxpool.Pool) application.RubricRepository {
	return &rubricRepository{q: pool, now: func(context.Context) (time.Time, error) { return time.Now(), nil }}
}

func (r *rubricRepository) CreateVersion(ctx context.Context, version *domain.RubricVersion) error {
	now, err := r.now(ctx)
	if err != nil {
		return err
	}
	dimensions, err := json.Marshal(version.Dimensions)
	if err != nil {
		return err
	}
	weights, err := json.Marshal(version.Weights)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `
		INSERT INTO rubric_versions (
			tenant_id, id, version_number, name, dimensions, weights,
			algorithm_version, is_active, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		version.TenantID, version.ID, version.VersionNumber, version.Name,
		dimensions, weights, version.AlgorithmVersion, version.IsActive,
		now,
	)
	return err
}

func (r *rubricRepository) GetActive(ctx context.Context, tenantID string) (*domain.RubricVersion, error) {
	var version domain.RubricVersion
	var dimensions, weights []byte
	err := r.q.QueryRow(ctx, `
		SELECT tenant_id, id, version_number, name, dimensions, weights,
		       algorithm_version, is_active, created_at
		FROM rubric_versions
		WHERE tenant_id=$1 AND is_active=true`,
		tenantID,
	).Scan(
		&version.TenantID, &version.ID, &version.VersionNumber, &version.Name,
		&dimensions, &weights, &version.AlgorithmVersion, &version.IsActive,
		&version.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return scanRubricVersionData(&version, dimensions, weights)
}

func (r *rubricRepository) GetByID(ctx context.Context, tenantID, rubricID string) (*domain.RubricVersion, error) {
	var version domain.RubricVersion
	var dimensions, weights []byte
	err := r.q.QueryRow(ctx, `
		SELECT tenant_id, id, version_number, name, dimensions, weights,
		       algorithm_version, is_active, created_at
		FROM rubric_versions
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, rubricID,
	).Scan(
		&version.TenantID, &version.ID, &version.VersionNumber, &version.Name,
		&dimensions, &weights, &version.AlgorithmVersion, &version.IsActive,
		&version.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, appdomain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return scanRubricVersionData(&version, dimensions, weights)
}

func (r *rubricRepository) ListVersions(ctx context.Context, tenantID string) ([]domain.RubricVersion, error) {
	rows, err := r.q.Query(ctx, `
		SELECT tenant_id, id, version_number, name, dimensions, weights,
		       algorithm_version, is_active, created_at
		FROM rubric_versions
		WHERE tenant_id=$1
		ORDER BY version_number DESC, id ASC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []domain.RubricVersion
	for rows.Next() {
		version, err := scanRubricVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, *version)
	}
	return versions, rows.Err()
}

var _ application.RubricRepository = (*rubricRepository)(nil)

// NewRubricRepositoryFromTx returns a rubric repository bound to an existing pgx transaction.
func NewRubricRepositoryFromTx(tx pgx.Tx, now func(context.Context) (time.Time, error)) application.RubricRepository {
	return &rubricRepository{q: tx, now: now}
}

type rubricScanner interface {
	Scan(...any) error
}

func scanRubricVersion(row rubricScanner) (*domain.RubricVersion, error) {
	var version domain.RubricVersion
	var dimensions, weights []byte
	if err := row.Scan(
		&version.TenantID, &version.ID, &version.VersionNumber, &version.Name,
		&dimensions, &weights, &version.AlgorithmVersion, &version.IsActive,
		&version.CreatedAt,
	); err != nil {
		return nil, err
	}
	return scanRubricVersionData(&version, dimensions, weights)
}

func scanRubricVersionData(version *domain.RubricVersion, dimensions, weights []byte) (*domain.RubricVersion, error) {
	if len(dimensions) > 0 {
		if err := json.Unmarshal(dimensions, &version.Dimensions); err != nil {
			return nil, err
		}
	}
	if len(weights) > 0 {
		if err := json.Unmarshal(weights, &version.Weights); err != nil {
			return nil, err
		}
	}
	return version, nil
}

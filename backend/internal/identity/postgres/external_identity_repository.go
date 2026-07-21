package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type externalIdentityRepository struct {
	q queryer
}

func NewExternalIdentityRepository(pool *pgxpool.Pool) application.ExternalIdentityRepository {
	return &externalIdentityRepository{q: pool}
}

func (r *externalIdentityRepository) Insert(ctx context.Context, identity *domain.ExternalIdentity) error {
	_, err := r.q.Exec(ctx, `
		INSERT INTO agent_external_identities (
			id, agent_id, provider, provider_subject_id, login, status,
			verified_at, created_at, updated_at, revoked_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		identity.ID, identity.AgentID, identity.Provider, identity.ProviderSubjectID,
		identity.Login, identity.Status, identity.VerifiedAt, identity.CreatedAt,
		identity.UpdatedAt, identity.RevokedAt,
	)
	return externalIdentityWriteError(err)
}

func (r *externalIdentityRepository) GetByProviderSubject(ctx context.Context, provider domain.ExternalIdentityProvider, providerSubjectID string) (*domain.ExternalIdentity, error) {
	identity, err := scanExternalIdentity(r.q.QueryRow(ctx, `
		SELECT id, agent_id, provider, provider_subject_id, login, status,
		       verified_at, created_at, updated_at, revoked_at
		FROM agent_external_identities
		WHERE provider=$1 AND provider_subject_id=$2`, provider, providerSubjectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return identity, err
}

func (r *externalIdentityRepository) ListByAgent(ctx context.Context, agentID string) ([]domain.ExternalIdentity, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, agent_id, provider, provider_subject_id, login, status,
		       verified_at, created_at, updated_at, revoked_at
		FROM agent_external_identities
		WHERE agent_id=$1
		ORDER BY provider, provider_subject_id`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var identities []domain.ExternalIdentity
	for rows.Next() {
		identity, err := scanExternalIdentity(rows)
		if err != nil {
			return nil, err
		}
		identities = append(identities, *identity)
	}
	return identities, rows.Err()
}

func (r *externalIdentityRepository) Update(ctx context.Context, identity *domain.ExternalIdentity) error {
	tag, err := r.q.Exec(ctx, `
		UPDATE agent_external_identities
		SET login=$3, status=$4, verified_at=$5, updated_at=$6, revoked_at=$7
		WHERE id=$1 AND agent_id=$2`,
		identity.ID, identity.AgentID, identity.Login, identity.Status,
		identity.VerifiedAt, identity.UpdatedAt, identity.RevokedAt,
	)
	if err != nil {
		return externalIdentityWriteError(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

type externalIdentityScanner interface {
	Scan(...any) error
}

func scanExternalIdentity(row externalIdentityScanner) (*domain.ExternalIdentity, error) {
	var identity domain.ExternalIdentity
	err := row.Scan(
		&identity.ID, &identity.AgentID, &identity.Provider, &identity.ProviderSubjectID,
		&identity.Login, &identity.Status, &identity.VerifiedAt, &identity.CreatedAt,
		&identity.UpdatedAt, &identity.RevokedAt,
	)
	return &identity, err
}

func externalIdentityWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrStateConflict
	}
	return err
}

var _ application.ExternalIdentityRepository = (*externalIdentityRepository)(nil)

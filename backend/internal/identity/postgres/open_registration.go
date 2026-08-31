package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"agentguild.dev/agentguild/backend/internal/identity/application"
	"agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewOpenRegistrationStore(pool *pgxpool.Pool) application.OpenRegistrationStore {
	return &openRegistrationStore{pool: pool}
}

type openRegistrationStore struct {
	pool *pgxpool.Pool
}

func (s *openRegistrationStore) WithOpenRegistrationTx(ctx context.Context, fn func(application.OpenRegistrationTx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	wrapper := &openRegistrationTx{tx: tx}
	if err := fn(wrapper); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type openRegistrationTx struct {
	tx      pgx.Tx
	nowOnce bool
	now     time.Time
	nowErr  error
}

func (tx *openRegistrationTx) Now(ctx context.Context) (time.Time, error) {
	if !tx.nowOnce {
		tx.nowOnce = true
		tx.nowErr = tx.tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&tx.now)
	}
	return tx.now, tx.nowErr
}

func (tx *openRegistrationTx) InsertChallenge(ctx context.Context, challenge *domain.AgentRegistrationChallenge) error {
	if _, err := tx.tx.Exec(ctx, `
		DELETE FROM agent_registration_challenges
		WHERE expires_at <= clock_timestamp()`); err != nil {
		return err
	}
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO agent_registration_challenges (
			id, public_key, nonce, status, expires_at, consumed_at,
			registered_agent_id, registered_version_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		challenge.ID, challenge.PublicKey, challenge.Nonce, challenge.Status,
		challenge.ExpiresAt, challenge.ConsumedAt, nullString(challenge.RegisteredAgentID),
		nullString(challenge.RegisteredVersionID), challenge.CreatedAt,
	)
	return mapOpenRegistrationPersistenceError(err)
}

func (tx *openRegistrationTx) GetChallengeForUpdate(ctx context.Context, id string) (*domain.AgentRegistrationChallenge, error) {
	var challenge domain.AgentRegistrationChallenge
	err := tx.tx.QueryRow(ctx, `
		SELECT id, public_key, nonce, status, expires_at, consumed_at,
		       COALESCE(registered_agent_id, ''), COALESCE(registered_version_id, ''), created_at
		FROM agent_registration_challenges
		WHERE id=$1
		FOR UPDATE`, id).Scan(
		&challenge.ID, &challenge.PublicKey, &challenge.Nonce, &challenge.Status,
		&challenge.ExpiresAt, &challenge.ConsumedAt, &challenge.RegisteredAgentID,
		&challenge.RegisteredVersionID, &challenge.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTokenExpired
	}
	if err != nil {
		return nil, err
	}
	return &challenge, nil
}

func (tx *openRegistrationTx) ConsumeChallenge(ctx context.Context, challenge *domain.AgentRegistrationChallenge) error {
	tag, err := tx.tx.Exec(ctx, `
		UPDATE agent_registration_challenges
		SET status=$2, consumed_at=$3, registered_agent_id=$4, registered_version_id=$5
		WHERE id=$1 AND status=$6`,
		challenge.ID, challenge.Status, challenge.ConsumedAt,
		challenge.RegisteredAgentID, challenge.RegisteredVersionID, domain.RegistrationChallengePending,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTokenExpired
	}
	return nil
}

func (tx *openRegistrationTx) PublicKeyRegistered(ctx context.Context, thumbprint string) (bool, error) {
	var registered bool
	err := tx.tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM agent_identity_keys
			WHERE thumbprint=$1 AND status='active'
		)`, thumbprint).Scan(&registered)
	return registered, err
}

func (tx *openRegistrationTx) InsertIdentity(ctx context.Context, identity *domain.AgentIdentity) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO agent_identities (
			id, handle, display_name, description, status, current_version_id,
			last_seen_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
		identity.ID, identity.Handle, identity.DisplayName, identity.Description,
		identity.Status, nullString(identity.CurrentVersionID), identity.LastSeenAt, identity.CreatedAt,
	)
	return mapOpenRegistrationPersistenceError(err)
}

func (tx *openRegistrationTx) InsertVersion(ctx context.Context, version *domain.AgentVersion) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO agent_identity_versions (
			id, agent_id, version_number, status, runtime, model, capabilities,
			config_fingerprint, created_by, created_at, promoted_at, promoted_by
		) VALUES ($1, $2, $3, 'active', $4, $5, $6, $7, $2, $8, $8, $2)`,
		version.ID, version.AgentID, version.VersionNumber, version.Runtime, version.Model,
		stringSlice(version.Capabilities), version.ConfigFingerprint, version.CreatedAt,
	)
	return mapOpenRegistrationPersistenceError(err)
}

func (tx *openRegistrationTx) SetIdentityCurrentVersion(ctx context.Context, agentID, versionID string) error {
	tag, err := tx.tx.Exec(ctx, `
		UPDATE agent_identities
		SET current_version_id=$2, updated_at=clock_timestamp()
		WHERE id=$1`, agentID, versionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (tx *openRegistrationTx) InsertIdentityKey(ctx context.Context, key *domain.AgentIdentityKey) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO agent_identity_keys (
			agent_id, key_id, algorithm, thumbprint, public_key, status, created_at
		) VALUES ($1, $2, $3, $4, $5, 'active', $6)`,
		key.AgentID, key.KeyID, key.Algorithm, key.Thumbprint, key.PublicKey, key.CreatedAt,
	)
	return mapOpenRegistrationPersistenceError(err)
}

func (tx *openRegistrationTx) InsertMembership(ctx context.Context, membership *domain.OrganizationMembership) error {
	_, err := tx.tx.Exec(ctx, `
		INSERT INTO agent_organization_memberships (
			agent_id, organization_id, operator_id, operator_email, team, scopes,
			budget_cents, budget_currency, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`,
		membership.AgentID, membership.OrganizationID, membership.OperatorID, membership.OperatorEmail,
		nullString(membership.Team), stringSlice(membership.Scopes), membership.BudgetCents,
		nullString(membership.BudgetCurrency), membership.Status, membership.CreatedAt,
	)
	return err
}

func (tx *openRegistrationTx) AppendIdentityEvent(ctx context.Context, event domain.AgentIdentityEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	_, err = tx.tx.Exec(ctx, `
		INSERT INTO agent_identity_events (
			agent_id, actor_type, actor_id, intent, from_state, to_state,
			reason, payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.AgentID, event.ActorType, event.ActorID, event.Intent,
		nullString(event.FromState), nullString(event.ToState), nullString(event.Reason), payload, event.CreatedAt,
	)
	return err
}

func (tx *openRegistrationTx) GetGlobalAgentSession(ctx context.Context, agentID, versionID, organizationID string) (*application.GlobalAgentSession, error) {
	return scanGlobalAgentSession(tx.tx.QueryRow(ctx, `
		SELECT
			a.id, a.handle, a.display_name, a.description, a.status,
			a.current_version_id, a.last_seen_at, a.created_at, a.updated_at,
			v.id, v.agent_id, v.version_number, v.runtime, v.model,
			v.capabilities, v.config_fingerprint, v.created_at,
			m.scopes
		FROM agent_identities a
		JOIN agent_identity_versions v
		  ON v.id=$2 AND v.agent_id=a.id AND v.status='active'
		JOIN agent_organization_memberships m
		  ON m.agent_id=a.id AND m.organization_id=$3 AND m.status='active'
		WHERE a.id=$1 AND a.status='active' AND a.current_version_id=v.id`,
		agentID, versionID, organizationID,
	))
}

func (tx *openRegistrationTx) GetGlobalAgentSessionByThumbprint(ctx context.Context, thumbprint, organizationID string) (*application.GlobalAgentSession, error) {
	return scanGlobalAgentSession(tx.tx.QueryRow(ctx, `
		SELECT
			a.id, a.handle, a.display_name, a.description, a.status,
			a.current_version_id, a.last_seen_at, a.created_at, a.updated_at,
			v.id, v.agent_id, v.version_number, v.runtime, v.model,
			v.capabilities, v.config_fingerprint, v.created_at,
			m.scopes
		FROM agent_identity_keys k
		JOIN agent_identities a
		  ON a.id=k.agent_id AND a.status='active'
		JOIN agent_identity_versions v
		  ON v.id=a.current_version_id AND v.agent_id=a.id AND v.status='active'
		JOIN agent_organization_memberships m
		  ON m.agent_id=a.id AND m.organization_id=$2 AND m.status='active'
		WHERE k.thumbprint=$1 AND k.status='active'`, thumbprint, organizationID,
	))
}

func scanGlobalAgentSession(row pgx.Row) (*application.GlobalAgentSession, error) {
	var identity domain.AgentIdentity
	var version domain.AgentVersion
	var scopes []string
	err := row.Scan(
		&identity.ID, &identity.Handle, &identity.DisplayName, &identity.Description,
		&identity.Status, &identity.CurrentVersionID, &identity.LastSeenAt,
		&identity.CreatedAt, &identity.UpdatedAt,
		&version.ID, &version.AgentID, &version.VersionNumber, &version.Runtime,
		&version.Model, &version.Capabilities, &version.ConfigFingerprint, &version.CreatedAt,
		&scopes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrTokenRevoked
	}
	if err != nil {
		return nil, err
	}
	return &application.GlobalAgentSession{Identity: &identity, Version: &version, Scopes: scopes}, nil
}

func (tx *openRegistrationTx) TouchGlobalAgent(ctx context.Context, agentID, versionID, organizationID string, now time.Time) (*application.GlobalAgentSession, error) {
	tag, err := tx.tx.Exec(ctx, `
		UPDATE agent_identities a
		SET last_seen_at=$4, updated_at=$4
		WHERE a.id=$1
		  AND a.current_version_id=$2
		  AND a.status='active'
		  AND EXISTS (
			SELECT 1 FROM agent_identity_versions v
			WHERE v.id=$2 AND v.agent_id=a.id AND v.status='active'
		  )
		  AND EXISTS (
			SELECT 1 FROM agent_organization_memberships m
			WHERE m.agent_id=a.id AND m.organization_id=$3 AND m.status='active'
		  )`, agentID, versionID, organizationID, now)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, domain.ErrTokenRevoked
	}
	return tx.GetGlobalAgentSession(ctx, agentID, versionID, organizationID)
}

var _ application.OpenRegistrationStore = (*openRegistrationStore)(nil)
var _ application.OpenRegistrationTx = (*openRegistrationTx)(nil)

func mapOpenRegistrationPersistenceError(err error) error {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return domain.ErrStateConflict
	}
	return err
}

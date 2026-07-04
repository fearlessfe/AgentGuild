package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/auth"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5"
)

func (tx *Tx) RequireLiveAgent(ctx context.Context, principal auth.Principal) error {
	var status string
	var currentVersionID string
	err := tx.tx.QueryRow(ctx, `
		SELECT status, COALESCE(current_version_id, '')
		FROM agents
		WHERE tenant_id=$1 AND id=$2
		FOR UPDATE`,
		principal.TenantID, principal.AgentID,
	).Scan(&status, &currentVersionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return identitydomain.ErrForbidden
	}
	if err != nil {
		return err
	}
	if currentVersionID != "" && principal.AgentVersionID != currentVersionID {
		return identitydomain.ErrForbidden
	}
	switch status {
	case identitydomain.AgentActive:
		return nil
	case identitydomain.AgentRevoked:
		return identitydomain.ErrTokenRevoked
	default:
		return identitydomain.ErrStateConflict
	}
}

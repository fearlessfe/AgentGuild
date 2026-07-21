package postgres

import (
	"context"
	"errors"

	"agentguild.dev/agentguild/backend/internal/auth"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
	"github.com/jackc/pgx/v5"
)

func (tx *Tx) RequireLiveAgent(ctx context.Context, principal auth.Principal) error {
	if principal.IsGlobalAgent() {
		return tx.requireLiveGlobalAgent(ctx, principal)
	}
	if _, err := (auth.ResourcePolicy{}).Tenant(principal); err != nil {
		return identitydomain.ErrForbidden
	}
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

func (tx *Tx) requireLiveGlobalAgent(ctx context.Context, principal auth.Principal) error {
	var agentStatus, versionStatus string
	err := tx.tx.QueryRow(ctx, `
		SELECT a.status, v.status
		FROM agent_identities a
		JOIN agent_identity_versions v ON v.agent_id=a.id
		WHERE a.id=$1 AND v.id=$2
		FOR UPDATE OF a, v`,
		principal.AgentID, principal.AgentVersionID,
	).Scan(&agentStatus, &versionStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return identitydomain.ErrForbidden
	}
	if err != nil {
		return err
	}
	if agentStatus == string(identitydomain.AgentRevoked) {
		return identitydomain.ErrTokenRevoked
	}
	if agentStatus != string(identitydomain.AgentActive) || versionStatus != "active" {
		return identitydomain.ErrStateConflict
	}
	return nil
}

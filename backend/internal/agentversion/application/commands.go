package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// NewVersionService creates the version lifecycle service.
func NewVersionService(
	store Store,
	versions VersionRepository,
	evalProvider EvaluationRunProvider,
	policy *Policy,
	options VersionOptions,
) (*VersionService, error) {
	if store == nil {
		return nil, invalidArgument("store")
	}
	if versions == nil {
		return nil, invalidArgument("versions")
	}
	if policy == nil {
		return nil, invalidArgument("policy")
	}
	if evalProvider == nil {
		return nil, invalidArgument("eval_provider")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	return &VersionService{
		store:        store,
		versions:     versions,
		evalProvider: evalProvider,
		policy:       policy,
		newID:        options.NewID,
	}, nil
}

// CreateDraft creates a new draft version from the current active version.
// If the configuration is unchanged, it returns domain.ErrNoChange.
func (s *VersionService) CreateDraft(
	ctx context.Context,
	cmd CreateDraft,
) (*CreateDraftResponse, error) {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.CreatedBy,
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return nil, err
	}

	cfg := domain.DraftConfig{
		Runtime:           cmd.Runtime,
		Model:             cmd.Model,
		Capabilities:      cmd.Capabilities,
		PromptRef:         cmd.PromptRef,
		SkillRefs:         cmd.SkillRefs,
		MemoryRef:         cmd.MemoryRef,
		ToolRefs:          cmd.ToolRefs,
		EnvironmentDigest: cmd.EnvironmentDigest,
		CreatedBy:         cmd.CreatedBy,
	}

	var version *domain.AgentVersion
	if err := s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		active, err := s.versions.GetActiveByAgent(ctx, cmd.TenantID, cmd.AgentID)
		if err != nil && !isNotFound(err) {
			return err
		}
		if active != nil {
			version, err = domain.NewDraftFromCurrent(active, cfg, s.newID, now)
			if err != nil {
				return err
			}
		} else {
			version, err = domain.NewAgentVersion(
				s.newID(), cmd.TenantID, cmd.AgentID, 1, "",
				cfg.Runtime, cfg.Model, cfg.Capabilities,
				cfg.PromptRef, cfg.SkillRefs, cfg.MemoryRef, cfg.ToolRefs,
				cfg.EnvironmentDigest, cfg.CreatedBy, now,
			)
			if err != nil {
				return err
			}
		}
		return s.versions.Create(ctx, tx, version)
	}); err != nil {
		return nil, err
	}

	return &CreateDraftResponse{Version: version}, nil
}

// StartEvaluation transitions a draft version to evaluating.
func (s *VersionService) StartEvaluation(
	ctx context.Context,
	principal identityapp.Principal,
	cmd StartEvaluation,
) error {
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx Tx) error {
		version, err := s.versions.GetByID(ctx, cmd.TenantID, cmd.AgentID, cmd.VersionID)
		if err != nil {
			return err
		}
		if err := version.StartEvaluation(); err != nil {
			return err
		}
		return s.versions.UpdateStatus(ctx, tx, version)
	})
}

// Promote transitions an eligible version to active and retires the previous
// active version atomically.
func (s *VersionService) Promote(
	ctx context.Context,
	principal identityapp.Principal,
	cmd Promote,
) error {
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx Tx) error {
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		if err := s.versions.LockAgent(ctx, tx, cmd.TenantID, cmd.AgentID); err != nil {
			return err
		}
		target, err := s.versions.GetByID(ctx, cmd.TenantID, cmd.AgentID, cmd.VersionID)
		if err != nil {
			return err
		}
		if target.Status != domain.StatusEligible {
			return domain.ErrStateConflict
		}
		run, err := s.evalProvider.GetLatestPassed(ctx, tx, cmd.TenantID, cmd.VersionID)
		if err != nil {
			return err
		}
		if run == nil {
			return domain.ErrStateConflict
		}
		active, err := s.versions.GetActiveByAgent(ctx, cmd.TenantID, cmd.AgentID)
		if err != nil && !isNotFound(err) {
			return err
		}
		if active != nil {
			if err := active.Retire(now); err != nil {
				return err
			}
			if err := s.versions.UpdateStatus(ctx, tx, active); err != nil {
				return err
			}
		}
		if err := target.Promote(now); err != nil {
			return err
		}
		if err := s.versions.UpdateStatus(ctx, tx, target); err != nil {
			return err
		}
		return s.versions.UpdateAgentCurrentVersion(ctx, tx, cmd.TenantID, cmd.AgentID, target.ID)
	})
}

// Rollback switches the agent's current version to a historical
// active/eligible/retired version.
func (s *VersionService) Rollback(
	ctx context.Context,
	principal identityapp.Principal,
	cmd Rollback,
) error {
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return err
	}
	return s.store.WithTx(ctx, func(tx Tx) error {
		if err := s.versions.LockAgent(ctx, tx, cmd.TenantID, cmd.AgentID); err != nil {
			return err
		}
		target, err := s.versions.GetByID(ctx, cmd.TenantID, cmd.AgentID, cmd.VersionID)
		if err != nil {
			return err
		}
		if !domain.CanRollbackTo(target.Status) {
			return domain.ErrStateConflict
		}
		return s.versions.UpdateAgentCurrentVersion(ctx, tx, cmd.TenantID, cmd.AgentID, target.ID)
	})
}

func isNotFound(err error) bool {
	return domain.CodeOf(err) == "not_found"
}

func invalidArgument(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

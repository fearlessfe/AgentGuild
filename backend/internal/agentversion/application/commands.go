package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sort"

	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
	identityapp "agentguild.dev/agentguild/backend/internal/identity/application"
)

// NewVersionService creates the version lifecycle service.
func NewVersionService(
	store Store,
	versions VersionRepository,
	evalProvider EvaluationRunProvider,
	xpProvider ExperienceCandidateProvider,
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
		store:            store,
		versions:         versions,
		evalProvider:     evalProvider,
		xpProvider:       xpProvider,
		draftCreatedHook: options.DraftCreatedHook,
		policy:           policy,
		newID:            options.NewID,
	}, nil
}

// CreateDraft creates a new draft version from the latest version for the agent.
// If the configuration is unchanged, it returns domain.ErrNoChange.
// Approved experience candidates may be bound to the draft by including their
// IDs; their evidence references are merged into the new version's memory_ref.
func (s *VersionService) CreateDraft(
	ctx context.Context,
	cmd CreateDraft,
) (*CreateDraftResponse, error) {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.CreatedBy,
		IsAdmin:  cmd.IsAdmin,
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return nil, err
	}

	if len(cmd.ApprovedExperienceIDs) > 0 && s.xpProvider == nil {
		return nil, invalidArgument("approved_experience_ids")
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

		memoryRef := cfg.MemoryRef
		if len(cmd.ApprovedExperienceIDs) > 0 {
			approved, err := s.xpProvider.ListApprovedByAgentTx(ctx, tx, cmd.TenantID, cmd.AgentID)
			if err != nil {
				return err
			}
			selected, err := filterApprovedExperiences(cmd.ApprovedExperienceIDs, approved)
			if err != nil {
				return err
			}
			memoryRef = mergeExperienceRefs(memoryRef, selected)
		}
		cfg.MemoryRef = memoryRef

		latest, err := s.versions.GetLatestByAgent(ctx, cmd.TenantID, cmd.AgentID)
		if err != nil && !isNotFound(err) {
			return err
		}
		if latest != nil {
			version, err = domain.NewDraftFromCurrent(latest, cfg, s.newID, now)
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

	// Best-effort post-commit hook (EVALUATION_AUTO): a hook failure must never
	// fail draft creation — the draft is already committed.
	if s.draftCreatedHook != nil {
		if err := s.draftCreatedHook.OnDraftCreated(ctx, version.TenantID, version.AgentID, version.ID, cmd.CreatedBy); err != nil {
			slog.Warn("draft created hook failed",
				"tenant_id", version.TenantID, "agent_id", version.AgentID,
				"version_id", version.ID, "error", err)
		}
	}

	return &CreateDraftResponse{Version: version}, nil
}

// filterApprovedExperiences returns only the approved candidates whose IDs are
// requested. It returns an error if any requested ID is missing.
func filterApprovedExperiences(requested []string, approved []ExperienceCandidateRef) ([]ExperienceCandidateRef, error) {
	approvedByID := make(map[string]ExperienceCandidateRef, len(approved))
	for _, ref := range approved {
		approvedByID[ref.ID] = ref
	}
	seen := make(map[string]bool)
	var selected []ExperienceCandidateRef
	for _, id := range requested {
		if seen[id] {
			continue
		}
		seen[id] = true
		ref, ok := approvedByID[id]
		if !ok {
			return nil, &domain.Error{Code: "invalid_argument", Message: "experience candidate is not approved or does not belong to agent", Field: "approved_experience_ids"}
		}
		selected = append(selected, ref)
	}
	return selected, nil
}

// mergeExperienceRefs combines the base memory reference with approved candidate
// evidence references into a deterministic JSON array used as the new memory_ref.
func mergeExperienceRefs(baseMemory string, approved []ExperienceCandidateRef) string {
	refs := make([]string, 0, len(approved)+1)
	if baseMemory != "" {
		refs = append(refs, baseMemory)
	}
	for _, ref := range approved {
		if ref.EvidenceRef != "" {
			refs = append(refs, ref.EvidenceRef)
		}
	}
	sort.Strings(refs)
	payload, _ := json.Marshal(refs)
	return string(payload)
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
// active version atomically. The agent's current version observed when Promote
// starts is used as an optimistic guard: if a concurrent promote (or rollback)
// changed it before this transaction commits, Promote fails with
// domain.ErrStateConflict instead of overwriting it.
func (s *VersionService) Promote(
	ctx context.Context,
	cmd Promote,
) error {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.ActorID,
		IsAdmin:  cmd.IsAdmin,
	}
	if err := s.policy.RequireOwnerOrAdmin(ctx, principal, cmd.TenantID, cmd.AgentID); err != nil {
		return err
	}
	expectedCurrent, err := s.versions.GetAgentCurrentVersionID(ctx, cmd.TenantID, cmd.AgentID)
	if err != nil {
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
		if err := target.Promote(now, cmd.ActorID); err != nil {
			return err
		}
		if err := s.versions.UpdateStatus(ctx, tx, target); err != nil {
			return err
		}
		return s.versions.PromoteAgentCurrentVersion(ctx, tx, cmd.TenantID, cmd.AgentID, target.ID, expectedCurrent)
	})
}

// Rollback switches the agent's current version to a historical
// active/eligible/retired version.
func (s *VersionService) Rollback(
	ctx context.Context,
	cmd Rollback,
) error {
	principal := identityapp.Principal{
		TenantID: cmd.TenantID,
		OwnerID:  cmd.ActorID,
		IsAdmin:  cmd.IsAdmin,
	}
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

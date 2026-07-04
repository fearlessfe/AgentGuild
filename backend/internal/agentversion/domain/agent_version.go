package domain

import (
	"time"
)

// AgentVersion is an immutable snapshot of an Agent's configuration at a point
// in time. Once persisted, its content fields must never change.
type AgentVersion struct {
	ID                string
	TenantID          string
	AgentID           string
	VersionNumber     int
	ParentVersionID   string
	// Status is the current lifecycle status. It is exported solely so
	// repositories can scan it from the database; callers must use the
	// state-machine methods (StartEvaluation, MarkEligible, MarkRejected,
	// Promote, Retire) and must never assign to this field directly.
	Status            VersionStatus
	Runtime           string
	Model             string
	Capabilities      []string
	ConfigFingerprint string
	ContentHash       string
	EnvironmentDigest string
	PromptRef         string
	SkillRefs         []string
	MemoryRef         string
	ToolRefs          []string
	CreatedBy         string
	CreatedAt         time.Time
	PromotedAt        *time.Time
	RetiredAt         *time.Time
	RejectedReason    string

	// Persisted is true after the version has been written to the store.
	// It is exported so the repository can mark the version as persisted.
	Persisted bool
}

// DraftConfig contains the mutable configuration references used when creating
// a new draft from an existing version.
type DraftConfig struct {
	Runtime           string
	Model             string
	Capabilities      []string
	PromptRef         string
	SkillRefs         []string
	MemoryRef         string
	ToolRefs          []string
	EnvironmentDigest string
	CreatedBy         string
}

// NewAgentVersion creates a new draft version. It computes the content hash and
// config fingerprint from the supplied references.
func NewAgentVersion(
	id, tenantID, agentID string,
	versionNumber int,
	parentVersionID string,
	runtime, model string,
	capabilities []string,
	promptRef string,
	skillRefs []string,
	memoryRef string,
	toolRefs []string,
	environmentDigest, createdBy string,
	now time.Time,
) (*AgentVersion, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if agentID == "" {
		return nil, invalidArgument("agent_id")
	}
	if versionNumber < 1 {
		return nil, invalidArgument("version_number")
	}
	if runtime == "" {
		return nil, invalidArgument("runtime")
	}
	if model == "" {
		return nil, invalidArgument("model")
	}
	if promptRef == "" {
		return nil, invalidArgument("prompt_ref")
	}

	contentHash := ComputeContentHash(runtime, model, capabilities, promptRef, skillRefs, memoryRef, toolRefs)
	fingerprint := ComputeConfigFingerprint(runtime, model, capabilities, promptRef, skillRefs, memoryRef, toolRefs)

	return &AgentVersion{
		ID:                id,
		TenantID:          tenantID,
		AgentID:           agentID,
		VersionNumber:     versionNumber,
		ParentVersionID:   parentVersionID,
		Status:            StatusDraft,
		Runtime:           runtime,
		Model:             model,
		Capabilities:      append([]string(nil), capabilities...),
		ConfigFingerprint: fingerprint,
		ContentHash:       contentHash,
		EnvironmentDigest: environmentDigest,
		PromptRef:         promptRef,
		SkillRefs:         append([]string(nil), skillRefs...),
		MemoryRef:         memoryRef,
		ToolRefs:          append([]string(nil), toolRefs...),
		CreatedBy:         createdBy,
		CreatedAt:         now,
		Persisted:         false,
	}, nil
}

// NewDraftFromCurrent creates a draft from the current active/eligible version.
// If the configuration fingerprint is unchanged, it returns ErrNoChange.
func NewDraftFromCurrent(
	current *AgentVersion,
	cfg DraftConfig,
	newID func() string,
	now time.Time,
) (*AgentVersion, error) {
	if current == nil {
		return nil, invalidArgument("current")
	}
	if current.AgentID == "" {
		return nil, invalidArgument("agent_id")
	}
	fingerprint := ComputeConfigFingerprint(
		cfg.Runtime, cfg.Model, cfg.Capabilities,
		cfg.PromptRef, cfg.SkillRefs, cfg.MemoryRef, cfg.ToolRefs,
	)
	if fingerprint == current.ConfigFingerprint {
		return nil, ErrNoChange
	}
	return NewAgentVersion(
		newID(), current.TenantID, current.AgentID,
		current.VersionNumber+1, current.ID,
		cfg.Runtime, cfg.Model, cfg.Capabilities,
		cfg.PromptRef, cfg.SkillRefs, cfg.MemoryRef, cfg.ToolRefs,
		cfg.EnvironmentDigest, cfg.CreatedBy, now,
	)
}

// Now returns the current time. It is exposed for test convenience.
func Now() time.Time { return time.Now() }

// Status returns the current lifecycle status.
func (v *AgentVersion) StatusValue() VersionStatus { return v.Status }

// StartEvaluation transitions draft -> evaluating.
func (v *AgentVersion) StartEvaluation() error {
	if v.Status != StatusDraft {
		return ErrStateConflict
	}
	v.Status = StatusEvaluating
	return nil
}

// MarkEligible transitions evaluating -> eligible.
func (v *AgentVersion) MarkEligible() error {
	if v.Status != StatusEvaluating {
		return ErrStateConflict
	}
	v.Status = StatusEligible
	return nil
}

// MarkRejected transitions evaluating -> rejected.
func (v *AgentVersion) MarkRejected(reason string) error {
	if v.Status != StatusEvaluating {
		return ErrStateConflict
	}
	v.Status = StatusRejected
	v.RejectedReason = reason
	return nil
}

// Promote transitions eligible -> active.
func (v *AgentVersion) Promote(now time.Time) error {
	if v.Status != StatusEligible {
		return ErrStateConflict
	}
	v.Status = StatusActive
	v.PromotedAt = &now
	return nil
}

// Retire transitions active -> retired.
func (v *AgentVersion) Retire(now time.Time) error {
	if v.Status != StatusActive {
		return ErrStateConflict
	}
	v.Status = StatusRetired
	v.RetiredAt = &now
	return nil
}

func (v *AgentVersion) ensureMutableDraft() error {
	if v.Persisted {
		return ErrImmutableResource
	}
	if v.Status != StatusDraft {
		return ErrImmutableResource
	}
	return nil
}

// UpdatePromptRef updates the prompt reference before persistence.
func (v *AgentVersion) UpdatePromptRef(ref string) error {
	if err := v.ensureMutableDraft(); err != nil {
		return err
	}
	if ref == "" {
		return invalidArgument("prompt_ref")
	}
	v.PromptRef = ref
	v.recomputeHashes()
	return nil
}

// UpdateSkillRefs updates the skill references before persistence.
func (v *AgentVersion) UpdateSkillRefs(refs []string) error {
	if err := v.ensureMutableDraft(); err != nil {
		return err
	}
	v.SkillRefs = append([]string(nil), refs...)
	v.recomputeHashes()
	return nil
}

// UpdateMemoryRef updates the memory reference before persistence.
func (v *AgentVersion) UpdateMemoryRef(ref string) error {
	if err := v.ensureMutableDraft(); err != nil {
		return err
	}
	v.MemoryRef = ref
	v.recomputeHashes()
	return nil
}

// UpdateToolRefs updates the tool references before persistence.
func (v *AgentVersion) UpdateToolRefs(refs []string) error {
	if err := v.ensureMutableDraft(); err != nil {
		return err
	}
	v.ToolRefs = append([]string(nil), refs...)
	v.recomputeHashes()
	return nil
}

func (v *AgentVersion) recomputeHashes() {
	v.ContentHash = ComputeContentHash(
		v.Runtime, v.Model, v.Capabilities,
		v.PromptRef, v.SkillRefs, v.MemoryRef, v.ToolRefs,
	)
	v.ConfigFingerprint = ComputeConfigFingerprint(
		v.Runtime, v.Model, v.Capabilities,
		v.PromptRef, v.SkillRefs, v.MemoryRef, v.ToolRefs,
	)
}

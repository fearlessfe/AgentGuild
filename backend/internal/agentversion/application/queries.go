package application

import (
	"context"

	"agentguild.dev/agentguild/backend/internal/agentversion/domain"
)

// ListVersions returns a summary of all versions for an agent.
func (s *VersionService) ListVersions(ctx context.Context, tenantID, agentID string) ([]VersionSummary, error) {
	versions, err := s.versions.ListByAgent(ctx, tenantID, agentID)
	if err != nil {
		return nil, err
	}
	summaries := make([]VersionSummary, 0, len(versions))
	for _, v := range versions {
		summaries = append(summaries, VersionSummary{
			ID:                v.ID,
			VersionNumber:     v.VersionNumber,
			Status:            v.Status,
			ParentVersionID:   v.ParentVersionID,
			ConfigFingerprint: v.ConfigFingerprint,
			CreatedAt:         v.CreatedAt,
			PromotedAt:        v.PromotedAt,
			RetiredAt:         v.RetiredAt,
		})
	}
	return summaries, nil
}

// GetVersion returns the full detail of a specific version.
func (s *VersionService) GetVersion(ctx context.Context, tenantID, agentID, versionID string) (*VersionDetail, error) {
	v, err := s.versions.GetByID(ctx, tenantID, agentID, versionID)
	if err != nil {
		return nil, err
	}
	return versionDetail(v), nil
}

// GetVersionDiff returns the content reference differences between a version and
// a base version. If baseVersionID is empty, the version's parent is used when
// available; otherwise the current active version is used.
func (s *VersionService) GetVersionDiff(
	ctx context.Context,
	tenantID, agentID, versionID, baseVersionID string,
) (*VersionDiff, error) {
	target, err := s.versions.GetByID(ctx, tenantID, agentID, versionID)
	if err != nil {
		return nil, err
	}

	baseID := baseVersionID
	if baseID == "" {
		if target.ParentVersionID != "" {
			baseID = target.ParentVersionID
		} else {
			active, err := s.versions.GetActiveByAgent(ctx, tenantID, agentID)
			if err != nil {
				return nil, err
			}
			baseID = active.ID
		}
	}
	base, err := s.versions.GetByID(ctx, tenantID, agentID, baseID)
	if err != nil {
		return nil, err
	}

	return diffVersions(base, target), nil
}

func versionDetail(v *domain.AgentVersion) *VersionDetail {
	return &VersionDetail{
		ID:                v.ID,
		TenantID:          v.TenantID,
		AgentID:           v.AgentID,
		VersionNumber:     v.VersionNumber,
		ParentVersionID:   v.ParentVersionID,
		Status:            v.Status,
		Runtime:           v.Runtime,
		Model:             v.Model,
		Capabilities:      append([]string(nil), v.Capabilities...),
		ConfigFingerprint: v.ConfigFingerprint,
		ContentHash:       v.ContentHash,
		EnvironmentDigest: v.EnvironmentDigest,
		PromptRef:         v.PromptRef,
		SkillRefs:         append([]string(nil), v.SkillRefs...),
		MemoryRef:         v.MemoryRef,
		ToolRefs:          append([]string(nil), v.ToolRefs...),
		CreatedBy:         v.CreatedBy,
		CreatedAt:         v.CreatedAt,
		PromotedAt:        v.PromotedAt,
		PromotedBy:        v.PromotedBy,
		RetiredAt:         v.RetiredAt,
	}
}

func diffVersions(base, target *domain.AgentVersion) *VersionDiff {
	diff := &VersionDiff{
		BaseVersionID: base.ID,
		Added:         make(map[string]RefChange),
		Removed:       make(map[string]RefChange),
		Changed:       make(map[string]RefChange),
	}

	if target.Runtime != base.Runtime {
		diff.Changed["runtime"] = RefChange{From: base.Runtime, To: target.Runtime}
	}
	if target.Model != base.Model {
		diff.Changed["model"] = RefChange{From: base.Model, To: target.Model}
	}
	if target.EnvironmentDigest != base.EnvironmentDigest {
		diff.Changed["environment_digest"] = RefChange{From: base.EnvironmentDigest, To: target.EnvironmentDigest}
	}
	if target.PromptRef != base.PromptRef {
		diff.Changed["prompt_ref"] = RefChange{From: base.PromptRef, To: target.PromptRef}
	}
	if target.MemoryRef != base.MemoryRef {
		diff.Changed["memory_ref"] = RefChange{From: base.MemoryRef, To: target.MemoryRef}
	}

	baseCaps := toSet(base.Capabilities)
	targetCaps := toSet(target.Capabilities)
	for c := range targetCaps {
		if !baseCaps[c] {
			diff.Added["capability:"+c] = RefChange{From: "", To: c}
		}
	}
	for c := range baseCaps {
		if !targetCaps[c] {
			diff.Removed["capability:"+c] = RefChange{From: c, To: ""}
		}
	}

	diffArray("skill", base.SkillRefs, target.SkillRefs, diff)
	diffArray("tool", base.ToolRefs, target.ToolRefs, diff)

	return diff
}

func diffArray(kind string, base, target []string, diff *VersionDiff) {
	baseSet := toSet(base)
	targetSet := toSet(target)
	for ref := range targetSet {
		if !baseSet[ref] {
			diff.Added[kind+":"+ref] = RefChange{From: "", To: ref}
		}
	}
	for ref := range baseSet {
		if !targetSet[ref] {
			diff.Removed[kind+":"+ref] = RefChange{From: ref, To: ""}
		}
	}
}

func toSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

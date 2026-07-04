package domain

import (
	"testing"
	"time"
)

// NewTestAgentVersion returns a persisted test version in the requested state.
func NewTestAgentVersion(t testing.TB, status VersionStatus) *AgentVersion {
	t.Helper()
	return NewTestAgentVersionWithConfig(t, status, DraftConfig{
		Runtime:      "python",
		Model:        "gpt-4",
		Capabilities: []string{"code"},
		PromptRef:    "sha256:prompt",
		MemoryRef:    "sha256:memory",
		SkillRefs:    []string{"sha256:skill1"},
		ToolRefs:     []string{"sha256:tool1"},
	})
}

// NewTestAgentVersionWithConfig returns a persisted test version with a custom
// configuration.
func NewTestAgentVersionWithConfig(t testing.TB, status VersionStatus, cfg DraftConfig) *AgentVersion {
	t.Helper()
	now := time.Now()
	v, err := NewAgentVersion(
		"version-id", "tenant-id", "agent-id", 1, "",
		cfg.Runtime, cfg.Model, cfg.Capabilities,
		cfg.PromptRef, cfg.SkillRefs, cfg.MemoryRef, cfg.ToolRefs,
		"env-digest", "owner-id", now,
	)
	if err != nil {
		t.Fatalf("new test agent version: %v", err)
	}
	v.Persisted = true
	v.Status = status
	switch status {
	case StatusActive:
		v.PromotedAt = &now
	case StatusRetired:
		v.PromotedAt = &now
		retired := now.Add(time.Minute)
		v.RetiredAt = &retired
	case StatusRejected:
		v.RejectedReason = "test rejection"
	}
	return v
}

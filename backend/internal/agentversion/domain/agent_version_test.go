package domain_test

import (
	"testing"
	"time"

	avdomain "agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"github.com/stretchr/testify/require"
)

func TestAgentVersionStatusMachine(t *testing.T) {
	now := time.Now()
	v := avdomain.NewTestAgentVersion(t, avdomain.StatusDraft)

	require.NoError(t, v.StartEvaluation())
	require.Equal(t, avdomain.StatusEvaluating, v.StatusValue())
	require.ErrorIs(t, v.Promote(now, "owner"), avdomain.ErrStateConflict)

	require.NoError(t, v.MarkEligible())
	require.NoError(t, v.Promote(now, "owner"))
	require.Equal(t, avdomain.StatusActive, v.StatusValue())
	require.Equal(t, "owner", v.PromotedBy)
	require.NotNil(t, v.PromotedAt)
}

func TestAgentVersionContentImmutable(t *testing.T) {
	v := avdomain.NewTestAgentVersion(t, avdomain.StatusDraft)

	err := v.UpdatePromptRef("sha256:new")
	require.ErrorIs(t, err, avdomain.ErrImmutableResource)
}

func TestSameFingerprintRejected(t *testing.T) {
	cfg := avdomain.DraftConfig{
		Runtime:      "python",
		Model:        "gpt-4",
		Capabilities: []string{"code"},
		PromptRef:    "sha256:abc",
		MemoryRef:    "sha256:mem",
		SkillRefs:    []string{"sha256:s1"},
		ToolRefs:     []string{"sha256:t1"},
	}
	current := avdomain.NewTestAgentVersionWithConfig(t, avdomain.StatusActive, cfg)

	_, err := avdomain.NewDraftFromCurrent(current, cfg, func() string { return "v2" }, time.Now())
	require.ErrorIs(t, err, avdomain.ErrNoChange)
}

func TestDraftCanUpdateBeforePersist(t *testing.T) {
	now := time.Now()
	v, err := avdomain.NewAgentVersion(
		"v1", "t1", "a1", 1, "",
		"python", "gpt-4",
		[]string{"code"},
		"sha256:abc",
		[]string{"sha256:s1"},
		"sha256:mem",
		[]string{"sha256:t1"},
		"env1", "owner", now,
	)
	require.NoError(t, err)

	require.NoError(t, v.UpdatePromptRef("sha256:new"))
	require.Equal(t, "sha256:new", v.PromptRef)
}

func TestDraftCannotUpdateAfterPersist(t *testing.T) {
	v := avdomain.NewTestAgentVersion(t, avdomain.StatusDraft)

	require.ErrorIs(t, v.UpdatePromptRef("sha256:new"), avdomain.ErrImmutableResource)
	require.ErrorIs(t, v.UpdateSkillRefs([]string{"sha256:new"}), avdomain.ErrImmutableResource)
	require.ErrorIs(t, v.UpdateMemoryRef("sha256:new"), avdomain.ErrImmutableResource)
	require.ErrorIs(t, v.UpdateToolRefs([]string{"sha256:new"}), avdomain.ErrImmutableResource)
}

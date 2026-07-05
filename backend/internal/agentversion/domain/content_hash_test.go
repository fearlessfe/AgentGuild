package domain_test

import (
	"strings"
	"testing"

	avdomain "agentguild.dev/agentguild/backend/internal/agentversion/domain"
	"github.com/stretchr/testify/require"
)

func TestComputeContentHashStable(t *testing.T) {
	caps := []string{"b", "a"}
	skills := []string{"skill-b", "skill-a"}
	tools := []string{"tool-b", "tool-a"}

	h1 := avdomain.ComputeContentHash(
		"runtime", "model",
		caps,
		"sha256:prompt",
		skills,
		"sha256:memory",
		tools,
	)
	h2 := avdomain.ComputeContentHash(
		"runtime", "model",
		[]string{"a", "b"},
		"sha256:prompt",
		[]string{"skill-a", "skill-b"},
		"sha256:memory",
		[]string{"tool-a", "tool-b"},
	)

	require.Equal(t, h1, h2)
	require.True(t, strings.HasPrefix(h1, "sha256:"))
}

func TestComputeContentHashDifferent(t *testing.T) {
	h1 := avdomain.ComputeContentHash("r", "m", nil, "sha256:p", nil, "sha256:m", nil)
	h2 := avdomain.ComputeContentHash("r", "m", nil, "sha256:q", nil, "sha256:m", nil)

	require.NotEqual(t, h1, h2)
}

func TestComputeConfigFingerprintPrefix(t *testing.T) {
	hash := avdomain.ComputeContentHash("r", "m", nil, "sha256:p", nil, "sha256:m", nil)
	fp := avdomain.ComputeConfigFingerprint("r", "m", nil, "sha256:p", nil, "sha256:m", nil)

	require.Equal(t, hash[7:23], fp)
}

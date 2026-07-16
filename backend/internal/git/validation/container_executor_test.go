package validation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainerExecutorRequiresPinnedImage(t *testing.T) {
	for _, image := range []string{"", "agentguild-validation:latest", "agentguild-validation@sha256:short"} {
		_, err := NewContainerExecutor(image).Execute(context.Background(), t.TempDir(), nil, "true")
		require.ErrorContains(t, err, "pinned by sha256")
	}
	require.True(t, pinnedContainerImage("agentguild-validation@sha256:"+strings.Repeat("a", 64)))
}

func TestRunnerWithoutExecutorFailsClosed(t *testing.T) {
	executor := errorExecutor{}
	_, err := executor.Execute(context.Background(), t.TempDir(), nil, "true")
	require.ErrorContains(t, err, "sandbox executor is not configured")
}

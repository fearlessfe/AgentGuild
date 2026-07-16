package proxy

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnforceReceivePackRefAllowsOnlyExecutionBranch(t *testing.T) {
	allowed := receivePackBody(
		"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/agentguild/exec-1\x00report-status\n",
	)
	body, err := enforceReceivePackRef(strings.NewReader(allowed), "refs/heads/agentguild/exec-1")
	require.NoError(t, err)
	got, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, allowed, string(got))
}

func TestEnforceReceivePackRefRejectsDefaultAndMultipleBranches(t *testing.T) {
	for name, body := range map[string]string{
		"default": receivePackBody("0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/main\x00report-status\n"),
		"multiple": receivePackBody(
			"0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/agentguild/exec-1\x00report-status\n",
			"0000000000000000000000000000000000000000 2222222222222222222222222222222222222222 refs/heads/other\n",
		),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := enforceReceivePackRef(strings.NewReader(body), "refs/heads/agentguild/exec-1")
			require.Error(t, err)
		})
	}
}

func receivePackBody(lines ...string) string {
	var body strings.Builder
	for _, line := range lines {
		body.WriteString(fmt.Sprintf("%04x%s", len(line)+4, line))
	}
	body.WriteString("0000PACK")
	return body.String()
}

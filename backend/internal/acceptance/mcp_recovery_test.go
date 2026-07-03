package acceptance

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentRecoversAfterHeartbeatResponseLoss(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(time.Hour))

	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	first := env.MCP.HeartbeatDropResponse(claimed.ID, claimed.LeaseGeneration, "req-heartbeat")
	replay := env.MCP.Heartbeat(claimed.ID, claimed.LeaseGeneration, "req-heartbeat")

	require.Equal(t, first.StoredGeneration, replay.LeaseGeneration)
	require.Equal(t, 1, env.CountAuditIntent("heartbeat"))
}

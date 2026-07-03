package acceptance

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAgentHeartbeatReceivesTenMinuteLeaseAndPollAfterSeconds(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(time.Hour))

	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	require.Equal(t, int64(1), claimed.LeaseGeneration)
	require.WithinDuration(t, time.Now().Add(10*time.Minute), claimed.LeaseSoftExpiresAt, 30*time.Second)
	require.WithinDuration(t, time.Now().Add(10*time.Minute+30*time.Second), claimed.LeaseHardExpiresAt, 30*time.Second)
	require.Equal(t, 30, env.MCP.LastMeta().PollAfterSeconds)

	taskIDRest := env.PublishTask(time.Now().Add(time.Hour))
	restClaimed := env.REST.ClaimTask(taskIDRest, "req-claim-rest")
	_ = env.REST.Heartbeat(restClaimed.ID, restClaimed.LeaseGeneration, "req-hb-rest")
	require.Equal(t, 30, env.REST.LastMeta().PollAfterSeconds)
}

func TestHeartbeatAfterDeadlineReturnsDeadlineExceeded(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(200 * time.Millisecond))

	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	time.Sleep(300 * time.Millisecond) // 越过 task deadline

	require.Equal(t, "DEADLINE_EXCEEDED", env.MCP.HeartbeatCode(claimed.ID, claimed.LeaseGeneration, "req-hb"))
	require.Equal(t, "DEADLINE_EXCEEDED", env.REST.HeartbeatCode(claimed.ID, claimed.LeaseGeneration, "req-hb-rest"))
}

func TestNonHolderReceivesSafeNotFoundResponse(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(time.Hour))

	claimed := env.MCP.TaskClaim(taskID, "req-claim")

	// 另一个 agent 对不属于自己的 execution 进行 heartbeat，不能探测资源是否存在。
	require.Equal(t, "NOT_FOUND", env.MCP.As("token-agent-2").HeartbeatCode(claimed.ID, claimed.LeaseGeneration, "req-hb"))
	require.Equal(t, "NOT_FOUND", env.REST.As("token-agent-2").HeartbeatCode(claimed.ID, claimed.LeaseGeneration, "req-hb"))
}

func TestOnlyOneClaimSucceedsAmongOneHundredConcurrentAgents(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(time.Hour))

	var successes atomic.Int32
	var conflicts atomic.Int32
	var unexpected atomic.Pointer[error]
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := env.MCP.As(fmt.Sprintf("token-agent-%d", i)).TaskClaimCode(taskID, fmt.Sprintf("req-%d", i))
			switch code {
			case "":
				successes.Add(1)
			case "STATE_CONFLICT":
				conflicts.Add(1)
			default:
				err := fmt.Errorf("agent-%d got %s", i, code)
				unexpected.CompareAndSwap(nil, &err)
			}
		}(i)
	}
	wg.Wait()

	require.Nil(t, unexpected.Load())
	require.Equal(t, int32(1), successes.Load())
	require.Equal(t, int32(99), conflicts.Load())
}

func TestGracePeriodPreventsReclaimAndStaleGenerationIsRejected(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(time.Hour))

	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	// 将 soft expiry 推到过去，hard expiry 留在未来，模拟宽限期。
	env.SetLeaseOffsets(claimed.ID, -1*time.Minute, 1*time.Minute)

	// 宽限期内其它 agent 不能重新领取。
	require.Equal(t, "STATE_CONFLICT", env.MCP.As("token-agent-2").TaskClaimCode(taskID, "req-reclaim"))

	// 持有者正常 heartbeat，generation 升级到 2。
	beated := env.MCP.Heartbeat(claimed.ID, claimed.LeaseGeneration, "req-hb-1")
	require.Equal(t, int64(2), beated.LeaseGeneration)

	// 旧 generation 被拒绝。
	require.Equal(t, "LEASE_EXPIRED", env.MCP.HeartbeatCode(claimed.ID, claimed.LeaseGeneration, "req-hb-stale"))
}

func TestIllegalStateTransitionsAreRejected(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(time.Hour))

	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	started := env.MCP.StartExecution(claimed.ID, claimed.LeaseGeneration, "req-start")

	// 重复 start 返回 STATE_CONFLICT。
	require.Equal(t, "STATE_CONFLICT", env.MCP.StartExecutionCode(started.ID, started.LeaseGeneration, "req-start-again"))

	// 已经被领取的任务再次 claim 返回 STATE_CONFLICT。
	require.Equal(t, "STATE_CONFLICT", env.MCP.TaskClaimCode(taskID, "req-claim-again"))
}

func TestRESTAndMCPHeartbeatErrorCodesAreEquivalent(t *testing.T) {
	env := Start(t)
	taskID := env.PublishTask(time.Now().Add(200 * time.Millisecond))

	claimedByMCP := env.MCP.TaskClaim(taskID, "req-claim")
	time.Sleep(300 * time.Millisecond) // 越过 deadline

	mcpCode := env.MCP.HeartbeatCode(claimedByMCP.ID, claimedByMCP.LeaseGeneration, "req-hb")
	restCode := env.REST.HeartbeatCode(claimedByMCP.ID, claimedByMCP.LeaseGeneration, "req-hb-rest")

	require.Equal(t, "DEADLINE_EXCEEDED", mcpCode)
	require.Equal(t, "DEADLINE_EXCEEDED", restCode)
}

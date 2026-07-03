package application_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/ratelimit"
)

type countingRateLimiter struct{ calls int }

func (l *countingRateLimiter) Allow(context.Context, ratelimit.Key) (ratelimit.Decision, error) {
	l.calls++
	return ratelimit.Decision{Allowed: true}, nil
}

func TestStartAndHeartbeatAuthorizeBeforeConsumingRateLimit(t *testing.T) {
	limiter := &countingRateLimiter{}
	svc, _ := newServiceFixtureWithRateLimiter(limiter)
	unauthorized := principal("tenant", "worker")
	_, err := svc.StartExecution(context.Background(), unauthorized, application.StartExecution{ExecutionID: "execution"})
	assertDomainError(t, err, "forbidden", "")
	_, err = svc.HeartbeatExecution(context.Background(), unauthorized, application.HeartbeatExecution{ExecutionID: "execution"})
	assertDomainError(t, err, "forbidden", "")
	if limiter.calls != 0 {
		t.Fatalf("unauthorized requests consumed %d rate-limit tokens", limiter.calls)
	}
}

func TestClaimIsAtomicAndReplayStable(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen, Deadline: fixtureNow.Add(time.Hour)})
	p := principal("tenant", "worker", "tasks:claim")
	command := application.ClaimTask{RequestID: "claim-1", TaskID: "task"}

	first, err := svc.ClaimTask(context.Background(), p, command)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.ClaimTask(context.Background(), p, command)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("unstable replay: first=%#v second=%#v", first, second)
	}
	if first.Data.LeaseGeneration != 1 || !first.Data.LeaseSoftExpiresAt.Equal(fixtureNow.Add(10*time.Minute)) || !first.Data.LeaseHardExpiresAt.Equal(fixtureNow.Add(10*time.Minute+30*time.Second)) {
		t.Fatalf("lease=%#v", first.Data)
	}
	task := tx.tasks[tx.key("tenant", "task")]
	if task.Status != domain.TaskClaimed || task.ActiveExecutionID != first.Data.ID || len(tx.executions) != 1 || len(tx.events) != 1 || len(tx.outbox) != 1 {
		t.Fatalf("task=%#v executions=%d events=%d outbox=%d", task, len(tx.executions), len(tx.events), len(tx.outbox))
	}
}

func TestClaimAtDeadlineIsRejected(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen, Deadline: fixtureNow})
	_, err := svc.ClaimTask(context.Background(), principal("tenant", "worker", "tasks:claim"), application.ClaimTask{RequestID: "claim", TaskID: "task"})
	assertDomainError(t, err, "state_conflict", "")
}

func TestStartAndHeartbeatFenceOwnerGenerationAndHardExpiry(t *testing.T) {
	for _, test := range []struct {
		name       string
		now        time.Time
		principal  string
		generation int64
		wantCode   string
	}{
		{name: "wrong owner", principal: "other", generation: 3, wantCode: "not_found"},
		{name: "stale generation", principal: "worker", generation: 2, wantCode: "lease_expired"},
		{name: "hard expiry equality", now: fixtureNow.Add(30 * time.Second), principal: "worker", generation: 3, wantCode: "lease_expired"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, tx := newServiceFixture()
			if !test.now.IsZero() {
				tx.now = test.now
			}
			tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskClaimed, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
			tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionLeased, Lease: domain.Lease{Generation: 3, SoftExpiry: fixtureNow.Add(10 * time.Second), HardExpiry: fixtureNow.Add(30 * time.Second)}}
			_, err := svc.StartExecution(context.Background(), principal("tenant", test.principal, "tasks:execute"), application.StartExecution{RequestID: "start", ExecutionID: "execution", LeaseGeneration: test.generation})
			assertDomainError(t, err, test.wantCode, "")
		})
	}
}

func TestHeartbeatInGraceRenewsAndIncrementsGeneration(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.now = fixtureNow.Add(20 * time.Second)
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 3, SoftExpiry: fixtureNow.Add(10 * time.Second), HardExpiry: fixtureNow.Add(30 * time.Second)}}

	got, err := svc.HeartbeatExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.HeartbeatExecution{RequestID: "beat", ExecutionID: "execution", LeaseGeneration: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.LeaseGeneration != 4 || !got.Data.LeaseSoftExpiresAt.Equal(tx.now.Add(10*time.Minute)) || !got.Data.LeaseHardExpiresAt.Equal(tx.now.Add(10*time.Minute+30*time.Second)) {
		t.Fatalf("heartbeat=%#v", got)
	}
}

func TestStartAtomicallyMovesTaskToInProgress(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskClaimed, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionLeased, Lease: domain.Lease{Generation: 1, SoftExpiry: fixtureNow.Add(10 * time.Minute), HardExpiry: fixtureNow.Add(10*time.Minute + 30*time.Second)}}

	got, err := svc.StartExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.StartExecution{RequestID: "start", ExecutionID: "execution", LeaseGeneration: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.Status != domain.ExecutionRunning || tx.tasks[tx.key("tenant", "task")].Status != domain.TaskInProgress {
		t.Fatalf("execution=%s task=%s", got.Data.Status, tx.tasks[tx.key("tenant", "task")].Status)
	}
}

func TestStartRollsBackExecutionWhenTaskUpdateFails(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskClaimed, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionLeased, Lease: domain.Lease{Generation: 1, SoftExpiry: fixtureNow.Add(10 * time.Minute), HardExpiry: fixtureNow.Add(10*time.Minute + 30*time.Second)}}
	tx.failAt = "task"

	_, err := svc.StartExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.StartExecution{RequestID: "start", ExecutionID: "execution", LeaseGeneration: 1})
	if err == nil {
		t.Fatal("expected injected task failure")
	}
	if tx.tasks[tx.key("tenant", "task")].Status != domain.TaskClaimed || tx.executions["execution"].Status != domain.ExecutionLeased || len(tx.events) != 0 || len(tx.outbox) != 0 || len(tx.idem) != 0 {
		t.Fatalf("partial start: task=%s execution=%s events=%d outbox=%d idem=%d", tx.tasks[tx.key("tenant", "task")].Status, tx.executions["execution"].Status, len(tx.events), len(tx.outbox), len(tx.idem))
	}
}

func TestHeartbeatAtTaskDeadlineIsRejected(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1, SoftExpiry: fixtureNow.Add(10 * time.Minute), HardExpiry: fixtureNow.Add(10*time.Minute + 30*time.Second)}}

	_, err := svc.HeartbeatExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.HeartbeatExecution{RequestID: "beat", ExecutionID: "execution", LeaseGeneration: 1})
	assertDomainError(t, err, "deadline_exceeded", "")
}

func TestGetExecutionIsTenantScoped(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1}}
	_, err := svc.GetExecution(context.Background(), principal("other", "worker", "tasks:read"), application.GetExecution{ExecutionID: "execution"})
	assertDomainError(t, err, "not_found", "")
	got, err := svc.GetExecution(context.Background(), principal("tenant", "other", "tasks:read"), application.GetExecution{ExecutionID: "execution"})
	if err != nil {
		t.Fatalf("GetExecution()=%v", err)
	}
	if got.Data.ID != "execution" {
		t.Fatalf("got execution id=%s", got.Data.ID)
	}
}

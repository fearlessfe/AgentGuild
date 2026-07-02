package domain_test

import (
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestHeartbeatRejectsStaleGeneration(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	execution := domain.NewLeasedExecution(
		"exe-1",
		"task-1",
		"tenant-1",
		"agent-1",
		now,
		3,
	)

	_, err := execution.Heartbeat(now.Add(time.Minute), 2)

	if !errors.Is(err, domain.ErrLeaseExpired) {
		t.Fatalf("Heartbeat() error = %v, want %v", err, domain.ErrLeaseExpired)
	}
}

func TestRenewLeaseUsesConfiguredWindows(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)

	lease := domain.RenewLease(now, 3)

	if lease.Generation != 4 {
		t.Fatalf("generation = %d, want 4", lease.Generation)
	}
	if want := now.Add(domain.LeaseDuration); !lease.SoftExpiry.Equal(want) {
		t.Fatalf("soft expiry = %v, want %v", lease.SoftExpiry, want)
	}
	if want := now.Add(domain.LeaseDuration + domain.LeaseGrace); !lease.HardExpiry.Equal(want) {
		t.Fatalf("hard expiry = %v, want %v", lease.HardExpiry, want)
	}
}

func TestExecutionStartsWithCurrentLease(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	execution := domain.NewLeasedExecution(
		"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
	)

	if err := execution.Start(now.Add(time.Minute), 3); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if execution.Status != domain.ExecutionRunning {
		t.Fatalf("status = %q, want %q", execution.Status, domain.ExecutionRunning)
	}
}

func TestHeartbeatRenewsCurrentLease(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	heartbeatAt := now.Add(time.Minute)
	execution := domain.NewLeasedExecution(
		"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
	)

	lease, err := execution.Heartbeat(heartbeatAt, 3)

	if err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if lease.Generation != 4 {
		t.Fatalf("generation = %d, want 4", lease.Generation)
	}
	if !execution.Lease.SoftExpiry.Equal(heartbeatAt.Add(domain.LeaseDuration)) {
		t.Fatalf("execution lease was not renewed: %+v", execution.Lease)
	}
}

func TestExecutionRejectsOperationsAfterHardExpiry(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	execution := domain.NewLeasedExecution(
		"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
	)
	afterHardExpiry := execution.Lease.HardExpiry.Add(time.Nanosecond)

	if err := execution.Start(afterHardExpiry, 3); !errors.Is(err, domain.ErrLeaseExpired) {
		t.Fatalf("Start() error = %v, want %v", err, domain.ErrLeaseExpired)
	}
	if _, err := execution.Heartbeat(afterHardExpiry, 3); !errors.Is(err, domain.ErrLeaseExpired) {
		t.Fatalf("Heartbeat() error = %v, want %v", err, domain.ErrLeaseExpired)
	}
	if execution.Status != domain.ExecutionLeased {
		t.Fatalf("status = %q, want unchanged %q", execution.Status, domain.ExecutionLeased)
	}
}

func TestExecutionExpiresOnlyAfterHardExpiry(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	execution := domain.NewLeasedExecution(
		"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
	)

	if err := execution.Expire(execution.Lease.HardExpiry); !errors.Is(err, domain.ErrStateConflict) {
		t.Fatalf("Expire(at boundary) error = %v, want %v", err, domain.ErrStateConflict)
	}
	if err := execution.Expire(execution.Lease.HardExpiry.Add(time.Nanosecond)); err != nil {
		t.Fatalf("Expire(after boundary) error = %v", err)
	}
	if execution.Status != domain.ExecutionExpired {
		t.Fatalf("status = %q, want %q", execution.Status, domain.ExecutionExpired)
	}
}

func FuzzExecutionNeverReturnsFromTerminal(f *testing.F) {
	f.Add(uint8(0))
	f.Add(uint8(255))

	f.Fuzz(func(t *testing.T, sequence uint8) {
		execution := domain.AcceptedExecutionFixture()

		_ = execution.Apply(
			domain.Intent(sequence),
			domain.SystemActor(),
			time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC),
		)

		if execution.Status != domain.ExecutionAccepted {
			t.Fatalf("status = %q, want terminal %q", execution.Status, domain.ExecutionAccepted)
		}
	})
}

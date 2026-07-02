package domain_test

import (
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestHeartbeatRejectsStaleGeneration(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	execution := mustNewExecution(t,
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
	execution := mustNewExecution(t,
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
	execution := mustNewExecution(t,
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
	execution := mustNewExecution(t,
		"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
	)
	afterHardExpiry := execution.Lease.HardExpiry

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
	execution := mustNewExecution(t,
		"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
	)

	if err := execution.Expire(execution.Lease.HardExpiry); err != nil {
		t.Fatalf("Expire(at boundary) error = %v", err)
	}
	if execution.Status != domain.ExecutionExpired {
		t.Fatalf("status = %q, want %q", execution.Status, domain.ExecutionExpired)
	}
}

func TestExecutionAcceptRequiresRunningStateAndReviewer(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	reviewer := domain.Actor{Type: domain.ActorReviewer, ID: "reviewer-1"}
	execution := mustNewExecution(t,
		"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
	)

	if err := execution.Apply(domain.IntentAccept, reviewer, now.Add(time.Minute)); !errors.Is(err, domain.ErrStateConflict) {
		t.Fatalf("Apply(accept leased) error = %v, want %v", err, domain.ErrStateConflict)
	}
	if err := execution.Start(now.Add(time.Minute), 3); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := execution.Apply(
		domain.IntentAccept,
		domain.Actor{Type: domain.ActorAgent, ID: "agent-1"},
		now.Add(2*time.Minute),
	); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Apply(accept by agent) error = %v, want %v", err, domain.ErrForbidden)
	}
	if execution.Status != domain.ExecutionRunning {
		t.Fatalf("status after rejected acceptance = %q, want %q", execution.Status, domain.ExecutionRunning)
	}
	if err := execution.Apply(domain.IntentAccept, reviewer, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("Apply(accept by reviewer) error = %v", err)
	}
	if execution.Status != domain.ExecutionAccepted {
		t.Fatalf("status = %q, want %q", execution.Status, domain.ExecutionAccepted)
	}
}

func TestExecutionExplicitOperationMatrix(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	reviewer := domain.Actor{Type: domain.ActorReviewer, ID: "reviewer-1"}

	operations := []struct {
		name       string
		transition func(*domain.Execution) error
		allowed    map[domain.ExecutionStatus]domain.ExecutionStatus
	}{
		{
			name: "start",
			transition: func(execution *domain.Execution) error {
				return execution.Start(now.Add(time.Minute), 3)
			},
			allowed: map[domain.ExecutionStatus]domain.ExecutionStatus{
				domain.ExecutionLeased: domain.ExecutionRunning,
			},
		},
		{
			name: "heartbeat",
			transition: func(execution *domain.Execution) error {
				_, err := execution.Heartbeat(now.Add(time.Minute), 3)
				return err
			},
			allowed: map[domain.ExecutionStatus]domain.ExecutionStatus{
				domain.ExecutionLeased:  domain.ExecutionLeased,
				domain.ExecutionRunning: domain.ExecutionRunning,
			},
		},
		{
			name: "accept",
			transition: func(execution *domain.Execution) error {
				return execution.Accept(reviewer, now.Add(time.Minute))
			},
			allowed: map[domain.ExecutionStatus]domain.ExecutionStatus{
				domain.ExecutionRunning: domain.ExecutionAccepted,
			},
		},
		{
			name: "expire",
			transition: func(execution *domain.Execution) error {
				return execution.Expire(execution.Lease.HardExpiry)
			},
			allowed: map[domain.ExecutionStatus]domain.ExecutionStatus{
				domain.ExecutionLeased:  domain.ExecutionExpired,
				domain.ExecutionRunning: domain.ExecutionExpired,
			},
		},
	}
	statuses := []domain.ExecutionStatus{
		domain.ExecutionLeased,
		domain.ExecutionRunning,
		domain.ExecutionAccepted,
		domain.ExecutionExpired,
	}

	for _, operation := range operations {
		for _, status := range statuses {
			t.Run(operation.name+"/"+string(status), func(t *testing.T) {
				execution := mustNewExecution(
					t, "exe-1", "task-1", "tenant-1", "agent-1", now, 3,
				)
				execution.Status = status
				before := *execution

				err := operation.transition(execution)
				wantStatus, allowed := operation.allowed[status]
				if !allowed {
					if !errors.Is(err, domain.ErrStateConflict) {
						t.Fatalf("operation error = %v, want %v", err, domain.ErrStateConflict)
					}
					if *execution != before {
						t.Fatalf("execution mutated: got %+v, want %+v", *execution, before)
					}
					return
				}
				if err != nil {
					t.Fatalf("operation error = %v", err)
				}
				if execution.Status != wantStatus {
					t.Fatalf("status = %q, want %q", execution.Status, wantStatus)
				}
			})
		}
	}
}

func TestExecutionConstructorRejectsInvalidIdentityAndGeneration(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		id         string
		taskID     string
		tenantID   string
		agentID    string
		generation int64
		field      string
	}{
		{name: "empty execution ID", taskID: "task-1", tenantID: "tenant-1", agentID: "agent-1", generation: 1, field: "id"},
		{name: "empty task ID", id: "exe-1", tenantID: "tenant-1", agentID: "agent-1", generation: 1, field: "task_id"},
		{name: "empty tenant ID", id: "exe-1", taskID: "task-1", agentID: "agent-1", generation: 1, field: "tenant_id"},
		{name: "empty agent ID", id: "exe-1", taskID: "task-1", tenantID: "tenant-1", generation: 1, field: "agent_id"},
		{name: "negative generation", id: "exe-1", taskID: "task-1", tenantID: "tenant-1", agentID: "agent-1", generation: -1, field: "generation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execution, err := domain.NewLeasedExecution(
				tt.id, tt.taskID, tt.tenantID, tt.agentID, now, tt.generation,
			)
			if execution != nil {
				t.Fatalf("NewLeasedExecution() = %+v, want nil", execution)
			}
			assertInvalidArgument(t, err, tt.field)
		})
	}
}

func FuzzExecutionNeverReturnsFromTerminal(f *testing.F) {
	f.Add(uint8(0))
	f.Add(uint8(255))

	f.Fuzz(func(t *testing.T, sequence uint8) {
		now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
		execution := mustNewExecution(t,
			"exe-1", "task-1", "tenant-1", "agent-1", now, 3,
		)
		reviewer := domain.Actor{Type: domain.ActorReviewer, ID: "reviewer-1"}
		generation := int64(3)

		for step := 0; step < 4; step++ {
			switch (sequence >> (step * 2)) & 0x3 {
			case 0:
				_ = execution.Start(now.Add(time.Minute), generation)
			case 1:
				if lease, err := execution.Heartbeat(now.Add(time.Minute), generation); err == nil {
					generation = lease.Generation
				}
			case 2:
				_ = execution.Apply(domain.IntentAccept, reviewer, now.Add(time.Minute))
			case 3:
				_ = execution.Expire(execution.Lease.HardExpiry)
			}

			if execution.Status == domain.ExecutionAccepted || execution.Status == domain.ExecutionExpired {
				terminal := execution.Status
				_ = execution.Start(now.Add(time.Minute), generation)
				_, _ = execution.Heartbeat(now.Add(time.Minute), generation)
				_ = execution.Apply(domain.IntentAccept, reviewer, now.Add(time.Minute))
				_ = execution.Expire(execution.Lease.HardExpiry)
				if execution.Status != terminal {
					t.Fatalf("terminal status changed from %q to %q", terminal, execution.Status)
				}
			}
		}
	})
}

func mustNewExecution(
	t *testing.T,
	id, taskID, tenantID, agentID string,
	now time.Time,
	generation int64,
) *domain.Execution {
	t.Helper()
	execution, err := domain.NewLeasedExecution(id, taskID, tenantID, agentID, now, generation)
	if err != nil {
		t.Fatalf("NewLeasedExecution() error = %v", err)
	}
	return execution
}

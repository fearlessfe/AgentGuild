package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
)

func rejectionEvents(tx *fakeTx) []application.TaskEvent {
	var out []application.TaskEvent
	for _, e := range tx.events {
		if strings.HasPrefix(e.Intent, application.RejectedIntentPrefix) {
			out = append(out, e)
		}
	}
	return out
}

func requireSingleRejection(t *testing.T, tx *fakeTx, intent, actorID, reason string) application.TaskEvent {
	t.Helper()
	events := rejectionEvents(tx)
	if len(events) != 1 {
		t.Fatalf("rejection events=%d, want 1: %#v", len(events), tx.events)
	}
	event := events[0]
	if event.Intent != application.RejectedIntentPrefix+intent {
		t.Fatalf("intent=%q, want %q", event.Intent, application.RejectedIntentPrefix+intent)
	}
	if event.ActorID != actorID {
		t.Fatalf("actor=%q, want %q", event.ActorID, actorID)
	}
	if event.Reason != reason {
		t.Fatalf("reason=%q, want %q", event.Reason, reason)
	}
	if event.ToState != event.FromState || event.FromState == "" {
		t.Fatalf("rejected transition must keep state unchanged: from=%q to=%q", event.FromState, event.ToState)
	}
	return event
}

func TestHeartbeatByNonOwnerIsRejectedAndAudited(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1, SoftExpiry: fixtureNow.Add(10 * time.Minute), HardExpiry: fixtureNow.Add(10*time.Minute + 30*time.Second)}}

	_, err := svc.HeartbeatExecution(context.Background(), principal("tenant", "intruder", "tasks:execute"), application.HeartbeatExecution{RequestID: "beat", ExecutionID: "execution", LeaseGeneration: 1})
	assertDomainError(t, err, "not_found", "")

	event := requireSingleRejection(t, tx, "heartbeat", "intruder", "not_found")
	if event.TaskID != "task" || event.ExecutionID != "execution" || event.ActorType != string(domain.ActorAgent) || event.FromState != string(domain.ExecutionRunning) {
		t.Fatalf("event=%#v", event)
	}
	if tx.executions["execution"].Status != domain.ExecutionRunning {
		t.Fatalf("execution mutated: %#v", tx.executions["execution"])
	}
}

func TestClaimOnClaimedTaskIsRejectedAndAudited(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskClaimed, ClaimedBy: "worker", Deadline: fixtureNow.Add(time.Hour)})

	_, err := svc.ClaimTask(context.Background(), principal("tenant", "other", "tasks:claim"), application.ClaimTask{RequestID: "claim", TaskID: "task"})
	assertDomainError(t, err, "state_conflict", "")

	event := requireSingleRejection(t, tx, "claim", "other", "state_conflict")
	if event.ExecutionID != "" || event.FromState != string(domain.TaskClaimed) {
		t.Fatalf("event=%#v", event)
	}
}

func TestCancelByNonPublisherIsRejectedAndAudited(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen, Deadline: fixtureNow.Add(time.Hour)})

	_, err := svc.CancelTask(context.Background(), principal("tenant-1", "other-publisher", "tasks:cancel"), application.CancelTask{RequestID: "cancel", TaskID: "task"})
	assertDomainError(t, err, "not_found", "")

	event := requireSingleRejection(t, tx, "cancel", "other-publisher", "not_found")
	if event.ActorType != string(domain.ActorPublisher) || event.FromState != string(domain.TaskOpen) {
		t.Fatalf("event=%#v", event)
	}
	if tx.tasks[tx.key("tenant-1", "task")].Status != domain.TaskOpen {
		t.Fatalf("task mutated: %#v", tx.tasks[tx.key("tenant-1", "task")])
	}
}

func TestHeartbeatOnSubmittedExecutionIsRejectedAndAudited(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionSubmitted, Lease: domain.Lease{Generation: 2, SoftExpiry: fixtureNow.Add(10 * time.Minute), HardExpiry: fixtureNow.Add(10*time.Minute + 30*time.Second)}}

	_, err := svc.HeartbeatExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.HeartbeatExecution{RequestID: "beat", ExecutionID: "execution", LeaseGeneration: 2})
	assertDomainError(t, err, "state_conflict", "")

	event := requireSingleRejection(t, tx, "heartbeat", "worker", "state_conflict")
	if event.FromState != string(domain.ExecutionSubmitted) {
		t.Fatalf("event=%#v", event)
	}
}

func TestStaleLeaseHeartbeatIsNotAudited(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 3, SoftExpiry: fixtureNow.Add(10 * time.Minute), HardExpiry: fixtureNow.Add(10*time.Minute + 30*time.Second)}}

	_, err := svc.HeartbeatExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.HeartbeatExecution{RequestID: "beat", ExecutionID: "execution", LeaseGeneration: 2})
	assertDomainError(t, err, "lease_expired", "")
	if got := rejectionEvents(tx); len(got) != 0 {
		t.Fatalf("stale lease retry must not be audited as rejection: %#v", got)
	}
}

func TestPublisherForgingSystemValidationIsRejectedAndAudited(t *testing.T) {
	tx := newFakeTx()
	notifier := application.NewCoreExecutionNotifier(&fakeStore{tx: tx})
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher-v1", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionSubmitted}

	err := notifier.Notify(context.Background(), gitapp.ExecutionStateCommand{
		TenantID: "tenant", ExecutionID: "execution",
		Intent: domain.IntentStartValidation,
		Actor:  domain.Actor{Type: domain.ActorPublisher, ID: "publisher-v1"},
	}, fixtureNow)
	assertDomainError(t, err, "forbidden", "")

	event := requireSingleRejection(t, tx, "start_validation", "publisher-v1", "forbidden")
	if event.ActorType != string(domain.ActorPublisher) || event.FromState != string(domain.ExecutionSubmitted) {
		t.Fatalf("event=%#v", event)
	}
	if tx.executions["execution"].Status != domain.ExecutionSubmitted {
		t.Fatalf("execution mutated: %#v", tx.executions["execution"])
	}
}

func TestExecutorAcceptingOwnWorkIsRejectedAndAudited(t *testing.T) {
	tx := newFakeTx()
	notifier := application.NewCoreExecutionNotifier(&fakeStore{tx: tx})
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionReviewing}

	err := notifier.Notify(context.Background(), gitapp.ExecutionStateCommand{
		TenantID: "tenant", ExecutionID: "execution",
		Intent: domain.IntentAccept,
		Actor:  domain.Actor{Type: domain.ActorAgent, ID: "worker"},
	}, fixtureNow)
	assertDomainError(t, err, "forbidden", "")

	event := requireSingleRejection(t, tx, "accept", "worker", "forbidden")
	if event.ActorType != string(domain.ActorAgent) || event.FromState != string(domain.ExecutionReviewing) {
		t.Fatalf("event=%#v", event)
	}
}

func TestSystemValidationOnWrongStateIsRejectedAndAudited(t *testing.T) {
	tx := newFakeTx()
	notifier := application.NewCoreExecutionNotifier(&fakeStore{tx: tx})
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning}

	err := notifier.Notify(context.Background(), gitapp.ExecutionStateCommand{
		TenantID: "tenant", ExecutionID: "execution",
		Intent: domain.IntentStartValidation,
		Actor:  domain.Actor{Type: domain.ActorSystem, ID: "validation-worker"},
	}, fixtureNow)
	assertDomainError(t, err, "state_conflict", "")

	event := requireSingleRejection(t, tx, "start_validation", "validation-worker", "state_conflict")
	if event.ActorType != string(domain.ActorSystem) || event.FromState != string(domain.ExecutionRunning) {
		t.Fatalf("event=%#v", event)
	}
}

func TestRejectionAuditFailureDoesNotMaskOriginalError(t *testing.T) {
	tx := newFakeTx()
	notifier := application.NewCoreExecutionNotifier(&fakeStore{tx: tx})
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionReviewing}
	tx.failAt = "event"

	err := notifier.Notify(context.Background(), gitapp.ExecutionStateCommand{
		TenantID: "tenant", ExecutionID: "execution",
		Intent: domain.IntentAccept,
		Actor:  domain.Actor{Type: domain.ActorAgent, ID: "worker"},
	}, fixtureNow)
	assertDomainError(t, err, "forbidden", "")
}

func TestRejectionEventDoesNotPolluteLatestExecutionSummary(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ClaimedBy: "worker", ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1, SoftExpiry: fixtureNow.Add(10 * time.Minute), HardExpiry: fixtureNow.Add(10*time.Minute + 30*time.Second)}}

	if _, err := svc.HeartbeatExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.HeartbeatExecution{RequestID: "beat-1", ExecutionID: "execution", LeaseGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.HeartbeatExecution(context.Background(), principal("tenant", "intruder", "tasks:execute"), application.HeartbeatExecution{RequestID: "beat-2", ExecutionID: "execution", LeaseGeneration: 2}); err == nil {
		t.Fatal("non-owner heartbeat unexpectedly succeeded")
	}

	got, err := svc.GetExecution(context.Background(), principal("tenant", "observer", "tasks:read"), application.GetExecution{ExecutionID: "execution"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Data.AuditSummary, "heartbeat") || strings.Contains(got.Data.AuditSummary, "reject") || strings.Contains(got.Data.AuditSummary, "intruder") {
		t.Fatalf("audit summary polluted by rejection event: %q", got.Data.AuditSummary)
	}

	page, err := svc.ListTaskEvents(context.Background(), principal("tenant", "observer", "tasks:read"), application.ListTaskEvents{TaskID: "task", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, event := range page.Data.Events {
		if event.Intent == application.RejectedIntentPrefix+"heartbeat" && event.ActorID == "intruder" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rejection event missing from audit query: %#v", page.Data.Events)
	}
}

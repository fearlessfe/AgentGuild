package application_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	identitydomain "agentguild.dev/agentguild/backend/internal/identity/domain"
)

var fixtureNow = time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)

func TestPublishRequiresDeadlineAndScope(t *testing.T) {
	svc, _ := newServiceFixture()
	_, err := svc.PublishTask(context.Background(), principal("tenant-1", "publisher-1", "tasks:publish"), application.PublishTask{
		RequestID: "req-1", Title: "Fix parser",
	})
	assertDomainError(t, err, "invalid_argument", "deadline")

	_, err = svc.PublishTask(context.Background(), principal("tenant-1", "publisher-1"), application.PublishTask{
		RequestID: "req-1", Title: "Fix parser", Deadline: fixtureNow.Add(time.Hour),
	})
	assertDomainError(t, err, "forbidden", "")
}

func TestNewServiceRejectsShortCursorSecret(t *testing.T) {
	_, err := application.NewService(&fakeStore{tx: newFakeTx()}, application.Options{CursorSecret: []byte(strings.Repeat("x", 31))})
	assertDomainError(t, err, "invalid_argument", "cursor_secret")
}

func TestPublishDeadlineMustBeStrictlyAfterTransactionTime(t *testing.T) {
	for _, deadline := range []time.Time{fixtureNow, fixtureNow.Add(-time.Nanosecond)} {
		svc, tx := newServiceFixture()
		_, err := svc.PublishTask(context.Background(), principal("tenant-1", "publisher-1", "tasks:publish"), application.PublishTask{RequestID: "req", Deadline: deadline})
		assertDomainError(t, err, "invalid_argument", "deadline")
		if len(tx.tasks) != 0 {
			t.Fatalf("deadline %v persisted a task", deadline)
		}
	}
}

func TestEveryEntryRejectsIncompletePrincipalBeforeAuthorization(t *testing.T) {
	svc, _ := newServiceFixture()
	calls := []struct {
		name string
		call func(auth.Principal) error
	}{
		{"publish", func(p auth.Principal) error {
			_, err := svc.PublishTask(context.Background(), p, application.PublishTask{})
			return err
		}},
		{"list", func(p auth.Principal) error {
			_, err := svc.ListTasks(context.Background(), p, application.ListTasks{})
			return err
		}},
		{"get", func(p auth.Principal) error {
			_, err := svc.GetTask(context.Background(), p, application.GetTask{})
			return err
		}},
		{"cancel", func(p auth.Principal) error {
			_, err := svc.CancelTask(context.Background(), p, application.CancelTask{})
			return err
		}},
	}
	for _, call := range calls {
		t.Run(call.name, func(t *testing.T) {
			err := call.call(auth.Principal{AgentID: "agent", AgentVersionID: "version", Scopes: []string{"tasks:publish", "tasks:read", "tasks:cancel"}})
			assertDomainError(t, err, "invalid_argument", "tenant_id")
		})
	}
}

func TestPublishPersistsLosslessBodyEventsAndStableReplay(t *testing.T) {
	svc, tx := newServiceFixture()
	cmd := application.PublishTask{
		RequestID: "req-1", Type: "code", Title: "Fix parser", Problem: "It races",
		Constraints: []string{"offline"}, Requirements: []string{"tests"},
		Deadline: fixtureNow.Add(time.Hour),
	}
	first, err := svc.PublishTask(context.Background(), principal("tenant-1", "publisher-1", "tasks:publish"), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.PublishTask(context.Background(), principal("tenant-1", "publisher-1", "tasks:publish"), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.Data.ID == "" || !reflect.DeepEqual(first.Data, second.Data) || first.Meta != second.Meta {
		t.Fatalf("unstable replay: first=%#v second=%#v", first, second)
	}
	record := tx.tasks[tx.key("tenant-1", first.Data.ID)]
	if string(record.Constraints) != `["offline"]` || string(record.Requirements) != `["tests"]` || len(tx.events) != 1 || len(tx.outbox) != 1 {
		t.Fatalf("lossy or incomplete publish: record=%#v events=%d outbox=%d", record, len(tx.events), len(tx.outbox))
	}
}

func TestTaskOperationsCheckLiveAgentInsideTaskTransaction(t *testing.T) {
	tx := newFakeTx()
	tx.seedLiveAgent("tenant-1", "publisher", identitydomain.AgentActive, "publisher-v1")
	store := &fakeStore{
		tx: tx,
		beforeTx: func(tx *fakeTx) {
			tx.seedLiveAgent("tenant-1", "publisher", identitydomain.AgentSuspended, "publisher-v1")
		},
	}
	svc, err := application.NewService(store, application.Options{
		CursorSecret: []byte("01234567890123456789012345678901"),
	})
	if err != nil {
		t.Fatal(err)
	}
	p := principal("tenant-1", "publisher-v1", "tasks:publish")
	p.Type = auth.PrincipalTypeAgent
	p.AgentID = "publisher"

	_, err = svc.PublishTask(context.Background(), p, application.PublishTask{RequestID: "req", Deadline: fixtureNow.Add(time.Hour)})

	if !errors.Is(err, identitydomain.ErrStateConflict) {
		t.Fatalf("PublishTask error=%v, want identity state conflict", err)
	}
	if len(tx.tasks) != 0 {
		t.Fatalf("suspended agent transaction persisted tasks: %#v", tx.tasks)
	}
}

func TestTaskOperationsRejectUnknownLiveAgent(t *testing.T) {
	svc, tx := newServiceFixture()
	p := principal("tenant-1", "unknown-version", "tasks:publish")
	p.Type = auth.PrincipalTypeAgent
	p.AgentID = "unknown-agent"

	_, err := svc.PublishTask(context.Background(), p, application.PublishTask{RequestID: "req", Deadline: fixtureNow.Add(time.Hour)})

	if !errors.Is(err, identitydomain.ErrForbidden) {
		t.Fatalf("PublishTask error=%v, want identity forbidden", err)
	}
	if len(tx.tasks) != 0 {
		t.Fatalf("unknown agent persisted tasks: %#v", tx.tasks)
	}
}

func TestListCursorIsSignedAndBoundToTenantAndFilter(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task-3", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskOpen, CreatedAt: fixtureNow.Add(3 * time.Minute)})
	tx.seed(application.TaskRecord{ID: "task-2", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskOpen, CreatedAt: fixtureNow.Add(2 * time.Minute)})
	tx.seed(application.TaskRecord{ID: "task-1", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskCancelled, CreatedAt: fixtureNow.Add(time.Minute)})

	page, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskOpen}, Limit: 1})
	if err != nil || len(page.Data.Items) != 1 || page.Data.Items[0].ID != "task-3" || page.Meta.NextCursor == "" {
		t.Fatalf("first page=%#v err=%v", page, err)
	}
	next, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskOpen}, Limit: 1, Cursor: page.Meta.NextCursor})
	if err != nil || len(next.Data.Items) != 1 || next.Data.Items[0].ID != "task-2" {
		t.Fatalf("next page=%#v err=%v", next, err)
	}
	_, err = svc.ListTasks(context.Background(), principal("tenant-2", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskOpen}, Cursor: page.Meta.NextCursor})
	assertDomainError(t, err, "invalid_argument", "cursor")
	_, err = svc.ListTasks(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskCancelled}, Cursor: page.Meta.NextCursor})
	assertDomainError(t, err, "invalid_argument", "cursor")
}

func TestListRejectsTamperedExpiredCursorAndInvalidLimits(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "b", TenantID: "tenant-1", Status: domain.TaskOpen, CreatedAt: fixtureNow})
	tx.seed(application.TaskRecord{ID: "a", TenantID: "tenant-1", Status: domain.TaskOpen, CreatedAt: fixtureNow})
	page, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	tampered := page.Meta.NextCursor[:len(page.Meta.NextCursor)-1] + "A"
	_, err = svc.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Cursor: tampered})
	assertDomainError(t, err, "invalid_argument", "cursor")
	tx.now = fixtureNow.Add(2 * time.Hour)
	_, err = svc.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Cursor: page.Meta.NextCursor})
	assertDomainError(t, err, "invalid_argument", "cursor")
	for _, limit := range []int{-1, 101} {
		_, err = svc.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Limit: limit})
		assertDomainError(t, err, "invalid_argument", "limit")
	}
}

func TestCursorCannotBeVerifiedByDifferentSecret(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "b", TenantID: "tenant-1", Status: domain.TaskOpen, CreatedAt: fixtureNow})
	tx.seed(application.TaskRecord{ID: "a", TenantID: "tenant-1", Status: domain.TaskOpen, CreatedAt: fixtureNow})
	page, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	other, err := application.NewService(&fakeStore{tx: tx}, application.Options{CursorSecret: []byte("abcdefghijklmnopqrstuvwxyz123456")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = other.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Cursor: page.Meta.NextCursor})
	assertDomainError(t, err, "invalid_argument", "cursor")
}

func TestListPaginatesEveryTaskWithSameTimestamp(t *testing.T) {
	svc, tx := newServiceFixture()
	for _, id := range []string{"c", "b", "a"} {
		tx.seed(application.TaskRecord{ID: id, TenantID: "tenant-1", Status: domain.TaskOpen, CreatedAt: fixtureNow})
	}
	var ids []string
	cursor := ""
	for {
		page, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Limit: 1, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range page.Data.Items {
			ids = append(ids, task.ID)
		}
		cursor = page.Meta.NextCursor
		if cursor == "" {
			break
		}
	}
	if !reflect.DeepEqual(ids, []string{"c", "b", "a"}) {
		t.Fatalf("ids=%v", ids)
	}
}

func TestIdempotencyRequestMismatchIsRejected(t *testing.T) {
	svc, _ := newServiceFixture()
	p := principal("tenant-1", "publisher-1", "tasks:publish")
	_, err := svc.PublishTask(context.Background(), p, application.PublishTask{RequestID: "same", Title: "first", Deadline: fixtureNow.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.PublishTask(context.Background(), p, application.PublishTask{RequestID: "same", Title: "different", Deadline: fixtureNow.Add(time.Hour)})
	assertDomainError(t, err, "idempotency_mismatch", "")
}

func TestGetIsTenantScopedAndCancelAtomicallyCancelsActiveExecution(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "shared", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskInProgress, Deadline: fixtureNow.Add(time.Hour), ActiveExecutionID: "execution-1", StateVersion: 2})
	tx.executions["execution-1"] = &domain.Execution{ID: "execution-1", TaskID: "shared", TenantID: "tenant-1", AgentID: "worker-1", Status: domain.ExecutionRunning}
	tx.seed(application.TaskRecord{ID: "shared", TenantID: "tenant-2", PublisherAgentVersionID: "publisher-2", Status: domain.TaskOpen, Deadline: fixtureNow.Add(time.Hour)})

	got, err := svc.GetTask(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.GetTask{TaskID: "shared"})
	if err != nil || got.Data.PublisherAgentVersionID != "publisher-1" {
		t.Fatalf("GetTask()=%#v, %v", got, err)
	}
	_, err = svc.CancelTask(context.Background(), principal("tenant-1", "other-publisher", "tasks:cancel"), application.CancelTask{RequestID: "cancel-1", TaskID: "shared"})
	assertDomainError(t, err, "not_found", "")

	cancelled, err := svc.CancelTask(context.Background(), principal("tenant-1", "publisher-1", "tasks:cancel"), application.CancelTask{RequestID: "cancel-2", TaskID: "shared", Reason: "superseded"})
	if err != nil || cancelled.Data.Status != domain.TaskCancelled || tx.executions["execution-1"].Status != domain.ExecutionCancelled {
		t.Fatalf("CancelTask()=%#v execution=%#v err=%v", cancelled, tx.executions["execution-1"], err)
	}
	if len(tx.events) != 1 || len(tx.outbox) != 1 {
		t.Fatalf("cancel audit/outbox: %d/%d", len(tx.events), len(tx.outbox))
	}
	if tx.outbox[0].EventType != "task.cancelled" {
		t.Fatalf("cancel outbox type = %q, want task.cancelled", tx.outbox[0].EventType)
	}
}

func TestListTasksFiltersByTypeAndReturnsPollAfterSeconds(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "code-task", TenantID: "tenant-1", Type: "code", Status: domain.TaskOpen, CreatedAt: fixtureNow})
	tx.seed(application.TaskRecord{ID: "doc-task", TenantID: "tenant-1", Type: "doc", Status: domain.TaskOpen, CreatedAt: fixtureNow})

	page, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.ListTasks{Type: "code"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Data.Items) != 1 || page.Data.Items[0].ID != "code-task" {
		t.Fatalf("filter by type returned %#v", page.Data.Items)
	}
	if page.Meta.PollAfterSeconds == 0 {
		t.Fatalf("poll_after_seconds not set")
	}
}

func TestGetExecutionIncludesUsageAndAuditSummary(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Stage: "running tests", Progress: 0.65, Lease: domain.Lease{Generation: 1}}
	tx.events = append(tx.events, application.TaskEvent{TenantID: "tenant", TaskID: "task", ExecutionID: "execution", ActorType: "agent", ActorID: "worker", Intent: "heartbeat", FromState: "running", ToState: "running", CreatedAt: fixtureNow})

	got, err := svc.GetExecution(context.Background(), principal("tenant", "observer", "tasks:read"), application.GetExecution{ExecutionID: "execution"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.Stage != "running tests" || got.Data.Progress != 0.65 {
		t.Fatalf("stage/progress missing: %#v", got.Data)
	}
	if got.Meta.PollAfterSeconds == 0 {
		t.Fatalf("poll_after_seconds not set")
	}
	if got.Data.AuditSummary == "" {
		t.Fatalf("audit summary missing")
	}
}

func TestGetExecutionAllowsExecutionAgentOwner(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1}}

	got, err := svc.GetExecution(context.Background(), principal("tenant", "worker", "tasks:execute"), application.GetExecution{ExecutionID: "execution"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Data.ID != "execution" {
		t.Fatalf("execution view missing: %#v", got.Data)
	}
}

func TestGetExecutionExecutionAgentNonOwnerIsNotFound(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1}}

	_, err := svc.GetExecution(context.Background(), principal("tenant", "other", "tasks:execute"), application.GetExecution{ExecutionID: "execution"})
	assertDomainError(t, err, "not_found", "")
}

func TestGetExecutionWithoutScopeIsForbidden(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1}}

	_, err := svc.GetExecution(context.Background(), principal("tenant", "worker"), application.GetExecution{ExecutionID: "execution"})
	assertDomainError(t, err, "forbidden", "")
}

func TestGetExecutionPropagatesUsageError(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1}}
	tx.usageErr = errors.New("injected usage error")

	_, err := svc.GetExecution(context.Background(), principal("tenant", "observer", "tasks:read"), application.GetExecution{ExecutionID: "execution"})
	if err == nil || err.Error() != "injected usage error" {
		t.Fatalf("expected injected usage error, got %v", err)
	}
}

func TestGetExecutionPropagatesLatestEventError(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, ActiveExecutionID: "execution", Deadline: fixtureNow.Add(time.Hour)})
	tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant", AgentID: "worker", Status: domain.ExecutionRunning, Lease: domain.Lease{Generation: 1}}
	tx.latestEventErr = errors.New("injected latest event error")

	_, err := svc.GetExecution(context.Background(), principal("tenant", "observer", "tasks:read"), application.GetExecution{ExecutionID: "execution"})
	if err == nil || err.Error() != "injected latest event error" {
		t.Fatalf("expected injected latest event error, got %v", err)
	}
}

func TestCancelDoesNotRevealWhetherTaskExistsOrHasDifferentOwner(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "owned-by-other", TenantID: "tenant-1", PublisherAgentVersionID: "other", Status: domain.TaskOpen, Deadline: fixtureNow.Add(time.Hour)})
	p := principal("tenant-1", "publisher", "tasks:cancel")
	_, invisible := svc.CancelTask(context.Background(), p, application.CancelTask{RequestID: "one", TaskID: "owned-by-other"})
	_, missing := svc.CancelTask(context.Background(), p, application.CancelTask{RequestID: "two", TaskID: "missing"})
	if invisible == nil || missing == nil || invisible.Error() != missing.Error() || domain.CodeOf(invisible) != domain.CodeOf(missing) {
		t.Fatalf("invisible=%v missing=%v", invisible, missing)
	}
}

func TestCancelRollsBackAtEveryMutationFailure(t *testing.T) {
	stages := []string{"execution", "task", "event", "outbox", "complete"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			svc, tx := newServiceFixture()
			tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, Deadline: fixtureNow.Add(time.Hour), ActiveExecutionID: "execution", StateVersion: 2})
			tx.executions["execution"] = &domain.Execution{ID: "execution", TaskID: "task", TenantID: "tenant-1", Status: domain.ExecutionRunning}
			tx.failAt = stage
			_, err := svc.CancelTask(context.Background(), principal("tenant-1", "publisher", "tasks:cancel"), application.CancelTask{RequestID: "cancel", TaskID: "task"})
			if err == nil {
				t.Fatal("expected injected failure")
			}
			task := tx.tasks[tx.key("tenant-1", "task")]
			if task.Status != domain.TaskInProgress || task.StateVersion != 2 || task.ActiveExecutionID != "execution" || tx.executions["execution"].Status != domain.ExecutionRunning || len(tx.events) != 0 || len(tx.outbox) != 0 || len(tx.idem) != 0 {
				t.Fatalf("partial state after %s failure: task=%#v execution=%s events=%d outbox=%d idem=%d", stage, task, tx.executions["execution"].Status, len(tx.events), len(tx.outbox), len(tx.idem))
			}
		})
	}
}

func TestCancelQueriesAllActiveExecutionsInsteadOfTrustingTaskPointer(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskInProgress, Deadline: fixtureNow.Add(time.Hour), ActiveExecutionID: "stale"})
	for i, status := range []domain.ExecutionStatus{domain.ExecutionLeased, domain.ExecutionRunning, domain.ExecutionSubmitted, domain.ExecutionValidating, domain.ExecutionReviewing, domain.ExecutionRevisionRequested} {
		id := "execution-" + string(rune('a'+i))
		tx.executions[id] = &domain.Execution{ID: id, TaskID: "task", TenantID: "tenant-1", Status: status}
	}
	_, err := svc.CancelTask(context.Background(), principal("tenant-1", "publisher", "tasks:cancel"), application.CancelTask{RequestID: "cancel", TaskID: "task"})
	if err != nil {
		t.Fatal(err)
	}
	for id, execution := range tx.executions {
		if execution.Status != domain.ExecutionCancelled {
			t.Errorf("%s status=%s", id, execution.Status)
		}
	}
}

func TestCorruptTaskJSONIsReturnedAsDataIntegrityError(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "bad", TenantID: "tenant-1", Constraints: []byte(`{}`), Requirements: []byte(`[]`), Status: domain.TaskOpen})
	_, err := svc.GetTask(context.Background(), principal("tenant-1", "agent", "tasks:read"), application.GetTask{TaskID: "bad"})
	if err == nil {
		t.Fatal("corrupt constraints were silently ignored")
	}
}

func principal(tenant, agentVersion string, scopes ...string) auth.Principal {
	return auth.Principal{TenantID: tenant, AgentID: "agent", AgentVersionID: agentVersion, Scopes: scopes}
}

func newServiceFixture() (*application.Service, *fakeTx) {
	tx := newFakeTx()
	next := 0
	svc, err := application.NewService(&fakeStore{tx: tx}, application.Options{
		CursorSecret: []byte("01234567890123456789012345678901"), CursorTTL: time.Hour,
		NewID: func() string { next++; return "id-" + string(rune('0'+next)) },
	})
	if err != nil {
		panic(err)
	}
	return svc, tx
}

func newFakeTx() *fakeTx {
	return &fakeTx{now: fixtureNow, tasks: map[string]application.TaskRecord{}, executions: map[string]*domain.Execution{}, idem: map[application.IdempotencyKey]*application.IdempotencyRecord{}}
}

func assertDomainError(t *testing.T, err error, code, field string) {
	t.Helper()
	var target *domain.Error
	if !errors.As(err, &target) || target.Code != code || target.Field != field {
		t.Fatalf("error=%v, want code=%q field=%q", err, code, field)
	}
}

type liveAgentRecord struct {
	status           string
	currentVersionID string
}

type fakeStore struct {
	tx       *fakeTx
	beforeTx func(*fakeTx)
}

func (s *fakeStore) WithTx(_ context.Context, fn func(application.Tx) error) error {
	if s.beforeTx != nil {
		s.beforeTx(s.tx)
	}
	working := s.tx.clone()
	if err := fn(working); err != nil {
		return err
	}
	*s.tx = *working
	return nil
}

type fakeTx struct {
	now            time.Time
	tasks          map[string]application.TaskRecord
	executions     map[string]*domain.Execution
	idem           map[application.IdempotencyKey]*application.IdempotencyRecord
	events         []application.TaskEvent
	outbox         []application.OutboxEvent
	failAt         string
	usageErr       error
	latestEventErr error
	liveAgents     map[string]liveAgentRecord
}

func (tx *fakeTx) clone() *fakeTx {
	copyTx := &fakeTx{now: tx.now, tasks: make(map[string]application.TaskRecord, len(tx.tasks)), executions: make(map[string]*domain.Execution, len(tx.executions)), idem: make(map[application.IdempotencyKey]*application.IdempotencyRecord, len(tx.idem)), events: append([]application.TaskEvent(nil), tx.events...), outbox: append([]application.OutboxEvent(nil), tx.outbox...), failAt: tx.failAt, usageErr: tx.usageErr, latestEventErr: tx.latestEventErr, liveAgents: make(map[string]liveAgentRecord, len(tx.liveAgents))}
	for key, task := range tx.tasks {
		task.Constraints = append([]byte(nil), task.Constraints...)
		task.Requirements = append([]byte(nil), task.Requirements...)
		copyTx.tasks[key] = task
	}
	for key, execution := range tx.executions {
		copyExecution := *execution
		copyTx.executions[key] = &copyExecution
	}
	for key, record := range tx.idem {
		copyRecord := *record
		copyRecord.ResponseBody = append([]byte(nil), record.ResponseBody...)
		copyTx.idem[key] = &copyRecord
	}
	for key, record := range tx.liveAgents {
		copyTx.liveAgents[key] = record
	}
	return copyTx
}

func (tx *fakeTx) key(tenant, id string) string { return tenant + "/" + id }
func (tx *fakeTx) seedLiveAgent(tenantID, agentID, status, currentVersionID string) {
	if tx.liveAgents == nil {
		tx.liveAgents = map[string]liveAgentRecord{}
	}
	tx.liveAgents[tx.key(tenantID, agentID)] = liveAgentRecord{status: status, currentVersionID: currentVersionID}
}
func (tx *fakeTx) RequireLiveAgent(_ context.Context, principal auth.Principal) error {
	record, ok := tx.liveAgents[tx.key(principal.TenantID, principal.AgentID)]
	if !ok {
		return identitydomain.ErrForbidden
	}
	if record.currentVersionID != "" && principal.AgentVersionID != record.currentVersionID {
		return identitydomain.ErrForbidden
	}
	switch record.status {
	case identitydomain.AgentActive:
		return nil
	case identitydomain.AgentRevoked:
		return identitydomain.ErrTokenRevoked
	default:
		return identitydomain.ErrStateConflict
	}
}
func (tx *fakeTx) seed(r application.TaskRecord) {
	if r.Constraints == nil {
		r.Constraints = []byte(`[]`)
	}
	if r.Requirements == nil {
		r.Requirements = []byte(`[]`)
	}
	tx.tasks[tx.key(r.TenantID, r.ID)] = r
}
func (tx *fakeTx) Now(context.Context) (time.Time, error) { return tx.now, nil }
func (tx *fakeTx) InsertTask(_ context.Context, r application.TaskRecord) error {
	tx.seed(r)
	return nil
}
func (tx *fakeTx) GetTask(_ context.Context, tenant, id string) (*application.TaskRecord, error) {
	r, ok := tx.tasks[tx.key(tenant, id)]
	if !ok {
		return nil, &domain.Error{Code: "not_found", Message: "task was not found"}
	}
	return &r, nil
}
func (tx *fakeTx) UpdateTask(_ context.Context, r application.TaskRecord, version int64, active string) (bool, error) {
	if tx.failAt == "task" {
		return false, errors.New("injected task failure")
	}
	old, ok := tx.tasks[tx.key(r.TenantID, r.ID)]
	if !ok || old.StateVersion != version {
		return false, nil
	}
	r.StateVersion = version + 1
	r.ActiveExecutionID = active
	r.UpdatedAt = tx.now
	tx.seed(r)
	return true, nil
}
func (tx *fakeTx) ClaimTask(_ context.Context, tenant, id string, version int64, executionID string) (bool, error) {
	r, ok := tx.tasks[tx.key(tenant, id)]
	if !ok || r.StateVersion != version || r.Status != domain.TaskOpen || !tx.now.Before(r.Deadline) {
		return false, nil
	}
	r.Status = domain.TaskClaimed
	r.StateVersion++
	r.ActiveExecutionID = executionID
	r.UpdatedAt = tx.now
	tx.seed(r)
	return true, nil
}
func (tx *fakeTx) ListTaskRecords(_ context.Context, q application.TaskListQuery) ([]application.TaskRecord, error) {
	var out []application.TaskRecord
	for _, r := range tx.tasks {
		if r.TenantID != q.TenantID || !statusAllowed(r.Status, q.Statuses) {
			continue
		}
		if q.Type != "" && r.Type != q.Type {
			continue
		}
		if !q.AfterCreatedAt.IsZero() && !(r.CreatedAt.Before(q.AfterCreatedAt) || (r.CreatedAt.Equal(q.AfterCreatedAt) && r.ID < q.AfterID)) {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}
func statusAllowed(status domain.TaskStatus, allowed []domain.TaskStatus) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, v := range allowed {
		if status == v {
			return true
		}
	}
	return false
}
func (tx *fakeTx) InsertExecution(_ context.Context, execution *domain.Execution, _ []byte) error {
	copy := *execution
	tx.executions[execution.ID] = &copy
	return nil
}
func (tx *fakeTx) GetExecution(_ context.Context, tenant, id string) (*domain.Execution, int64, error) {
	e, ok := tx.executions[id]
	if !ok || e.TenantID != tenant {
		return nil, 0, &domain.Error{Code: "not_found", Message: "resource not found"}
	}
	copy := *e
	return &copy, 0, nil
}
func (tx *fakeTx) GetExecutionUsage(_ context.Context, _, _ string) (*application.UsageView, error) {
	if tx.usageErr != nil {
		return nil, tx.usageErr
	}
	return nil, nil
}
func (tx *fakeTx) GetExecutionForUpdate(ctx context.Context, tenant, id string) (*domain.Execution, int64, error) {
	return tx.GetExecution(ctx, tenant, id)
}
func (tx *fakeTx) ListActiveExecutions(_ context.Context, tenant, task string) ([]application.ExecutionRecord, error) {
	var records []application.ExecutionRecord
	for _, e := range tx.executions {
		if e.TenantID == tenant && e.TaskID == task && isActiveExecution(e.Status) {
			copy := *e
			records = append(records, application.ExecutionRecord{Execution: &copy, StateVersion: 0})
		}
	}
	return records, nil
}
func isActiveExecution(status domain.ExecutionStatus) bool {
	switch status {
	case domain.ExecutionLeased, domain.ExecutionRunning, domain.ExecutionSubmitted, domain.ExecutionValidating, domain.ExecutionReviewing, domain.ExecutionRevisionRequested:
		return true
	default:
		return false
	}
}
func (tx *fakeTx) UpdateExecution(_ context.Context, e *domain.Execution, _ int64) (bool, error) {
	if tx.failAt == "execution" {
		return false, errors.New("injected execution failure")
	}
	copy := *e
	tx.executions[e.ID] = &copy
	return true, nil
}
func (tx *fakeTx) UpdateOwnedExecution(_ context.Context, e *domain.Execution, _ int64, owner string, generation int64) (bool, error) {
	stored, ok := tx.executions[e.ID]
	if !ok || stored.AgentID != owner || stored.Lease.Generation != generation || tx.now.After(stored.Lease.HardExpiry) {
		return false, nil
	}
	copy := *e
	tx.executions[e.ID] = &copy
	return true, nil
}
func (tx *fakeTx) AcquireIdempotency(_ context.Context, key application.IdempotencyKey, hash [32]byte, expires time.Time) (*application.IdempotencyRecord, error) {
	if r := tx.idem[key]; r != nil {
		if r.RequestHash != hash {
			return nil, &domain.Error{Code: "idempotency_mismatch", Message: "mismatch"}
		}
		copy := *r
		return &copy, nil
	}
	r := &application.IdempotencyRecord{Key: key, RequestHash: hash, ExpiresAt: expires, OwnerToken: "owner", Acquired: true}
	tx.idem[key] = r
	copy := *r
	return &copy, nil
}
func (tx *fakeTx) CompleteIdempotency(_ context.Context, key application.IdempotencyKey, _ string, code int, body []byte) error {
	if tx.failAt == "complete" {
		return errors.New("injected complete failure")
	}
	r := tx.idem[key]
	r.ResponseCode = &code
	r.ResponseBody = append([]byte(nil), body...)
	r.Completed = true
	r.Acquired = false
	return nil
}
func (tx *fakeTx) AppendTaskEvent(_ context.Context, e application.TaskEvent) error {
	if tx.failAt == "event" {
		return errors.New("injected event failure")
	}
	tx.events = append(tx.events, e)
	return nil
}
func (tx *fakeTx) ListTaskEvents(_ context.Context, tenantID, taskID string, afterID int64, limit int) ([]application.TaskEventSummary, error) {
	var out []application.TaskEventSummary
	for _, e := range tx.events {
		if e.TenantID != tenantID || e.TaskID != taskID {
			continue
		}
		// fakeTx 没有保存自增 ID，这里用 created_at 与 actor 模拟唯一键。
		id := e.CreatedAt.UnixNano()
		if id <= afterID {
			continue
		}
		out = append(out, application.TaskEventSummary{ID: id, TenantID: e.TenantID, TaskID: e.TaskID, ExecutionID: e.ExecutionID, ActorType: e.ActorType, ActorID: e.ActorID, Intent: e.Intent, FromState: e.FromState, ToState: e.ToState, CreatedAt: e.CreatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (tx *fakeTx) AppendOutboxEvent(_ context.Context, e application.OutboxEvent) error {
	if tx.failAt == "outbox" {
		return errors.New("injected outbox failure")
	}
	tx.outbox = append(tx.outbox, e)
	return nil
}
func (tx *fakeTx) GetLatestExecutionEvent(_ context.Context, _, executionID string) (application.TaskEventSummary, error) {
	if tx.latestEventErr != nil {
		return application.TaskEventSummary{}, tx.latestEventErr
	}
	for i := len(tx.events) - 1; i >= 0; i-- {
		if tx.events[i].ExecutionID == executionID {
			return application.TaskEventSummary{ID: tx.events[i].CreatedAt.UnixNano(), TenantID: tx.events[i].TenantID, TaskID: tx.events[i].TaskID, ExecutionID: tx.events[i].ExecutionID, ActorType: tx.events[i].ActorType, ActorID: tx.events[i].ActorID, Intent: tx.events[i].Intent, FromState: tx.events[i].FromState, ToState: tx.events[i].ToState, CreatedAt: tx.events[i].CreatedAt}, nil
		}
	}
	return application.TaskEventSummary{}, nil
}

func (tx *fakeTx) Reviews() application.ReviewRepository      { return nil }
func (tx *fakeTx) LineComments() application.LineCommentRepository { return nil }
func (tx *fakeTx) Rubrics() application.RubricRepository      { return nil }
func (tx *fakeTx) Reviewers() application.ReviewerRepository  { return nil }

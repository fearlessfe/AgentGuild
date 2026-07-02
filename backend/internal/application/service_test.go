package application_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
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

func TestListCursorIsSignedAndBoundToTenantAndFilter(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task-3", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskOpen, CreatedAt: fixtureNow.Add(3 * time.Minute)})
	tx.seed(application.TaskRecord{ID: "task-2", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskOpen, CreatedAt: fixtureNow.Add(2 * time.Minute)})
	tx.seed(application.TaskRecord{ID: "task-1", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskCancelled, CreatedAt: fixtureNow.Add(time.Minute)})

	page, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskOpen}, Limit: 1})
	if err != nil || len(page.Data) != 1 || page.Data[0].ID != "task-3" || page.Meta.NextCursor == "" {
		t.Fatalf("first page=%#v err=%v", page, err)
	}
	next, err := svc.ListTasks(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskOpen}, Limit: 1, Cursor: page.Meta.NextCursor})
	if err != nil || len(next.Data) != 1 || next.Data[0].ID != "task-2" {
		t.Fatalf("next page=%#v err=%v", next, err)
	}
	_, err = svc.ListTasks(context.Background(), principal("tenant-2", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskOpen}, Cursor: page.Meta.NextCursor})
	assertDomainError(t, err, "invalid_argument", "cursor")
	_, err = svc.ListTasks(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTasks{Statuses: []domain.TaskStatus{domain.TaskCancelled}, Cursor: page.Meta.NextCursor})
	assertDomainError(t, err, "invalid_argument", "cursor")
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
	assertDomainError(t, err, "forbidden", "")

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

func principal(tenant, agentVersion string, scopes ...string) auth.Principal {
	return auth.Principal{TenantID: tenant, AgentID: "agent", AgentVersionID: agentVersion, Scopes: scopes}
}

func newServiceFixture() (*application.Service, *fakeTx) {
	tx := &fakeTx{now: fixtureNow, tasks: map[string]application.TaskRecord{}, executions: map[string]*domain.Execution{}, idem: map[application.IdempotencyKey]*application.IdempotencyRecord{}}
	next := 0
	return application.NewService(&fakeStore{tx: tx}, application.Options{
		CursorSecret: []byte("01234567890123456789012345678901"), CursorTTL: time.Hour,
		NewID: func() string { next++; return "id-" + string(rune('0'+next)) },
	}), tx
}

func assertDomainError(t *testing.T, err error, code, field string) {
	t.Helper()
	var target *domain.Error
	if !errors.As(err, &target) || target.Code != code || target.Field != field {
		t.Fatalf("error=%v, want code=%q field=%q", err, code, field)
	}
}

type fakeStore struct{ tx *fakeTx }

func (s *fakeStore) WithTx(_ context.Context, fn func(application.Tx) error) error { return fn(s.tx) }

type fakeTx struct {
	now        time.Time
	tasks      map[string]application.TaskRecord
	executions map[string]*domain.Execution
	idem       map[application.IdempotencyKey]*application.IdempotencyRecord
	events     []application.TaskEvent
	outbox     []application.OutboxEvent
}

func (tx *fakeTx) key(tenant, id string) string           { return tenant + "/" + id }
func (tx *fakeTx) seed(r application.TaskRecord)          { tx.tasks[tx.key(r.TenantID, r.ID)] = r }
func (tx *fakeTx) Now(context.Context) (time.Time, error) { return tx.now, nil }
func (tx *fakeTx) InsertTask(_ context.Context, r application.TaskRecord) error {
	tx.seed(r)
	return nil
}
func (tx *fakeTx) GetTask(_ context.Context, tenant, id string) (*application.TaskRecord, error) {
	r, ok := tx.tasks[tx.key(tenant, id)]
	if !ok {
		return nil, &domain.Error{Code: "not_found", Message: "resource not found"}
	}
	return &r, nil
}
func (tx *fakeTx) UpdateTask(_ context.Context, r application.TaskRecord, version int64, active string) (bool, error) {
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
func (tx *fakeTx) ClaimTask(context.Context, string, string, int64, string) (bool, error) {
	return false, nil
}
func (tx *fakeTx) ListTaskRecords(_ context.Context, q application.TaskListQuery) ([]application.TaskRecord, error) {
	var out []application.TaskRecord
	for _, r := range tx.tasks {
		if r.TenantID != q.TenantID || !statusAllowed(r.Status, q.Statuses) {
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
func (tx *fakeTx) InsertExecution(context.Context, *domain.Execution, []byte) error { return nil }
func (tx *fakeTx) GetExecution(_ context.Context, tenant, id string) (*domain.Execution, int64, error) {
	e, ok := tx.executions[id]
	if !ok || e.TenantID != tenant {
		return nil, 0, &domain.Error{Code: "not_found", Message: "resource not found"}
	}
	copy := *e
	return &copy, 0, nil
}
func (tx *fakeTx) UpdateExecution(_ context.Context, e *domain.Execution, _ int64) (bool, error) {
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
	r := tx.idem[key]
	r.ResponseCode = &code
	r.ResponseBody = append([]byte(nil), body...)
	r.Completed = true
	r.Acquired = false
	return nil
}
func (tx *fakeTx) AppendTaskEvent(_ context.Context, e application.TaskEvent) error {
	tx.events = append(tx.events, e)
	return nil
}
func (tx *fakeTx) AppendOutboxEvent(_ context.Context, e application.OutboxEvent) error {
	tx.outbox = append(tx.outbox, e)
	return nil
}

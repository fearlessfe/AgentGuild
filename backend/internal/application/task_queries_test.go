package application_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestListTasksAllowsHumanPrincipal(t *testing.T) {
	svc, _ := newServiceFixture()
	human := auth.Principal{
		TenantID: "tenant-1",
		Type:     auth.PrincipalTypeHuman,
	}

	if _, err := svc.ListTasks(context.Background(), human, application.ListTasks{Limit: 20}); err != nil {
		t.Fatalf("human ListTasks should pass, got %v", err)
	}
}

func TestListTasksSource(t *testing.T) {
	tx := newFakeTx()
	tx.seed(application.TaskRecord{ID: "task-with-source", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskOpen, CreatedAt: fixtureNow.Add(2)})
	tx.seed(application.TaskRecord{ID: "task-without-source", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskOpen, CreatedAt: fixtureNow.Add(1)})
	lookup := fakeIssueSourceLookup{
		sources: map[string]application.TaskSource{
			"task-with-source": {Kind: "issue", Repo: "acme/widgets", IssueNumber: 42, IssueURL: "https://github.com/acme/widgets/issues/42"},
		},
	}
	svc, err := application.NewService(&fakeStore{tx: tx}, application.Options{
		CursorSecret:      []byte("01234567890123456789012345678901"),
		IssueSourceLookup: &lookup,
	})
	if err != nil {
		t.Fatal(err)
	}

	page, err := svc.ListTasks(context.Background(), principal("tenant-1", "reader-1", "tasks:read"), application.ListTasks{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}

	if lookup.tenantID != "tenant-1" || len(lookup.taskIDs) != 2 {
		t.Fatalf("lookup called with tenant=%q taskIDs=%v", lookup.tenantID, lookup.taskIDs)
	}
	if len(page.Data.Items) != 2 {
		t.Fatalf("items=%d, want 2", len(page.Data.Items))
	}
	byID := map[string]application.TaskView{}
	for _, item := range page.Data.Items {
		byID[item.ID] = item
	}
	if byID["task-with-source"].Source == nil {
		t.Fatalf("source missing for mapped task: %#v", byID["task-with-source"])
	}
	if got := *byID["task-with-source"].Source; got.Kind != "issue" || got.Repo != "acme/widgets" || got.IssueNumber != 42 || got.IssueURL == "" {
		t.Fatalf("source=%#v, want issue acme/widgets #42 with url", got)
	}
	if byID["task-without-source"].Source != nil {
		t.Fatalf("unexpected source for unmapped task: %#v", byID["task-without-source"].Source)
	}
}

func TestGetTaskSource(t *testing.T) {
	tx := newFakeTx()
	tx.seed(application.TaskRecord{ID: "task-with-source", TenantID: "tenant-1", PublisherAgentVersionID: "publisher-1", Status: domain.TaskOpen, CreatedAt: fixtureNow})
	lookup := fakeIssueSourceLookup{
		sources: map[string]application.TaskSource{
			"task-with-source": {Kind: "issue", Repo: "acme/widgets", IssueNumber: 43, IssueURL: "https://github.com/acme/widgets/issues/43"},
		},
	}
	svc, err := application.NewService(&fakeStore{tx: tx}, application.Options{
		CursorSecret:      []byte("01234567890123456789012345678901"),
		IssueSourceLookup: &lookup,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.GetTask(context.Background(), principal("tenant-1", "reader-1", "tasks:read"), application.GetTask{TaskID: "task-with-source"})
	if err != nil {
		t.Fatal(err)
	}

	if lookup.tenantID != "tenant-1" || len(lookup.taskIDs) != 1 || lookup.taskIDs[0] != "task-with-source" {
		t.Fatalf("lookup called with tenant=%q taskIDs=%v", lookup.tenantID, lookup.taskIDs)
	}
	if result.Data.Source == nil {
		t.Fatalf("source missing for mapped task: %#v", result.Data)
	}
	if got := *result.Data.Source; got.Kind != "issue" || got.Repo != "acme/widgets" || got.IssueNumber != 43 || got.IssueURL == "" {
		t.Fatalf("source=%#v, want issue acme/widgets #43 with url", got)
	}
}

type fakeIssueSourceLookup struct {
	tenantID string
	taskIDs  []string
	sources  map[string]application.TaskSource
}

func (l *fakeIssueSourceLookup) LookupByTaskIDs(_ context.Context, tenantID string, taskIDs []string) (map[string]application.TaskSource, error) {
	l.tenantID = tenantID
	l.taskIDs = append([]string(nil), taskIDs...)
	return l.sources, nil
}

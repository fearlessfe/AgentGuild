package application_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestSystemTaskPublishCreatesOpenTaskWithSystemPublisher(t *testing.T) {
	svc, _ := newServiceFixture()

	view, err := svc.PublishSystemTask(context.Background(), application.PublishSystemTask{
		TenantID:     "tenant-1",
		RequestID:    "repo#123",
		Type:         "code",
		Title:        "Sync issue",
		Problem:      "Create a task from GitHub issue",
		Constraints:  []string{"preserve tenant isolation"},
		Requirements: []string{"publish as system"},
		Deadline:     fixtureNow.Add(30 * 24 * time.Hour),
	})

	if err != nil {
		t.Fatal(err)
	}
	if view.PublisherAgentVersionID != domain.SystemIssuePublisherID {
		t.Fatalf("publisher = %q, want %q", view.PublisherAgentVersionID, domain.SystemIssuePublisherID)
	}
	if view.Status != domain.TaskOpen {
		t.Fatalf("status = %q, want %q", view.Status, domain.TaskOpen)
	}
}

func TestSystemTaskCancelOpenTaskIsAuthorizedAsPublisher(t *testing.T) {
	svc, _ := newServiceFixture()
	published, err := svc.PublishSystemTask(context.Background(), application.PublishSystemTask{
		TenantID:  "tenant-1",
		RequestID: "repo#124",
		Type:      "code",
		Title:     "Sync issue",
		Problem:   "Create a task from GitHub issue",
		Deadline:  fixtureNow.Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := svc.CancelSystemTask(context.Background(), "tenant-1", published.ID, "issue closed")

	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != domain.TaskCancelled {
		t.Fatalf("status = %q, want %q", cancelled.Status, domain.TaskCancelled)
	}
}

func TestSystemTaskUpdateContentOnlyMutatesOpenOrDraftTasks(t *testing.T) {
	svc, tx := newServiceFixture()
	published, err := svc.PublishSystemTask(context.Background(), application.PublishSystemTask{
		TenantID:     "tenant-1",
		RequestID:    "repo#125",
		Type:         "code",
		Title:        "Original title",
		Problem:      "Original problem",
		Constraints:  []string{"old constraint"},
		Requirements: []string{"old requirement"},
		Deadline:     fixtureNow.Add(30 * 24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	err = svc.UpdateSystemTaskContent(context.Background(), "tenant-1", published.ID, "Updated title", "Updated problem", []string{"new constraint"}, []string{"new requirement"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.GetTask(context.Background(), principal("tenant-1", "reader-1", "tasks:read"), application.GetTask{TaskID: published.ID})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Data.Title != "Updated title" {
		t.Fatalf("title = %q, want %q", updated.Data.Title, "Updated title")
	}
	if updated.Data.Problem != "Updated problem" {
		t.Fatalf("problem = %q, want %q", updated.Data.Problem, "Updated problem")
	}
	if got := updated.Data.Constraints; len(got) != 1 || got[0] != "new constraint" {
		t.Fatalf("constraints = %#v, want %#v", got, []string{"new constraint"})
	}
	if got := updated.Data.Requirements; len(got) != 1 || got[0] != "new requirement" {
		t.Fatalf("requirements = %#v, want %#v", got, []string{"new requirement"})
	}

	record := tx.tasks[tx.key("tenant-1", published.ID)]
	record.Status = domain.TaskClaimed
	record.Title = "Claimed title"
	record.Problem = "Claimed problem"
	tx.seed(record)

	err = svc.UpdateSystemTaskContent(context.Background(), "tenant-1", published.ID, "Ignored title", "Ignored problem", []string{"ignored constraint"}, []string{"ignored requirement"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := svc.GetTask(context.Background(), principal("tenant-1", "reader-1", "tasks:read"), application.GetTask{TaskID: published.ID})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Data.Title != "Claimed title" {
		t.Fatalf("claimed title = %q, want %q", claimed.Data.Title, "Claimed title")
	}
	if claimed.Data.Problem != "Claimed problem" {
		t.Fatalf("claimed problem = %q, want %q", claimed.Data.Problem, "Claimed problem")
	}
}

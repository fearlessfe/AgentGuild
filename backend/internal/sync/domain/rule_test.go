package domain

import (
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
)

func TestNewRuleAcceptsValidRule(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	lastSyncedAt := now.Add(-time.Hour)

	rule, err := NewRule(
		"rule-1",
		"tenant-1",
		"octo/hello-world",
		"bugfix",
		[]string{"bug", "sync"},
		[]string{"wontfix"},
		"open",
		"p1",
		DedupeUpdate,
		true,
		lastSyncedAt,
		now,
		now,
	)
	if err != nil {
		t.Fatalf("NewRule returned error: %v", err)
	}

	if rule.ID != "rule-1" || rule.TenantID != "tenant-1" || rule.Repo != "octo/hello-world" {
		t.Fatalf("NewRule did not preserve identity fields: %+v", rule)
	}
	if rule.TaskType != "bugfix" || rule.DefaultPriority != "p1" || rule.DedupeStrategy != DedupeUpdate {
		t.Fatalf("NewRule did not preserve task defaults: %+v", rule)
	}
	if rule.IssueState != "open" || !rule.Enabled || !rule.LastSyncedAt.Equal(lastSyncedAt) {
		t.Fatalf("NewRule did not preserve sync settings: %+v", rule)
	}
	if !rule.CreatedAt.Equal(now) || !rule.UpdatedAt.Equal(now) {
		t.Fatalf("NewRule did not preserve timestamps: %+v", rule)
	}
	if got := rule.IncludeLabels; len(got) != 2 || got[0] != "bug" || got[1] != "sync" {
		t.Fatalf("NewRule did not preserve include labels: %+v", got)
	}
	if got := rule.ExcludeLabels; len(got) != 1 || got[0] != "wontfix" {
		t.Fatalf("NewRule did not preserve exclude labels: %+v", got)
	}
}

func TestNewRuleRejectsRepoWithoutSlash(t *testing.T) {
	_, err := validRuleWithRepo("octo")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewRule error = %v, want invalid_argument", err)
	}
	if got := FieldOf(err); got != "repo" {
		t.Fatalf("FieldOf(err) = %q, want repo", got)
	}
}

func TestNewRuleRejectsInvalidIssueState(t *testing.T) {
	_, err := validRuleWithIssueState("merged")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewRule error = %v, want invalid_argument", err)
	}
	if got := FieldOf(err); got != "issue_state" {
		t.Fatalf("FieldOf(err) = %q, want issue_state", got)
	}
}

func TestNewRuleRejectsEmptyTaskType(t *testing.T) {
	_, err := validRuleWithTaskType("")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewRule error = %v, want invalid_argument", err)
	}
	if got := FieldOf(err); got != "task_type" {
		t.Fatalf("FieldOf(err) = %q, want task_type", got)
	}
}

func TestNewRuleRejectsInvalidDedupeStrategy(t *testing.T) {
	_, err := validRuleWithDedupeStrategy("replace")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("NewRule error = %v, want invalid_argument", err)
	}
	if got := FieldOf(err); got != "dedupe_strategy" {
		t.Fatalf("FieldOf(err) = %q, want dedupe_strategy", got)
	}
}

func TestRuleMatchesWhenIncludeLabelIsPresent(t *testing.T) {
	rule := Rule{
		IncludeLabels: []string{"bug"},
		IssueState:    "open",
	}

	if !rule.Matches(git.Issue{State: "open", Labels: []string{"help wanted", "bug"}}) {
		t.Fatal("Matches returned false, want true")
	}
}

func TestRuleDoesNotMatchWhenExcludeLabelIsPresent(t *testing.T) {
	rule := Rule{
		IncludeLabels: []string{"bug"},
		ExcludeLabels: []string{"wontfix"},
		IssueState:    "open",
	}

	if rule.Matches(git.Issue{State: "open", Labels: []string{"bug", "wontfix"}}) {
		t.Fatal("Matches returned true, want false")
	}
}

func TestRuleMatchesAnyLabelWhenIncludeLabelsAreEmpty(t *testing.T) {
	rule := Rule{
		IssueState: "open",
	}

	if !rule.Matches(git.Issue{State: "open", Labels: []string{"enhancement"}}) {
		t.Fatal("Matches returned false, want true")
	}
}

func TestRuleDoesNotMatchClosedIssueWhenRuleFiltersOpen(t *testing.T) {
	rule := Rule{
		IssueState: "open",
	}

	if rule.Matches(git.Issue{State: "closed"}) {
		t.Fatal("Matches returned true, want false")
	}
}

func validRuleWithRepo(repo string) (*Rule, error) {
	return validRule(repo, "open", "bugfix", DedupeUpdate)
}

func validRuleWithIssueState(issueState string) (*Rule, error) {
	return validRule("octo/hello-world", issueState, "bugfix", DedupeUpdate)
}

func validRuleWithTaskType(taskType string) (*Rule, error) {
	return validRule("octo/hello-world", "open", taskType, DedupeUpdate)
}

func validRuleWithDedupeStrategy(dedupeStrategy string) (*Rule, error) {
	return validRule("octo/hello-world", "open", "bugfix", dedupeStrategy)
}

func validRule(repo, issueState, taskType, dedupeStrategy string) (*Rule, error) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	return NewRule(
		"rule-1",
		"tenant-1",
		repo,
		taskType,
		[]string{"bug"},
		[]string{"wontfix"},
		issueState,
		"p1",
		dedupeStrategy,
		true,
		time.Time{},
		now,
		now,
	)
}

package postgres

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	syncapp "agentguild.dev/agentguild/backend/internal/sync/application"
	syncdomain "agentguild.dev/agentguild/backend/internal/sync/domain"
	"agentguild.dev/agentguild/backend/internal/testdb"
)

func TestRuleRepositoryRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	repo := NewRuleRepository(db)

	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	lastSyncedAt := now.Add(-2 * time.Hour)
	rule := newTestRule(t, "rule-1", "tenant-1", true, lastSyncedAt, now)
	rule.SourceAuth = syncdomain.SourceAuthPublic

	if err := repo.Create(ctx, rule); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.Get(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertRuleEqual(t, got, rule)

	list, err := repo.List(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List() len = %d, want 1", len(list))
	}
	assertRuleEqual(t, list[0], rule)

	updatedAt := now.Add(time.Hour)
	rule.IncludeLabels = []string{"triage", "backend"}
	rule.ExcludeLabels = []string{"duplicate"}
	rule.IssueState = "all"
	rule.TaskType = "investigation"
	rule.DefaultPriority = "P1"
	rule.DedupeStrategy = syncdomain.DedupeSkip
	rule.Enabled = false
	rule.LastSyncedAt = now
	rule.UpdatedAt = updatedAt

	if err := repo.Update(ctx, rule); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	got, err = repo.Get(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("Get() after Update error = %v", err)
	}
	assertRuleEqual(t, got, rule)

	if err := repo.Delete(ctx, "tenant-1", "rule-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err = repo.Get(ctx, "tenant-1", "rule-1")
	if !errors.Is(err, syncdomain.ErrNotFound) {
		t.Fatalf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}

func TestRuleRepositoryListEnabledAllTenantsOnlyReturnsEnabledRules(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	repo := NewRuleRepository(db)

	now := time.Date(2026, 7, 7, 13, 0, 0, 0, time.UTC)
	for _, rule := range []*syncdomain.Rule{
		newTestRule(t, "enabled-tenant-1", "tenant-1", true, time.Time{}, now),
		newTestRule(t, "disabled-tenant-1", "tenant-1", false, time.Time{}, now),
		newTestRule(t, "enabled-tenant-2", "tenant-2", true, now.Add(-time.Hour), now),
	} {
		if err := repo.Create(ctx, rule); err != nil {
			t.Fatalf("Create(%s) error = %v", rule.ID, err)
		}
	}

	rules, err := repo.ListEnabledAllTenants(ctx)
	if err != nil {
		t.Fatalf("ListEnabledAllTenants() error = %v", err)
	}
	got := ruleIDs(rules)
	want := []string{"enabled-tenant-1", "enabled-tenant-2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListEnabledAllTenants() ids = %v, want %v", got, want)
	}
}

func TestMapRepositoryUpsertRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	repo := NewMapRepository(db)

	firstSync := time.Date(2026, 7, 7, 14, 0, 0, 0, time.UTC)
	mapping := &syncapp.Mapping{
		TenantID:     "tenant-1",
		Repo:         "octo/hello-world",
		IssueNumber:  42,
		TaskID:       "task-1",
		IssueState:   "open",
		IssueClosed:  false,
		IssueURL:     "https://github.com/octo/hello-world/issues/42",
		LastSyncedAt: firstSync,
	}
	if err := repo.Upsert(ctx, mapping); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	got, err := repo.Get(ctx, "tenant-1", "octo/hello-world", 42)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	assertMappingEqual(t, got, mapping)

	secondSync := firstSync.Add(time.Hour)
	mapping.TaskID = "task-2"
	mapping.IssueState = "closed"
	mapping.IssueClosed = true
	mapping.LastSyncedAt = secondSync
	if err := repo.Upsert(ctx, mapping); err != nil {
		t.Fatalf("Upsert() update error = %v", err)
	}
	got, err = repo.Get(ctx, "tenant-1", "octo/hello-world", 42)
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	assertMappingEqual(t, got, mapping)
}

func TestMapRepositoryLookupByTaskIDs(t *testing.T) {
	ctx := context.Background()
	db := testdb.StartPostgres(t)
	repo := NewMapRepository(db)

	now := time.Date(2026, 7, 7, 15, 0, 0, 0, time.UTC)
	mappings := []*syncapp.Mapping{
		{TenantID: "tenant-1", Repo: "octo/hello-world", IssueNumber: 42, TaskID: "task-1", IssueState: "open", IssueURL: "https://github.com/octo/hello-world/issues/42", LastSyncedAt: now},
		{TenantID: "tenant-1", Repo: "octo/hello-world", IssueNumber: 43, TaskID: "task-2", IssueState: "open", IssueURL: "https://github.com/octo/hello-world/issues/43", LastSyncedAt: now},
		{TenantID: "tenant-2", Repo: "octo/hello-world", IssueNumber: 44, TaskID: "task-1", IssueState: "open", IssueURL: "https://github.com/octo/hello-world/issues/44", LastSyncedAt: now},
	}
	for _, mapping := range mappings {
		if err := repo.Upsert(ctx, mapping); err != nil {
			t.Fatalf("Upsert(%s/%d) error = %v", mapping.TenantID, mapping.IssueNumber, err)
		}
	}

	sources, err := repo.LookupByTaskIDs(ctx, "tenant-1", []string{"task-1", "task-2", "missing"})
	if err != nil {
		t.Fatalf("LookupByTaskIDs() error = %v", err)
	}

	if len(sources) != 2 {
		t.Fatalf("sources len = %d, want 2: %#v", len(sources), sources)
	}
	if got := sources["task-1"]; got.Kind != "issue" || got.Repo != "octo/hello-world" || got.IssueNumber != 42 || got.IssueURL != "https://github.com/octo/hello-world/issues/42" {
		t.Fatalf("task-1 source = %#v, want issue octo/hello-world #42", got)
	}
	if got := sources["task-2"]; got.Kind != "issue" || got.Repo != "octo/hello-world" || got.IssueNumber != 43 || got.IssueURL != "https://github.com/octo/hello-world/issues/43" {
		t.Fatalf("task-2 source = %#v, want issue octo/hello-world #43", got)
	}
	if _, ok := sources["missing"]; ok {
		t.Fatalf("missing task returned a source: %#v", sources["missing"])
	}
}

func newTestRule(t *testing.T, id, tenantID string, enabled bool, lastSyncedAt, now time.Time) *syncdomain.Rule {
	t.Helper()
	rule, err := syncdomain.NewRule(
		id,
		tenantID,
		"octo/hello-world",
		"bugfix",
		[]string{"bug", "sync"},
		[]string{"wontfix"},
		"open",
		"P2",
		syncdomain.DedupeUpdate,
		"",
		enabled,
		lastSyncedAt,
		now,
		now,
	)
	if err != nil {
		t.Fatalf("NewRule() error = %v", err)
	}
	return rule
}

func assertRuleEqual(t *testing.T, got, want *syncdomain.Rule) {
	t.Helper()
	if got.ID != want.ID ||
		got.TenantID != want.TenantID ||
		got.Repo != want.Repo ||
		got.IssueState != want.IssueState ||
		got.TaskType != want.TaskType ||
		got.DefaultPriority != want.DefaultPriority ||
		got.DedupeStrategy != want.DedupeStrategy ||
		got.SourceAuth != want.SourceAuth ||
		got.Enabled != want.Enabled ||
		!got.LastSyncedAt.Equal(want.LastSyncedAt) ||
		!got.CreatedAt.Equal(want.CreatedAt) ||
		!got.UpdatedAt.Equal(want.UpdatedAt) ||
		!reflect.DeepEqual(got.IncludeLabels, want.IncludeLabels) ||
		!reflect.DeepEqual(got.ExcludeLabels, want.ExcludeLabels) {
		t.Fatalf("rule mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func assertMappingEqual(t *testing.T, got, want *syncapp.Mapping) {
	t.Helper()
	if got.TenantID != want.TenantID ||
		got.Repo != want.Repo ||
		got.IssueNumber != want.IssueNumber ||
		got.TaskID != want.TaskID ||
		got.IssueState != want.IssueState ||
		got.IssueClosed != want.IssueClosed ||
		got.IssueURL != want.IssueURL ||
		!got.LastSyncedAt.Equal(want.LastSyncedAt) {
		t.Fatalf("mapping mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func ruleIDs(rules []*syncdomain.Rule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}
	return ids
}

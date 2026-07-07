package application

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/gittest"
	syncdomain "agentguild.dev/agentguild/backend/internal/sync/domain"
)

func TestEngineCreatesSystemTaskForMatchingOpenIssue(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 7, 7, 9, 0, 0, 0, time.UTC)
	rule := mustRule(t, "rule-1", "tenant-1", "acme/api", []string{"bug"}, nil, "open", syncdomain.DedupeUpdate)
	rules := newFakeRuleRepo(rule)
	maps := newFakeMapRepo()
	sink := newFakeTaskSink()
	source := &gittest.StubIssueSource{Issues: map[string][]git.Issue{
		"acme/api": {{
			Number:  42,
			Title:   "Fix flaky sync",
			Body:    "Steps to reproduce...",
			State:   "open",
			Labels:  []string{"bug"},
			HTMLURL: "https://github.example/acme/api/issues/42",
		}},
	}}
	engine := NewEngine(rules, maps, sink, fakeSources{"tenant-1": source}, EngineOptions{
		Now:             func() time.Time { return now },
		DefaultDeadline: 48 * time.Hour,
	})

	result, err := engine.RunRule(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("RunRule returned error: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("Created = %d, want 1", result.Created)
	}
	if len(sink.published) != 1 {
		t.Fatalf("published tasks = %d, want 1", len(sink.published))
	}
	published := sink.published[0]
	if published.Type != "coding" || published.Title != "Fix flaky sync" || published.Problem != "Steps to reproduce..." {
		t.Fatalf("published task = %+v", published)
	}
	if !published.Deadline.Equal(now.Add(48 * time.Hour)) {
		t.Fatalf("deadline = %s, want %s", published.Deadline, now.Add(48*time.Hour))
	}
	mapping, err := maps.Get(ctx, "tenant-1", "acme/api", 42)
	if err != nil {
		t.Fatalf("mapping not written: %v", err)
	}
	if mapping.TaskID != "task-1" || mapping.IssueState != "open" || mapping.IssueClosed || mapping.IssueURL != "https://github.example/acme/api/issues/42" {
		t.Fatalf("mapping = %+v", mapping)
	}
}

func TestEngineSkipsIssueWithExcludedLabel(t *testing.T) {
	ctx := context.Background()
	rule := mustRule(t, "rule-1", "tenant-1", "acme/api", []string{"bug"}, []string{"wontfix"}, "open", syncdomain.DedupeUpdate)
	rules := newFakeRuleRepo(rule)
	maps := newFakeMapRepo()
	sink := newFakeTaskSink()
	source := &gittest.StubIssueSource{Issues: map[string][]git.Issue{
		"acme/api": {{
			Number: 12,
			Title:  "Do not import",
			State:  "open",
			Labels: []string{"bug", "wontfix"},
		}},
	}}
	engine := NewEngine(rules, maps, sink, fakeSources{"tenant-1": source}, EngineOptions{})

	result, err := engine.RunRule(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("RunRule returned error: %v", err)
	}
	if result.Skipped != 1 || result.Created != 0 {
		t.Fatalf("result = %+v, want skipped issue without create", result)
	}
	if len(sink.published) != 0 {
		t.Fatalf("published tasks = %d, want 0", len(sink.published))
	}
	if _, err := maps.Get(ctx, "tenant-1", "acme/api", 12); !errors.Is(err, syncdomain.ErrNotFound) {
		t.Fatalf("mapping error = %v, want not found", err)
	}
}

func TestEngineDedupesExistingMappingByUpdatingOpenTask(t *testing.T) {
	ctx := context.Background()
	rule := mustRule(t, "rule-1", "tenant-1", "acme/api", nil, nil, "open", syncdomain.DedupeUpdate)
	rules := newFakeRuleRepo(rule)
	maps := newFakeMapRepo()
	sink := newFakeTaskSink()
	source := &gittest.StubIssueSource{Issues: map[string][]git.Issue{
		"acme/api": {{
			Number: 7,
			Title:  "Initial title",
			Body:   "Initial body",
			State:  "open",
		}},
	}}
	engine := NewEngine(rules, maps, sink, fakeSources{"tenant-1": source}, EngineOptions{})

	first, err := engine.RunRule(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("first RunRule returned error: %v", err)
	}
	if first.Created != 1 {
		t.Fatalf("first result = %+v, want Created=1", first)
	}

	source.Issues["acme/api"][0].Title = "Updated title"
	source.Issues["acme/api"][0].Body = "Updated body"
	second, err := engine.RunRule(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("second RunRule returned error: %v", err)
	}
	if second.Created != 0 || second.Updated != 1 {
		t.Fatalf("second result = %+v, want Created=0 Updated=1", second)
	}
	if len(sink.published) != 1 {
		t.Fatalf("published tasks = %d, want exactly one create", len(sink.published))
	}
	if len(sink.updated) != 1 {
		t.Fatalf("updated tasks = %d, want 1", len(sink.updated))
	}
	if sink.updated[0].Content.Title != "Updated title" || sink.updated[0].Content.Problem != "Updated body" {
		t.Fatalf("updated content = %+v", sink.updated[0].Content)
	}
}

func TestEngineDoesNotOverwriteClaimedTaskOnOpenIssueUpdate(t *testing.T) {
	ctx := context.Background()
	rule := mustRule(t, "rule-1", "tenant-1", "acme/api", nil, nil, "open", syncdomain.DedupeUpdate)
	rules := newFakeRuleRepo(rule)
	maps := newFakeMapRepo()
	sink := newFakeTaskSink()
	source := &gittest.StubIssueSource{Issues: map[string][]git.Issue{
		"acme/api": {{
			Number: 8,
			Title:  "Initial title",
			Body:   "Initial body",
			State:  "open",
		}},
	}}
	engine := NewEngine(rules, maps, sink, fakeSources{"tenant-1": source}, EngineOptions{})
	if _, err := engine.RunRule(ctx, "tenant-1", "rule-1"); err != nil {
		t.Fatalf("first RunRule returned error: %v", err)
	}
	sink.statuses["task-1"] = "claimed"
	source.Issues["acme/api"][0].Title = "Claimed title update"
	source.Issues["acme/api"][0].Body = "Claimed body update"

	result, err := engine.RunRule(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("second RunRule returned error: %v", err)
	}
	if result.Updated != 0 || result.Skipped != 1 {
		t.Fatalf("result = %+v, want Updated=0 Skipped=1", result)
	}
	if len(sink.updated) != 0 {
		t.Fatalf("updated tasks = %d, want 0", len(sink.updated))
	}
}

func TestEngineReconcilesClosedIssuesByCancellingOnlyOpenTasks(t *testing.T) {
	ctx := context.Background()
	rule := mustRule(t, "rule-1", "tenant-1", "acme/api", nil, nil, "all", syncdomain.DedupeUpdate)
	rules := newFakeRuleRepo(rule)
	maps := newFakeMapRepo()
	maps.mustUpsert(t, &Mapping{TenantID: "tenant-1", Repo: "acme/api", IssueNumber: 9, TaskID: "task-open", IssueState: "open"})
	maps.mustUpsert(t, &Mapping{TenantID: "tenant-1", Repo: "acme/api", IssueNumber: 10, TaskID: "task-claimed", IssueState: "open"})
	sink := newFakeTaskSink()
	sink.statuses["task-open"] = "open"
	sink.statuses["task-claimed"] = "claimed"
	source := &gittest.StubIssueSource{Issues: map[string][]git.Issue{
		"acme/api": {
			{Number: 9, Title: "Closed open task", State: "closed"},
			{Number: 10, Title: "Closed claimed task", State: "closed"},
		},
	}}
	engine := NewEngine(rules, maps, sink, fakeSources{"tenant-1": source}, EngineOptions{})

	result, err := engine.RunRule(ctx, "tenant-1", "rule-1")
	if err != nil {
		t.Fatalf("RunRule returned error: %v", err)
	}
	if result.Cancelled != 1 || result.Flagged != 1 {
		t.Fatalf("result = %+v, want Cancelled=1 Flagged=1", result)
	}
	if len(sink.cancelled) != 1 || sink.cancelled[0].TaskID != "task-open" {
		t.Fatalf("cancelled = %+v, want task-open only", sink.cancelled)
	}
	claimedMapping, err := maps.Get(ctx, "tenant-1", "acme/api", 10)
	if err != nil {
		t.Fatalf("claimed mapping missing: %v", err)
	}
	if !claimedMapping.IssueClosed || claimedMapping.IssueState != "closed" {
		t.Fatalf("claimed mapping = %+v, want closed metadata", claimedMapping)
	}
}

func TestEngineRunAllEnabledCountsFailedRuleAndContinues(t *testing.T) {
	ctx := context.Background()
	ruleOne := mustRule(t, "rule-1", "tenant-1", "acme/api", nil, nil, "open", syncdomain.DedupeUpdate)
	ruleTwo := mustRule(t, "rule-2", "tenant-2", "acme/web", nil, nil, "open", syncdomain.DedupeUpdate)
	rules := newFakeRuleRepo(ruleOne, ruleTwo)
	maps := newFakeMapRepo()
	sink := newFakeTaskSink()
	sourceErr := errors.New("source unavailable")
	sourceTwo := &gittest.StubIssueSource{Issues: map[string][]git.Issue{
		"acme/web": {{Number: 3, Title: "Import me", State: "open"}},
	}}
	engine := NewEngine(rules, maps, sink, fakeSources{
		"tenant-1": &gittest.StubIssueSource{Err: sourceErr},
		"tenant-2": sourceTwo,
	}, EngineOptions{})

	result, err := engine.RunAllEnabled(ctx)
	if err != nil {
		t.Fatalf("RunAllEnabled returned error: %v", err)
	}
	if result.Failed != 1 || result.Created != 1 || len(result.Errors) != 1 {
		t.Fatalf("result = %+v, want Failed=1 Created=1 Errors=1", result)
	}
	if len(sink.published) != 1 || sink.published[0].Title != "Import me" {
		t.Fatalf("published = %+v, want tenant-2 issue to continue", sink.published)
	}
}

func TestEngineUsesLastSyncedAtAsNextSinceWatermark(t *testing.T) {
	ctx := context.Background()
	firstNow := time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
	secondNow := firstNow.Add(2 * time.Hour)
	nowValues := []time.Time{firstNow, secondNow}
	rule := mustRule(t, "rule-1", "tenant-1", "acme/api", nil, nil, "open", syncdomain.DedupeUpdate)
	rules := newFakeRuleRepo(rule)
	maps := newFakeMapRepo()
	sink := newFakeTaskSink()
	source := &recordingIssueSource{issues: map[string][]git.Issue{"acme/api": nil}}
	engine := NewEngine(rules, maps, sink, fakeSources{"tenant-1": source}, EngineOptions{
		Now: func() time.Time {
			value := nowValues[0]
			nowValues = nowValues[1:]
			return value
		},
	})

	if _, err := engine.RunRule(ctx, "tenant-1", "rule-1"); err != nil {
		t.Fatalf("first RunRule returned error: %v", err)
	}
	if _, err := engine.RunRule(ctx, "tenant-1", "rule-1"); err != nil {
		t.Fatalf("second RunRule returned error: %v", err)
	}
	if len(source.calls) != 2 {
		t.Fatalf("source calls = %d, want 2", len(source.calls))
	}
	if !source.calls[0].Since.IsZero() {
		t.Fatalf("first since = %s, want zero", source.calls[0].Since)
	}
	if !source.calls[1].Since.Equal(firstNow) {
		t.Fatalf("second since = %s, want %s", source.calls[1].Since, firstNow)
	}
	if len(rules.touched) != 2 || !rules.touched[0].At.Equal(firstNow) || !rules.touched[1].At.Equal(secondNow) {
		t.Fatalf("touches = %+v", rules.touched)
	}
}

func mustRule(t *testing.T, id, tenantID, repo string, include, exclude []string, issueState, dedupe string) *syncdomain.Rule {
	t.Helper()
	rule, err := syncdomain.NewRule(
		id,
		tenantID,
		repo,
		"coding",
		include,
		exclude,
		issueState,
		"normal",
		dedupe,
		true,
		time.Time{},
		time.Date(2026, 7, 7, 8, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 7, 8, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewRule returned error: %v", err)
	}
	return rule
}

type fakeRuleRepo struct {
	rules   map[string]*syncdomain.Rule
	touched []touchCall
}

type touchCall struct {
	TenantID string
	RuleID   string
	At       time.Time
}

func newFakeRuleRepo(rules ...*syncdomain.Rule) *fakeRuleRepo {
	repo := &fakeRuleRepo{rules: make(map[string]*syncdomain.Rule, len(rules))}
	for _, rule := range rules {
		repo.rules[ruleKey(rule.TenantID, rule.ID)] = cloneEngineTestRule(rule)
	}
	return repo
}

func (r *fakeRuleRepo) Create(ctx context.Context, rule *syncdomain.Rule) error {
	r.rules[ruleKey(rule.TenantID, rule.ID)] = cloneEngineTestRule(rule)
	return nil
}

func (r *fakeRuleRepo) Get(ctx context.Context, tenantID, id string) (*syncdomain.Rule, error) {
	rule, ok := r.rules[ruleKey(tenantID, id)]
	if !ok {
		return nil, syncdomain.ErrNotFound
	}
	return cloneEngineTestRule(rule), nil
}

func (r *fakeRuleRepo) List(ctx context.Context, tenantID string) ([]*syncdomain.Rule, error) {
	var out []*syncdomain.Rule
	for _, rule := range r.rules {
		if rule.TenantID == tenantID {
			out = append(out, cloneEngineTestRule(rule))
		}
	}
	return out, nil
}

func (r *fakeRuleRepo) ListEnabledAllTenants(ctx context.Context) ([]*syncdomain.Rule, error) {
	var out []*syncdomain.Rule
	for _, rule := range r.rules {
		if rule.Enabled {
			out = append(out, cloneEngineTestRule(rule))
		}
	}
	return out, nil
}

func (r *fakeRuleRepo) Update(ctx context.Context, rule *syncdomain.Rule) error {
	key := ruleKey(rule.TenantID, rule.ID)
	if _, ok := r.rules[key]; !ok {
		return syncdomain.ErrNotFound
	}
	r.rules[key] = cloneEngineTestRule(rule)
	return nil
}

func (r *fakeRuleRepo) Delete(ctx context.Context, tenantID, id string) error {
	key := ruleKey(tenantID, id)
	if _, ok := r.rules[key]; !ok {
		return syncdomain.ErrNotFound
	}
	delete(r.rules, key)
	return nil
}

func (r *fakeRuleRepo) TouchSynced(ctx context.Context, tenantID, id string, syncedAt time.Time) error {
	key := ruleKey(tenantID, id)
	rule, ok := r.rules[key]
	if !ok {
		return syncdomain.ErrNotFound
	}
	rule.LastSyncedAt = syncedAt
	rule.UpdatedAt = syncedAt
	r.touched = append(r.touched, touchCall{TenantID: tenantID, RuleID: id, At: syncedAt})
	return nil
}

func cloneEngineTestRule(rule *syncdomain.Rule) *syncdomain.Rule {
	if rule == nil {
		return nil
	}
	clone := *rule
	clone.IncludeLabels = append([]string(nil), rule.IncludeLabels...)
	clone.ExcludeLabels = append([]string(nil), rule.ExcludeLabels...)
	return &clone
}

func ruleKey(tenantID, id string) string {
	return tenantID + "/" + id
}

type fakeMapRepo struct {
	mappings map[mapKey]*Mapping
}

type mapKey struct {
	TenantID string
	Repo     string
	Number   int
}

func newFakeMapRepo() *fakeMapRepo {
	return &fakeMapRepo{mappings: make(map[mapKey]*Mapping)}
}

func (r *fakeMapRepo) Get(ctx context.Context, tenantID, repo string, issueNumber int) (*Mapping, error) {
	mapping, ok := r.mappings[mapKey{TenantID: tenantID, Repo: repo, Number: issueNumber}]
	if !ok {
		return nil, syncdomain.ErrNotFound
	}
	clone := *mapping
	return &clone, nil
}

func (r *fakeMapRepo) Upsert(ctx context.Context, mapping *Mapping) error {
	clone := *mapping
	r.mappings[mapKey{TenantID: mapping.TenantID, Repo: mapping.Repo, Number: mapping.IssueNumber}] = &clone
	return nil
}

func (r *fakeMapRepo) mustUpsert(t *testing.T, mapping *Mapping) {
	t.Helper()
	if err := r.Upsert(context.Background(), mapping); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
}

type fakeTaskSink struct {
	nextID    int
	statuses  map[string]string
	published []PublishSystemTaskInput
	updated   []updateCall
	cancelled []cancelCall
}

type updateCall struct {
	TenantID string
	TaskID   string
	Content  ContentInput
}

type cancelCall struct {
	TenantID string
	TaskID   string
	Reason   string
}

func newFakeTaskSink() *fakeTaskSink {
	return &fakeTaskSink{nextID: 1, statuses: make(map[string]string)}
}

func (s *fakeTaskSink) PublishSystemTask(ctx context.Context, in PublishSystemTaskInput) (string, error) {
	taskID := fmt.Sprintf("task-%d", s.nextID)
	s.nextID++
	s.published = append(s.published, in)
	s.statuses[taskID] = "open"
	return taskID, nil
}

func (s *fakeTaskSink) CancelSystemTask(ctx context.Context, tenantID, taskID, reason string) error {
	s.cancelled = append(s.cancelled, cancelCall{TenantID: tenantID, TaskID: taskID, Reason: reason})
	s.statuses[taskID] = "cancelled"
	return nil
}

func (s *fakeTaskSink) UpdateSystemTaskContent(ctx context.Context, tenantID, taskID string, in ContentInput) (string, error) {
	s.updated = append(s.updated, updateCall{TenantID: tenantID, TaskID: taskID, Content: in})
	return s.statuses[taskID], nil
}

func (s *fakeTaskSink) TaskStatus(ctx context.Context, tenantID, taskID string) (string, error) {
	status, ok := s.statuses[taskID]
	if !ok {
		return "", syncdomain.ErrNotFound
	}
	return status, nil
}

type fakeSources map[string]git.IssueSource

func (s fakeSources) IssueSource(ctx context.Context, tenantID string) (git.IssueSource, error) {
	source, ok := s[tenantID]
	if !ok {
		return nil, syncdomain.ErrNotFound
	}
	return source, nil
}

type recordingIssueSource struct {
	issues map[string][]git.Issue
	calls  []listIssuesCall
	err    error
}

type listIssuesCall struct {
	Repo   string
	Filter git.IssueFilter
	Since  time.Time
}

func (s *recordingIssueSource) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error) {
	if s.err != nil {
		return nil, s.err
	}
	return nil, nil
}

func (s *recordingIssueSource) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error) {
	s.calls = append(s.calls, listIssuesCall{Repo: repo, Filter: filter, Since: since})
	if s.err != nil {
		return nil, s.err
	}
	return s.issues[repo], nil
}

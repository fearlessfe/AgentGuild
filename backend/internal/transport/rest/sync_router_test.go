package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	syncapp "agentguild.dev/agentguild/backend/internal/sync/application"
	syncdomain "agentguild.dev/agentguild/backend/internal/sync/domain"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestSyncCreateRuleAdminSessionReturnsCreatedView(t *testing.T) {
	rules := newSyncRuleService(t)
	server := newTestServer(&fakeApplication{}, rest.WithSyncRuleService(rules))

	res := postJSONWithSession(t, server, "/v1/sync-rules", `{
		"repo":"agentguild/agentguild",
		"include_labels":["agent-task"],
		"exclude_labels":["blocked"],
		"issue_state":"open",
		"task_type":"coding",
		"default_priority":"normal",
		"dedupe_strategy":"update"
	}`, sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusCreated, res.Code)
	var body struct {
		Data syncapp.RuleView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "rule-1", body.Data.ID)
	require.Equal(t, "agentguild/agentguild", body.Data.Repo)
	require.Equal(t, []string{"agent-task"}, body.Data.IncludeLabels)
	require.Equal(t, []string{"blocked"}, body.Data.ExcludeLabels)
	require.Equal(t, "open", body.Data.IssueState)
	require.Equal(t, "coding", body.Data.TaskType)
	require.Equal(t, "normal", body.Data.DefaultPriority)
	require.Equal(t, "update", body.Data.DedupeStrategy)
	require.True(t, body.Data.Enabled)
}

func TestSyncCreateRuleAcceptsPublicSourceAuth(t *testing.T) {
	rules := newSyncRuleService(t)
	server := newTestServer(&fakeApplication{}, rest.WithSyncRuleService(rules))

	res := postJSONWithSession(t, server, "/v1/sync-rules", `{
		"repo":"octo/hello-world",
		"include_labels":["good first issue"],
		"issue_state":"open",
		"task_type":"coding",
		"dedupe_strategy":"update",
		"source_auth":"public"
	}`, sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusCreated, res.Code)
	var body struct {
		Data syncapp.RuleView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "public", body.Data.SourceAuth)
}

func TestSyncCreateRuleRejectsNonAdminHuman(t *testing.T) {
	rules := newSyncRuleService(t)
	server := newTestServer(&fakeApplication{}, rest.WithSyncRuleService(rules))

	res := postJSONWithSession(t, server, "/v1/sync-rules", `{
		"repo":"agentguild/agentguild",
		"issue_state":"open",
		"task_type":"coding",
		"dedupe_strategy":"update"
	}`, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusForbidden, res.Code)
	require.JSONEq(t, `{"error":{"code":"FORBIDDEN","message":"admin session is required"}}`, res.Body.String())
}

func TestSyncCreateRuleRejectsAgentBearer(t *testing.T) {
	rules := newSyncRuleService(t)
	server := newTestServer(&fakeApplication{}, rest.WithSyncRuleService(rules))

	res := postJSON(t, server, "/v1/sync-rules", `{
		"repo":"agentguild/agentguild",
		"issue_state":"open",
		"task_type":"coding",
		"dedupe_strategy":"update"
	}`, "token-agent-1")

	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, res.Code)
}

func TestSyncUpdateRuleCanToggleEnabled(t *testing.T) {
	rules := newSyncRuleService(t)
	server := newTestServer(&fakeApplication{}, rest.WithSyncRuleService(rules))
	cookie := sessionCookie(t, "admin-1", true)
	created := postJSONWithSession(t, server, "/v1/sync-rules", `{
		"repo":"agentguild/agentguild",
		"issue_state":"open",
		"task_type":"coding",
		"dedupe_strategy":"update"
	}`, cookie)
	require.Equal(t, http.StatusCreated, created.Code)
	var createdBody struct {
		Data syncapp.RuleView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdBody))

	updated := putJSONWithSession(t, server, "/v1/sync-rules/"+createdBody.Data.ID, `{
		"repo":"agentguild/agentguild",
		"issue_state":"open",
		"task_type":"coding",
		"dedupe_strategy":"update",
		"enabled":false
	}`, cookie)

	require.Equal(t, http.StatusOK, updated.Code)
	var updatedBody struct {
		Data syncapp.RuleView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(updated.Body.Bytes(), &updatedBody))
	require.False(t, updatedBody.Data.Enabled)
}

func TestSyncListRepositoriesReturnsInstallationRepositories(t *testing.T) {
	manager := &fakeGitHubAppManager{
		issueSource: &fakeSyncIssueSource{repos: []git.Repository{{
			FullName:      "agentguild/agentguild",
			DefaultBranch: "main",
			Visibility:    "private",
		}}},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := getWithSession(t, server, "/v1/repositories", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			Items []struct {
				FullName      string `json:"full_name"`
				DefaultBranch string `json:"default_branch"`
				Visibility    string `json:"visibility"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	require.Equal(t, "agentguild/agentguild", body.Data.Items[0].FullName)
	require.Equal(t, "main", body.Data.Items[0].DefaultBranch)
	require.Equal(t, "private", body.Data.Items[0].Visibility)
}

func TestSyncRunRuleReturnsSummary(t *testing.T) {
	rules := newSyncRuleService(t)
	engine := &fakeSyncEngine{result: syncapp.SyncResult{
		Created:   1,
		Updated:   2,
		Skipped:   3,
		Cancelled: 4,
		Failed:    5,
	}}
	server := newTestServer(&fakeApplication{},
		rest.WithSyncRuleService(rules),
		rest.WithSyncEngine(engine),
	)

	res := postJSONWithSession(t, server, "/v1/sync-rules/rule-123:run", `{}`, sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			Created   int `json:"created"`
			Updated   int `json:"updated"`
			Skipped   int `json:"skipped"`
			Cancelled int `json:"cancelled"`
			Failed    int `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, 1, body.Data.Created)
	require.Equal(t, 2, body.Data.Updated)
	require.Equal(t, 3, body.Data.Skipped)
	require.Equal(t, 4, body.Data.Cancelled)
	require.Equal(t, 5, body.Data.Failed)
	require.Equal(t, "tenant-1", engine.tenantID)
	require.Equal(t, "rule-123", engine.ruleID)
}

type fakeSyncIssueSource struct {
	repos []git.Repository
}

func (f *fakeSyncIssueSource) ListInstallationRepositories(ctx context.Context) ([]git.Repository, error) {
	return append([]git.Repository(nil), f.repos...), nil
}

func (f *fakeSyncIssueSource) ListIssues(ctx context.Context, repo string, filter git.IssueFilter, since time.Time) ([]git.Issue, error) {
	return nil, nil
}

type fakeSyncEngine struct {
	result   syncapp.SyncResult
	err      error
	tenantID string
	ruleID   string
}

func (f *fakeSyncEngine) RunRule(ctx context.Context, tenantID, ruleID string) (syncapp.SyncResult, error) {
	f.tenantID = tenantID
	f.ruleID = ruleID
	return f.result, f.err
}

func newSyncRuleService(t *testing.T) *syncapp.RuleService {
	t.Helper()
	var next int
	svc, err := syncapp.NewRuleService(&memoryRuleRepository{}, syncapp.RuleServiceOptions{
		NewID: func() string {
			next++
			return "rule-" + string(rune('0'+next))
		},
		Now: func() time.Time {
			return time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC)
		},
	})
	require.NoError(t, err)
	return svc
}

func putJSONWithSession(t *testing.T, server http.Handler, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

type memoryRuleRepository struct {
	mu    sync.Mutex
	rules map[string]*syncdomain.Rule
}

func (r *memoryRuleRepository) Create(ctx context.Context, rule *syncdomain.Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	r.rules[key(rule.TenantID, rule.ID)] = cloneRule(rule)
	return nil
}

func (r *memoryRuleRepository) Get(ctx context.Context, tenantID, id string) (*syncdomain.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	rule, ok := r.rules[key(tenantID, id)]
	if !ok {
		return nil, syncdomain.ErrNotFound
	}
	return cloneRule(rule), nil
}

func (r *memoryRuleRepository) List(ctx context.Context, tenantID string) ([]*syncdomain.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	out := make([]*syncdomain.Rule, 0, len(r.rules))
	prefix := tenantID + "/"
	for k, rule := range r.rules {
		if strings.HasPrefix(k, prefix) {
			out = append(out, cloneRule(rule))
		}
	}
	return out, nil
}

func (r *memoryRuleRepository) ListEnabledAllTenants(ctx context.Context) ([]*syncdomain.Rule, error) {
	return nil, nil
}

func (r *memoryRuleRepository) Update(ctx context.Context, rule *syncdomain.Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	r.rules[key(rule.TenantID, rule.ID)] = cloneRule(rule)
	return nil
}

func (r *memoryRuleRepository) Delete(ctx context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	delete(r.rules, key(tenantID, id))
	return nil
}

func (r *memoryRuleRepository) TouchSynced(ctx context.Context, tenantID, id string, t time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensure()
	rule, ok := r.rules[key(tenantID, id)]
	if !ok {
		return syncdomain.ErrNotFound
	}
	rule.LastSyncedAt = t
	return nil
}

func (r *memoryRuleRepository) ensure() {
	if r.rules == nil {
		r.rules = make(map[string]*syncdomain.Rule)
	}
}

func key(tenantID, id string) string {
	return tenantID + "/" + id
}

func cloneRule(rule *syncdomain.Rule) *syncdomain.Rule {
	if rule == nil {
		return nil
	}
	clone := *rule
	clone.IncludeLabels = append([]string(nil), rule.IncludeLabels...)
	clone.ExcludeLabels = append([]string(nil), rule.ExcludeLabels...)
	return &clone
}

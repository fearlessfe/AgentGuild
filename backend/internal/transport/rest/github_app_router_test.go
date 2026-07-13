package rest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

// fakeGitHubAppManager 是一个内存实现的 gitapp.GitHubAppManager，用于隔离路由层测试。
type fakeGitHubAppManager struct {
	calls                  []githubAppCall
	store                  map[string]*gitapp.GitHubAppRecord
	getErr                 error
	upsertErr              error
	deleteErr              error
	driverErr              error
	issueSource            git.IssueSource
	issueSourceErr         error
	installationAccount    string
	installationAccountErr error
}

func (f *fakeGitHubAppManager) InstallationAccount(ctx context.Context, tenantID, appID string, installationID int64) (string, error) {
	f.calls = append(f.calls, githubAppCall{method: "InstallationAccount", tenantID: tenantID, payload: appID})
	if f.installationAccountErr != nil {
		return "", f.installationAccountErr
	}
	return f.installationAccount, nil
}

type githubAppCall struct {
	method   string
	tenantID string
	payload  any
}

func (f *fakeGitHubAppManager) Driver(ctx context.Context, tenantID string) (git.Driver, error) {
	f.calls = append(f.calls, githubAppCall{method: "Driver", tenantID: tenantID})
	return nil, f.driverErr
}

func (f *fakeGitHubAppManager) DriverForApp(ctx context.Context, tenantID, appID string) (git.Driver, error) {
	f.calls = append(f.calls, githubAppCall{method: "DriverForApp", tenantID: tenantID, payload: appID})
	return nil, f.driverErr
}

func (f *fakeGitHubAppManager) IssueSource(ctx context.Context, tenantID string) (git.IssueSource, error) {
	f.calls = append(f.calls, githubAppCall{method: "IssueSource", tenantID: tenantID})
	if f.issueSourceErr != nil {
		return nil, f.issueSourceErr
	}
	if f.issueSource != nil {
		return f.issueSource, nil
	}
	return nil, git.ErrGitHubAppNotConfigured
}

func (f *fakeGitHubAppManager) IssueSourceForApp(ctx context.Context, tenantID, appID string) (git.IssueSource, error) {
	f.calls = append(f.calls, githubAppCall{method: "IssueSourceForApp", tenantID: tenantID, payload: appID})
	if f.issueSourceErr != nil {
		return nil, f.issueSourceErr
	}
	record, ok := f.store[tenantID]
	if !ok || record.ID != appID {
		return nil, git.ErrGitHubAppNotConfigured
	}
	if f.issueSource != nil {
		return f.issueSource, nil
	}
	return nil, git.ErrGitHubAppNotConfigured
}

func (f *fakeGitHubAppManager) Get(ctx context.Context, tenantID string) (gitapp.GitHubAppView, error) {
	f.calls = append(f.calls, githubAppCall{method: "Get", tenantID: tenantID})
	if f.getErr != nil {
		return gitapp.GitHubAppView{}, f.getErr
	}
	record, ok := f.store[tenantID]
	if !ok {
		return gitapp.GitHubAppView{}, git.ErrGitHubAppNotConfigured
	}
	return toGitHubAppView(record), nil
}

func (f *fakeGitHubAppManager) GetByID(ctx context.Context, tenantID, appID string) (gitapp.GitHubAppView, error) {
	f.calls = append(f.calls, githubAppCall{method: "GetByID", tenantID: tenantID, payload: appID})
	record, ok := f.store[tenantID]
	if !ok || record.ID != appID {
		return gitapp.GitHubAppView{}, git.ErrGitHubAppNotConfigured
	}
	return toGitHubAppView(record), nil
}

func (f *fakeGitHubAppManager) List(ctx context.Context, tenantID string) ([]gitapp.GitHubAppView, error) {
	f.calls = append(f.calls, githubAppCall{method: "List", tenantID: tenantID})
	record, ok := f.store[tenantID]
	if !ok {
		return []gitapp.GitHubAppView{}, nil
	}
	return []gitapp.GitHubAppView{toGitHubAppView(record)}, nil
}

func (f *fakeGitHubAppManager) Upsert(ctx context.Context, cmd gitapp.UpsertGitHubApp) error {
	f.calls = append(f.calls, githubAppCall{method: "Upsert", tenantID: cmd.TenantID, payload: cmd})
	if f.upsertErr != nil {
		return f.upsertErr
	}
	now := time.Now().UTC().Truncate(time.Second)
	id := cmd.ID
	isDefault := true
	if existing, ok := f.store[cmd.TenantID]; ok {
		if id == "" {
			id = existing.ID
		}
		isDefault = existing.IsDefault
	}
	if id == "" {
		id = "default"
	}
	f.store[cmd.TenantID] = &gitapp.GitHubAppRecord{
		ID:             id,
		TenantID:       cmd.TenantID,
		Provider:       cmd.Provider,
		AppID:          cmd.AppID,
		InstallationID: cmd.InstallationID,
		PrivateKey:     cmd.PrivateKey,
		BaseURL:        cmd.BaseURL,
		IsDefault:      isDefault,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return nil
}

func (f *fakeGitHubAppManager) InstallByID(ctx context.Context, tenantID, appID string, installationID int64, accountLogin string) (gitapp.GitHubAppView, error) {
	f.calls = append(f.calls, githubAppCall{method: "InstallByID", tenantID: tenantID, payload: appID})
	record, ok := f.store[tenantID]
	if !ok || record.ID != appID {
		return gitapp.GitHubAppView{}, git.ErrGitHubAppNotConfigured
	}
	copy := *record
	copy.InstallationID = installationID
	copy.InstallationAccountLogin = accountLogin
	copy.UpdatedAt = time.Now().UTC().Truncate(time.Second)
	f.store[tenantID] = &copy
	return toGitHubAppView(&copy), nil
}

func (f *fakeGitHubAppManager) Install(ctx context.Context, tenantID string, installationID int64) (gitapp.GitHubAppView, error) {
	f.calls = append(f.calls, githubAppCall{method: "Install", tenantID: tenantID, payload: installationID})
	record, ok := f.store[tenantID]
	if !ok {
		return gitapp.GitHubAppView{}, git.ErrGitHubAppNotConfigured
	}
	copy := *record
	copy.InstallationID = installationID
	now := time.Now().UTC().Truncate(time.Second)
	if copy.CreatedAt.IsZero() {
		copy.CreatedAt = now
	}
	copy.UpdatedAt = now
	f.store[tenantID] = &copy
	return toGitHubAppView(&copy), nil
}

func (f *fakeGitHubAppManager) Delete(ctx context.Context, tenantID string) error {
	f.calls = append(f.calls, githubAppCall{method: "Delete", tenantID: tenantID})
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.store, tenantID)
	return nil
}

func (f *fakeGitHubAppManager) DeleteByID(ctx context.Context, tenantID, appID string) error {
	f.calls = append(f.calls, githubAppCall{method: "DeleteByID", tenantID: tenantID, payload: appID})
	if f.deleteErr != nil {
		return f.deleteErr
	}
	record, ok := f.store[tenantID]
	if !ok || record.ID != appID {
		return git.ErrGitHubAppNotConfigured
	}
	delete(f.store, tenantID)
	return nil
}

func toGitHubAppView(record *gitapp.GitHubAppRecord) gitapp.GitHubAppView {
	if record == nil {
		return gitapp.GitHubAppView{Configured: false}
	}
	return gitapp.GitHubAppView{
		ID:                       record.ID,
		TenantID:                 record.TenantID,
		Provider:                 record.Provider,
		AppID:                    record.AppID,
		InstallationID:           record.InstallationID,
		BaseURL:                  record.BaseURL,
		AppSlug:                  record.AppSlug,
		InstallationAccountLogin: record.InstallationAccountLogin,
		IsDefault:                record.IsDefault,
		Configured:               true,
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.UpdatedAt,
	}
}

func TestGitHubApp_GetConfigured(t *testing.T) {
	fixed := time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-1": {
				TenantID:       "tenant-1",
				Provider:       "github",
				AppID:          123,
				InstallationID: 456,
				BaseURL:        "https://api.github.com",
				PrivateKey:     "super-secret-key",
				CreatedAt:      fixed,
				UpdatedAt:      fixed,
			},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := getWithSession(t, server, "/v1/github-app", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var view gitapp.GitHubAppView
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &view))
	require.Equal(t, "tenant-1", view.TenantID)
	require.Equal(t, "github", view.Provider)
	require.Equal(t, int64(123), view.AppID)
	require.Equal(t, int64(456), view.InstallationID)
	require.Equal(t, "https://api.github.com", view.BaseURL)
	require.True(t, view.Configured)
	require.Equal(t, fixed, view.CreatedAt)
	require.Equal(t, fixed, view.UpdatedAt)
	require.NotContains(t, res.Body.String(), "super-secret-key")
}

func TestGitHubApp_GetNotConfigured(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := getWithSession(t, server, "/v1/github-app", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusNotFound, res.Code)
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "NOT_CONFIGURED", body.Error.Code)
	require.Contains(t, body.Error.Message, "not configured")
}

func TestGitHubApp_Create(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))
	body := `{"app_id":1,"installation_id":2,"private_key":"private-key","base_url":"https://github.example.com/api/v3","provider":"github"}`

	res := postJSONWithSession(t, server, "/v1/github-app", body, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var view gitapp.GitHubAppView
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &view))
	require.Equal(t, "tenant-1", view.TenantID)
	require.Equal(t, int64(1), view.AppID)
	require.Equal(t, int64(2), view.InstallationID)
	require.Equal(t, "https://github.example.com/api/v3", view.BaseURL)
	require.True(t, view.Configured)
	require.NotContains(t, res.Body.String(), "private-key")

	require.Len(t, manager.calls, 2)
	require.Equal(t, "Upsert", manager.calls[0].method)
	cmd := manager.calls[0].payload.(gitapp.UpsertGitHubApp)
	require.Equal(t, "tenant-1", cmd.TenantID)
	require.Equal(t, int64(1), cmd.AppID)
	require.Equal(t, int64(2), cmd.InstallationID)
	require.Equal(t, "private-key", cmd.PrivateKey)
	require.Equal(t, "https://github.example.com/api/v3", cmd.BaseURL)
	require.Equal(t, "github", cmd.Provider)
	require.Equal(t, "Get", manager.calls[1].method)
}

func TestGitHubApp_Update(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-1": {
				TenantID:       "tenant-1",
				Provider:       "github",
				AppID:          1,
				InstallationID: 2,
				BaseURL:        "https://api.github.com",
				PrivateKey:     "old-key",
			},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))
	body := `{"app_id":99,"installation_id":88,"private_key":"new-key","base_url":"https://api.github.com","provider":"github"}`

	res := postJSONWithSession(t, server, "/v1/github-app", body, sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var view gitapp.GitHubAppView
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &view))
	require.Equal(t, int64(99), view.AppID)
	require.Equal(t, int64(88), view.InstallationID)
	require.True(t, view.Configured)
	require.Len(t, manager.calls, 2)
	require.Equal(t, "Upsert", manager.calls[0].method)
	require.Equal(t, int64(99), manager.calls[0].payload.(gitapp.UpsertGitHubApp).AppID)
}

func TestGitHubApp_Delete(t *testing.T) {
	manager := &fakeGitHubAppManager{
		store: map[string]*gitapp.GitHubAppRecord{
			"tenant-1": {
				TenantID:       "tenant-1",
				Provider:       "github",
				AppID:          1,
				InstallationID: 2,
				BaseURL:        "https://api.github.com",
				PrivateKey:     "key",
			},
		},
	}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	res := deleteWithSession(t, server, "/v1/github-app", sessionCookie(t, "owner-1", false))

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data struct {
			Deleted bool `json:"deleted"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.True(t, body.Data.Deleted)
	require.Len(t, manager.calls, 1)
	require.Equal(t, "Delete", manager.calls[0].method)
	require.Equal(t, "tenant-1", manager.calls[0].tenantID)
	require.NotContains(t, res.Body.String(), "key")
}

func TestGitHubApp_RequiresSession(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))

	t.Run("GET", func(t *testing.T) {
		res := get(t, server, "/v1/github-app", "token-publisher")
		require.Equal(t, http.StatusUnauthorized, res.Code)
		require.Contains(t, res.Body.String(), "missing or invalid session")
	})

	t.Run("POST", func(t *testing.T) {
		res := postJSON(t, server, "/v1/github-app", `{}`, "token-publisher")
		require.Equal(t, http.StatusUnauthorized, res.Code)
		require.Contains(t, res.Body.String(), "missing or invalid session")
	})

	t.Run("DELETE", func(t *testing.T) {
		res := deleteWithBearer(t, server, "/v1/github-app", "token-publisher")
		require.Equal(t, http.StatusUnauthorized, res.Code)
		require.Contains(t, res.Body.String(), "missing or invalid session")
	})

	require.Empty(t, manager.calls)
}

func deleteWithSession(t *testing.T, server http.Handler, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

func deleteWithBearer(t *testing.T, server http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}

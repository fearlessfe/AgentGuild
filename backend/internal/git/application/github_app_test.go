package application_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/git"
	"agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/git/github"
	"github.com/stretchr/testify/require"
)

func TestGitHubAppManagerRejectsNilRepository(t *testing.T) {
	manager, err := application.NewGitHubAppManager(nil)
	require.Error(t, err)
	require.Nil(t, manager)

	var derr *domain.Error
	require.ErrorAs(t, err, &derr)
	require.Equal(t, "github_app_repository", derr.Field)
}

func TestGitHubAppManagerListsTenantAppsWithoutSecrets(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)

	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-1", TenantID: "tenant-1", AppID: 1, PrivateKey: key, AppSlug: "alpha",
	}))
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-2", TenantID: "tenant-1", AppID: 2, PrivateKey: key, AppSlug: "beta",
	}))
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-other", TenantID: "tenant-2", AppID: 3, PrivateKey: key,
	}))

	views, err := manager.List(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, views, 2)
	require.Equal(t, []string{"gha-1", "gha-2"}, []string{views[0].ID, views[1].ID})
	require.Equal(t, []string{"alpha", "beta"}, []string{views[0].AppSlug, views[1].AppSlug})
	require.True(t, views[0].IsDefault)
	require.False(t, views[1].IsDefault)
}

func TestGitHubAppManagerGetByIDIsTenantScoped(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-1", TenantID: "tenant-1", AppID: 1, PrivateKey: key,
	}))

	view, err := manager.GetByID(ctx, "tenant-1", "gha-1")
	require.NoError(t, err)
	require.Equal(t, "gha-1", view.ID)

	_, err = manager.GetByID(ctx, "tenant-2", "gha-1")
	require.ErrorIs(t, err, git.ErrGitHubAppNotConfigured)
}

func TestGitHubAppManagerGeneratesIDWhenAbsent(t *testing.T) {
	repo := newMemoryGitHubAppRepo()
	manager, err := application.NewGitHubAppManagerWithOptions(repo, application.GitHubAppManagerOptions{
		NewID: func() string { return "gha-generated" },
	})
	require.NoError(t, err)

	err = manager.Upsert(context.Background(), application.UpsertGitHubApp{
		TenantID: "tenant-1", AppID: 1, PrivateKey: generateRSAPrivateKeyPEM(t),
	})
	require.NoError(t, err)

	view, err := manager.GetByID(context.Background(), "tenant-1", "gha-generated")
	require.NoError(t, err)
	require.Equal(t, "gha-generated", view.ID)
	require.True(t, view.IsDefault)
}

func TestGitHubAppManagerLegacyUpsertWithoutIDUpdatesDefault(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryGitHubAppRepo()
	ids := []string{"gha-first", "gha-unexpected"}
	manager, err := application.NewGitHubAppManagerWithOptions(repo, application.GitHubAppManagerOptions{
		NewID: func() string {
			id := ids[0]
			ids = ids[1:]
			return id
		},
	})
	require.NoError(t, err)
	key := generateRSAPrivateKeyPEM(t)

	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID: "tenant-1", AppID: 1, PrivateKey: key, AppSlug: "before",
	}))
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID: "tenant-1", AppID: 2, PrivateKey: key, AppSlug: "after",
	}))

	views, err := manager.List(ctx, "tenant-1")
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Equal(t, "gha-first", views[0].ID)
	require.Equal(t, int64(2), views[0].AppID)
	require.Equal(t, "after", views[0].AppSlug)
	require.True(t, views[0].IsDefault)
}

func TestGitHubAppManagerInstallsAppByID(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-1", TenantID: "tenant-1", AppID: 1, PrivateKey: key,
	}))
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-2", TenantID: "tenant-1", AppID: 2, PrivateKey: key,
	}))

	view, err := manager.InstallByID(ctx, "tenant-1", "gha-2", 42, "acme-corp")
	require.NoError(t, err)
	require.Equal(t, "gha-2", view.ID)
	require.Equal(t, int64(42), view.InstallationID)
	require.Equal(t, "acme-corp", view.InstallationAccountLogin)

	defaultView, err := manager.Get(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-1", defaultView.ID)
}

func TestGitHubAppManagerRejectsDeletingBoundApp(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryGitHubAppRepo()
	manager, err := application.NewGitHubAppManager(repo)
	require.NoError(t, err)
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-1", TenantID: "tenant-1", AppID: 1, PrivateKey: generateRSAPrivateKeyPEM(t),
	}))
	repo.bind("tenant-1", "gha-1")

	err = manager.DeleteByID(ctx, "tenant-1", "gha-1")
	var derr *domain.Error
	require.ErrorAs(t, err, &derr)
	require.Equal(t, "state_conflict", derr.Code)

	_, getErr := manager.GetByID(ctx, "tenant-1", "gha-1")
	require.NoError(t, getErr)
}

func TestGitHubAppManagerDeletingDefaultPromotesEarliestRemainingApp(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)
	for _, app := range []application.UpsertGitHubApp{
		{ID: "gha-1", TenantID: "tenant-1", AppID: 1, PrivateKey: key},
		{ID: "gha-2", TenantID: "tenant-1", AppID: 2, PrivateKey: key},
		{ID: "gha-3", TenantID: "tenant-1", AppID: 3, PrivateKey: key},
	} {
		require.NoError(t, manager.Upsert(ctx, app))
	}

	require.NoError(t, manager.DeleteByID(ctx, "tenant-1", "gha-1"))
	view, err := manager.Get(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "gha-2", view.ID)
	require.True(t, view.IsDefault)
}

func TestGitHubAppUpsertValidatesRequiredFields(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	validKey := generateRSAPrivateKeyPEM(t)

	cases := []struct {
		name string
		cmd  application.UpsertGitHubApp
	}{
		{
			name: "missing tenant_id",
			cmd:  application.UpsertGitHubApp{AppID: 1, InstallationID: 2, PrivateKey: validKey},
		},
		{
			name: "missing app_id",
			cmd:  application.UpsertGitHubApp{TenantID: "tenant-1", InstallationID: 2, PrivateKey: validKey},
		},
		{
			name: "missing private_key",
			cmd:  application.UpsertGitHubApp{TenantID: "tenant-1", AppID: 1, InstallationID: 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := manager.Upsert(ctx, tc.cmd)
			require.Error(t, err)

			var derr *domain.Error
			require.ErrorAs(t, err, &derr)
			require.Equal(t, "invalid_argument", derr.Code)
		})
	}
}

func TestGitHubAppUpsertAllowsZeroInstallationID(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)

	err := manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID:   "tenant-1",
		AppID:      1,
		PrivateKey: key,
	})
	require.NoError(t, err)

	view, err := manager.Get(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, int64(0), view.InstallationID)
	require.True(t, view.Configured)
}

func TestGitHubAppInstallPreservesAppCredentials(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)

	err := manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID:      "tenant-1",
		Provider:      "github",
		AppID:         1,
		PrivateKey:    key,
		BaseURL:       "https://api.github.com",
		WebhookSecret: "webhook-secret",
		ClientID:      "client-id",
		ClientSecret:  "client-secret",
		AppSlug:       "agentguild-test",
	})
	require.NoError(t, err)

	view, err := manager.Install(ctx, "tenant-1", 42)

	require.NoError(t, err)
	require.Equal(t, int64(42), view.InstallationID)
	require.Equal(t, "agentguild-test", view.AppSlug)

	reloaded, err := manager.Get(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, int64(42), reloaded.InstallationID)
	require.Equal(t, "agentguild-test", reloaded.AppSlug)
}

func TestGitHubAppUpsertDefaultsProviderAndBaseURL(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)

	err := manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID:       "tenant-1",
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     key,
	})
	require.NoError(t, err)

	view, err := manager.Get(ctx, "tenant-1")
	require.NoError(t, err)
	require.Equal(t, "github", view.Provider)
	require.Equal(t, "https://api.github.com", view.BaseURL)
}

func TestGitHubAppGetReturnsConfiguredViewWithoutPrivateKey(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)

	err := manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID:       "tenant-1",
		Provider:       "github",
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     key,
		BaseURL:        "https://github.example.com/api/v3",
	})
	require.NoError(t, err)

	view, err := manager.Get(ctx, "tenant-1")
	require.NoError(t, err)
	require.True(t, view.Configured)
	require.Equal(t, "tenant-1", view.TenantID)
	require.Equal(t, "github", view.Provider)
	require.Equal(t, int64(1), view.AppID)
	require.Equal(t, int64(2), view.InstallationID)
	require.Equal(t, "https://github.example.com/api/v3", view.BaseURL)
	require.False(t, view.CreatedAt.IsZero())
	require.False(t, view.UpdatedAt.IsZero())
}

func TestGitHubAppGetReturnsNotConfiguredWhenMissing(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)

	_, err := manager.Get(ctx, "tenant-missing")
	require.ErrorIs(t, err, git.ErrGitHubAppNotConfigured)
}

func TestGitHubAppDeleteRemovesConfig(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)

	err := manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID:       "tenant-1",
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     key,
	})
	require.NoError(t, err)

	err = manager.Delete(ctx, "tenant-1")
	require.NoError(t, err)

	_, err = manager.Get(ctx, "tenant-1")
	require.ErrorIs(t, err, git.ErrGitHubAppNotConfigured)
}

func TestGitHubAppDeleteMissingReturnsNotConfigured(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)

	err := manager.Delete(ctx, "tenant-missing")
	require.ErrorIs(t, err, git.ErrGitHubAppNotConfigured)
}

func TestGitHubAppDriverReturnsDriverWhenConfigured(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)

	err := manager.Upsert(ctx, application.UpsertGitHubApp{
		TenantID:       "tenant-1",
		AppID:          1,
		InstallationID: 2,
		PrivateKey:     key,
	})
	require.NoError(t, err)

	driver, err := manager.Driver(ctx, "tenant-1")
	require.NoError(t, err)
	require.NotNil(t, driver)

	_, ok := driver.(*github.Driver)
	require.True(t, ok, "driver should be a *github.Driver")
}

func TestGitHubAppManagerResolvesDriverAndIssueSourceByAppID(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)
	key := generateRSAPrivateKeyPEM(t)
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-1", TenantID: "tenant-1", AppID: 1, InstallationID: 11, PrivateKey: key,
	}))
	require.NoError(t, manager.Upsert(ctx, application.UpsertGitHubApp{
		ID: "gha-2", TenantID: "tenant-1", AppID: 2, InstallationID: 22, PrivateKey: key,
	}))

	driver, err := manager.DriverForApp(ctx, "tenant-1", "gha-2")
	require.NoError(t, err)
	require.IsType(t, &github.Driver{}, driver)

	source, err := manager.IssueSourceForApp(ctx, "tenant-1", "gha-2")
	require.NoError(t, err)
	require.IsType(t, &github.Driver{}, source)

	_, err = manager.DriverForApp(ctx, "tenant-2", "gha-2")
	require.ErrorIs(t, err, git.ErrGitHubAppNotConfigured)
}

func TestGitHubAppDriverReturnsNotConfiguredWhenMissing(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)

	driver, err := manager.Driver(ctx, "tenant-missing")
	require.ErrorIs(t, err, git.ErrGitHubAppNotConfigured)
	require.Nil(t, driver)
}

func newGitHubAppManager(t *testing.T) application.GitHubAppManager {
	t.Helper()
	manager, err := application.NewGitHubAppManagerWithOptions(newMemoryGitHubAppRepo(), application.GitHubAppManagerOptions{AllowedHosts: []string{"github.example.com", "ghe.example"}})
	require.NoError(t, err)
	return manager
}

// memoryGitHubAppRepository is an in-memory fake for GitHubAppRepository.
type memoryGitHubAppRepository struct {
	mu       sync.Mutex
	apps     map[string]map[string]*application.GitHubAppRecord
	bindings map[string]map[string]bool
	nowFn    func() time.Time
}

func newMemoryGitHubAppRepo() *memoryGitHubAppRepository {
	return &memoryGitHubAppRepository{
		apps:     make(map[string]map[string]*application.GitHubAppRecord),
		bindings: make(map[string]map[string]bool),
		nowFn:    func() time.Time { return time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC) },
	}
}

func (r *memoryGitHubAppRepository) Upsert(_ context.Context, record *application.GitHubAppRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.nowFn()
	if r.apps[record.TenantID] == nil {
		r.apps[record.TenantID] = make(map[string]*application.GitHubAppRecord)
	}
	if existing, ok := r.apps[record.TenantID][record.ID]; ok {
		record.CreatedAt = existing.CreatedAt
		record.IsDefault = existing.IsDefault
	} else {
		record.CreatedAt = now
		record.IsDefault = len(r.apps[record.TenantID]) == 0
	}
	record.UpdatedAt = now
	copy := *record
	r.apps[record.TenantID][record.ID] = &copy
	return nil
}

func (r *memoryGitHubAppRepository) ListByTenant(_ context.Context, tenantID string) ([]application.GitHubAppRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	records := make([]application.GitHubAppRecord, 0, len(r.apps[tenantID]))
	for _, record := range r.apps[tenantID] {
		records = append(records, *record)
	}
	slices.SortFunc(records, func(a, b application.GitHubAppRecord) int { return strings.Compare(a.ID, b.ID) })
	return records, nil
}

func (r *memoryGitHubAppRepository) GetByID(_ context.Context, tenantID, id string) (*application.GitHubAppRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.apps[tenantID][id]
	if !ok {
		return nil, git.ErrGitHubAppNotConfigured
	}
	copy := *record
	return &copy, nil
}

func (r *memoryGitHubAppRepository) GetDefault(_ context.Context, tenantID string) (*application.GitHubAppRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range r.apps[tenantID] {
		if record.IsDefault {
			copy := *record
			return &copy, nil
		}
	}
	return nil, git.ErrGitHubAppNotConfigured
}

func (r *memoryGitHubAppRepository) Delete(_ context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.apps[tenantID][id]; !ok {
		return git.ErrGitHubAppNotConfigured
	}
	delete(r.apps[tenantID], id)
	return nil
}

func (r *memoryGitHubAppRepository) DeleteAndPromoteDefault(_ context.Context, tenantID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.apps[tenantID][id]
	if !ok {
		return git.ErrGitHubAppNotConfigured
	}
	if r.bindings[tenantID][id] {
		return git.ErrGitHubAppInUse
	}
	delete(r.apps[tenantID], id)
	if !record.IsDefault || len(r.apps[tenantID]) == 0 {
		return nil
	}
	ids := make([]string, 0, len(r.apps[tenantID]))
	for appID := range r.apps[tenantID] {
		ids = append(ids, appID)
	}
	slices.Sort(ids)
	r.apps[tenantID][ids[0]].IsDefault = true
	return nil
}

func (r *memoryGitHubAppRepository) bind(tenantID, appID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.bindings[tenantID] == nil {
		r.bindings[tenantID] = make(map[string]bool)
	}
	r.bindings[tenantID][appID] = true
}

func generateRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return string(pem.EncodeToMemory(block))
}

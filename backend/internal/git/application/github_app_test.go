package application_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
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

func TestGitHubAppDriverReturnsNotConfiguredWhenMissing(t *testing.T) {
	ctx := context.Background()
	manager := newGitHubAppManager(t)

	driver, err := manager.Driver(ctx, "tenant-missing")
	require.ErrorIs(t, err, git.ErrGitHubAppNotConfigured)
	require.Nil(t, driver)
}

func newGitHubAppManager(t *testing.T) application.GitHubAppManager {
	t.Helper()
	manager, err := application.NewGitHubAppManager(newMemoryGitHubAppRepo())
	require.NoError(t, err)
	return manager
}

// memoryGitHubAppRepository is an in-memory fake for GitHubAppRepository.
type memoryGitHubAppRepository struct {
	mu    sync.Mutex
	apps  map[string]*application.GitHubAppRecord
	nowFn func() time.Time
}

func newMemoryGitHubAppRepo() *memoryGitHubAppRepository {
	return &memoryGitHubAppRepository{
		apps:  make(map[string]*application.GitHubAppRecord),
		nowFn: func() time.Time { return time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC) },
	}
}

func (r *memoryGitHubAppRepository) Upsert(_ context.Context, record *application.GitHubAppRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.nowFn()
	if existing, ok := r.apps[record.TenantID]; ok {
		record.CreatedAt = existing.CreatedAt
	} else {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	r.apps[record.TenantID] = record
	return nil
}

func (r *memoryGitHubAppRepository) GetByTenant(_ context.Context, tenantID string) (*application.GitHubAppRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	record, ok := r.apps[tenantID]
	if !ok {
		return nil, git.ErrGitHubAppNotConfigured
	}
	return record, nil
}

func (r *memoryGitHubAppRepository) Delete(_ context.Context, tenantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.apps[tenantID]; !ok {
		return git.ErrGitHubAppNotConfigured
	}
	delete(r.apps, tenantID)
	return nil
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

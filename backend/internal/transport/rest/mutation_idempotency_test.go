package rest_test

import (
	"context"
	"net/http"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/git"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	"agentguild.dev/agentguild/backend/internal/transport/rest"
	"github.com/stretchr/testify/require"
)

func TestOnboardingMutationRequiresIdempotencyKey(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))

	res := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, sessionCookie(t, "admin-1", true))

	require.Equal(t, http.StatusBadRequest, res.Code)
	require.Contains(t, res.Body.String(), "idempotency_key is required")
	require.Empty(t, svc.store.records["tenant-1"])
}

func TestOnboardingMutationReplaysExactSuccessfulResponse(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))
	cookie := sessionCookie(t, "admin-1", true)

	first := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, cookie, "Idempotency-Key", "replay-success")
	second := postJSONWithSession(t, server, "/v1/repositories/public", `{ "repo": "octo/hello-world" }`, cookie, "Idempotency-Key", "replay-success")

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, first.Code, second.Code)
	require.Equal(t, first.Body.String(), second.Body.String())
	require.Len(t, svc.store.records["tenant-1"], 1)
}

func TestOnboardingDeleteReplaysAfterResourceIsGone(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	require.NoError(t, svc.store.UpsertOnboardedRepository(context.Background(), &gitapp.OnboardedRepositoryRecord{ID: "repo-1", TenantID: "tenant-1", SourceType: gitapp.RepositorySourcePublicGitHub, FullName: "octo/hello-world"}))
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))
	cookie := sessionCookie(t, "admin-1", true)

	first := deleteWithSession(t, server, "/v1/repositories/repo-1", cookie, "Idempotency-Key", "delete-replay")
	second := deleteWithSession(t, server, "/v1/repositories/repo-1", cookie, "Idempotency-Key", "delete-replay")

	require.Equal(t, http.StatusOK, first.Code)
	require.Equal(t, first.Body.String(), second.Body.String())
}

func TestOnboardingMutationRejectsKeyMismatch(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/one"] = git.Repository{FullName: "octo/one", DefaultBranch: "main", Visibility: "public"}
	svc.public.repos["octo/two"] = git.Repository{FullName: "octo/two", DefaultBranch: "main", Visibility: "public"}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))
	cookie := sessionCookie(t, "admin-1", true)

	require.Equal(t, http.StatusCreated, postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/one"}`, cookie, "Idempotency-Key", "mismatch").Code)
	second := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/two"}`, cookie, "Idempotency-Key", "mismatch")

	require.Equal(t, http.StatusConflict, second.Code)
	require.Contains(t, second.Body.String(), "IDEMPOTENCY_MISMATCH")
}

func TestOnboardingMutationScopesKeyByActor(t *testing.T) {
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/one"] = git.Repository{FullName: "octo/one", DefaultBranch: "main", Visibility: "public"}
	svc.public.repos["octo/two"] = git.Repository{FullName: "octo/two", DefaultBranch: "main", Visibility: "public"}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service))

	one := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/one"}`, sessionCookie(t, "admin-1", true), "Idempotency-Key", "shared")
	two := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/two"}`, sessionCookie(t, "admin-2", true), "Idempotency-Key", "shared")

	require.Equal(t, http.StatusCreated, one.Code)
	require.Equal(t, http.StatusCreated, two.Code)
}

func TestOnboardingMutationScopesKeyByTenant(t *testing.T) {
	store := newMemoryIdempotencyStore()
	body := []byte(`{"repo":"octo/one"}`)
	hash := rest.CanonicalMutationRequestHash(http.MethodPost, "/v1/repositories/public", body)

	one, err := store.AcquireIdempotency(context.Background(), application.IdempotencyKey{TenantID: "tenant-1", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "shared"}, hash, fixedRepositoryTime())
	require.NoError(t, err)
	two, err := store.AcquireIdempotency(context.Background(), application.IdempotencyKey{TenantID: "tenant-2", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "shared"}, hash, fixedRepositoryTime())

	require.NoError(t, err)
	require.True(t, one.Acquired)
	require.True(t, two.Acquired)
	require.NotEqual(t, one.Key.TenantID, two.Key.TenantID)
}

func TestOnboardingMutationScopesKeyByOperation(t *testing.T) {
	store := newMemoryIdempotencyStore()
	hash := rest.CanonicalMutationRequestHash(http.MethodDelete, "/v1/repositories/repo-1", nil)
	base := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "admin-1", RequestID: "shared"}
	first := base
	first.Operation = "repository.delete"
	second := base
	second.Operation = "github_app.delete"

	one, err := store.AcquireIdempotency(context.Background(), first, hash, fixedRepositoryTime())
	require.NoError(t, err)
	two, err := store.AcquireIdempotency(context.Background(), second, hash, fixedRepositoryTime())

	require.NoError(t, err)
	require.True(t, one.Acquired)
	require.True(t, two.Acquired)
}

func TestOnboardingMutationReturnsConflictForPendingRequest(t *testing.T) {
	store := newMemoryIdempotencyStore()
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "pending"}
	hash := rest.CanonicalMutationRequestHash(http.MethodPost, "/v1/repositories/public", []byte(`{"repo":"octo/hello-world"}`))
	store.records[key] = &application.IdempotencyRecord{Key: key, RequestHash: hash, OwnerToken: "other"}
	svc := newRepositoryOnboardingService(t)
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service), rest.WithIdempotencyStore(store))

	res := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, sessionCookie(t, "admin-1", true), "Idempotency-Key", "pending")

	require.Equal(t, http.StatusConflict, res.Code)
	require.Contains(t, res.Body.String(), "STATE_CONFLICT")
}

func TestGitHubAppDeleteReplaysExactHandledConflict(t *testing.T) {
	manager := &fakeGitHubAppManager{store: map[string]*gitapp.GitHubAppRecord{}, deleteErr: git.ErrGitHubAppInUse}
	server := newTestServer(&fakeApplication{}, rest.WithGitHubAppManager(manager))
	cookie := sessionCookie(t, "admin-1", true)

	first := deleteWithSession(t, server, "/v1/github-apps/gha-1", cookie, "Idempotency-Key", "stable-error")
	second := deleteWithSession(t, server, "/v1/github-apps/gha-1", cookie, "Idempotency-Key", "stable-error")

	require.Equal(t, http.StatusConflict, first.Code)
	require.Equal(t, first.Code, second.Code)
	require.Equal(t, first.Body.String(), second.Body.String())
	require.Len(t, manager.calls, 1)
}

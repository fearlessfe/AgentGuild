package rest_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	one, err := store.AcquireMutationIdempotency(context.Background(), application.IdempotencyKey{TenantID: "tenant-1", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "shared"}, hash, time.Hour)
	require.NoError(t, err)
	two, err := store.AcquireMutationIdempotency(context.Background(), application.IdempotencyKey{TenantID: "tenant-2", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "shared"}, hash, time.Hour)

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

	one, err := store.AcquireMutationIdempotency(context.Background(), first, hash, time.Hour)
	require.NoError(t, err)
	two, err := store.AcquireMutationIdempotency(context.Background(), second, hash, time.Hour)

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
	require.Contains(t, res.Body.String(), "IDEMPOTENCY_IN_PROGRESS")
}

func TestOnboardingMutationCompletesAfterRequestContextCancellation(t *testing.T) {
	store := newMemoryIdempotencyStore()
	completionContext := make(chan error, 1)
	observed := &completionContextStore{memoryIdempotencyStore: store, completionContext: completionContext}
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}
	ctx, cancel := context.WithCancel(context.Background())
	cancellingService := &cancelingRepositoryOnboardingService{RepositoryOnboardingService: svc.service, cancel: cancel}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(cancellingService), rest.WithIdempotencyStore(observed))
	request := httptest.NewRequest(http.MethodPost, "/v1/repositories/public", strings.NewReader(`{"repo":"octo/hello-world"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "cancelled-completion")
	request.AddCookie(sessionCookie(t, "admin-1", true))
	request = request.WithContext(ctx)

	res := httptest.NewRecorder()
	server.ServeHTTP(res, request)

	require.Equal(t, http.StatusCreated, res.Code)
	select {
	case err := <-completionContext:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("completion was not attempted")
	}
}

func TestOnboardingMutationReturns500AndLeavesPendingWhenCompletionFails(t *testing.T) {
	store := newMemoryIdempotencyStore()
	failing := &failingCompletionStore{memoryIdempotencyStore: store}
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}
	server := newTestServer(&fakeApplication{}, rest.WithRepositoryOnboardingService(svc.service), rest.WithIdempotencyStore(failing))

	res := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, sessionCookie(t, "admin-1", true), "Idempotency-Key", "failed-completion")

	require.Equal(t, http.StatusInternalServerError, res.Code)
	require.Contains(t, res.Body.String(), "INTERNAL_ERROR")
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "failed-completion"}
	require.False(t, store.records[key].Completed)
	require.Nil(t, store.records[key].ResponseCode)
}

func TestOnboardingMutationRenewsLeaseUntilHandlerCompletes(t *testing.T) {
	store := newMemoryIdempotencyStore()
	store.leaseDuration = 35 * time.Millisecond
	store.renewed = make(chan struct{}, 1)
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}
	started := make(chan struct{})
	release := make(chan struct{})
	blocking := &blockingRepositoryOnboardingService{RepositoryOnboardingService: svc.service, started: started, release: release}
	server := newTestServer(
		&fakeApplication{},
		rest.WithRepositoryOnboardingService(blocking),
		rest.WithIdempotencyStore(store),
		rest.WithMutationIdempotencyTimings(10*time.Millisecond, 20*time.Millisecond),
	)
	cookie := sessionCookie(t, "admin-1", true)
	firstResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		firstResult <- postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, cookie, "Idempotency-Key", "long-running")
	}()
	<-started
	select {
	case <-store.renewed:
	case <-time.After(time.Second):
		t.Fatal("long-running handler lease was not renewed")
	}
	time.Sleep(store.leaseDuration + 15*time.Millisecond)

	second := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, cookie, "Idempotency-Key", "long-running")
	require.Equal(t, http.StatusConflict, second.Code)
	require.Contains(t, second.Body.String(), "IDEMPOTENCY_IN_PROGRESS")
	close(release)
	first := <-firstResult
	require.Equal(t, http.StatusCreated, first.Code)

	store.mu.Lock()
	renewalsAfterCompletion := store.renewals
	store.mu.Unlock()
	time.Sleep(35 * time.Millisecond)
	store.mu.Lock()
	require.Equal(t, renewalsAfterCompletion, store.renewals, "heartbeat continued after completion")
	store.mu.Unlock()
}

func TestOnboardingMutationRenewalFailureCancelsHandlerAndReturns500(t *testing.T) {
	store := newMemoryIdempotencyStore()
	store.renewErr = errors.New("renewal unavailable")
	svc := newRepositoryOnboardingService(t)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	blocking := &blockingRepositoryOnboardingService{RepositoryOnboardingService: svc.service, started: started, cancelled: cancelled}
	server := newTestServer(
		&fakeApplication{},
		rest.WithRepositoryOnboardingService(blocking),
		rest.WithIdempotencyStore(store),
		rest.WithMutationIdempotencyTimings(5*time.Millisecond, 20*time.Millisecond),
	)

	res := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, sessionCookie(t, "admin-1", true), "Idempotency-Key", "renew-failure")
	require.Equal(t, http.StatusInternalServerError, res.Code)
	require.Contains(t, res.Body.String(), "INTERNAL_ERROR")
	select {
	case <-started:
	default:
		t.Fatal("handler did not start")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("renewal failure did not cancel handler context")
	}
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "renew-failure"}
	store.mu.Lock()
	require.False(t, store.records[key].Completed)
	store.mu.Unlock()
}

func TestOnboardingMutationHandlerCompletionCancelsBlockedRenewal(t *testing.T) {
	store := &blockingRenewalStore{
		memoryIdempotencyStore: newMemoryIdempotencyStore(),
		started:                make(chan struct{}),
		cancelled:              make(chan struct{}),
	}
	svc := newRepositoryOnboardingService(t)
	svc.public.repos["octo/hello-world"] = git.Repository{FullName: "octo/hello-world", DefaultBranch: "main", Visibility: "public"}
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	blocking := &blockingRepositoryOnboardingService{
		RepositoryOnboardingService: svc.service,
		started:                     handlerStarted,
		release:                     releaseHandler,
	}
	server := newTestServer(
		&fakeApplication{},
		rest.WithRepositoryOnboardingService(blocking),
		rest.WithIdempotencyStore(store),
		rest.WithMutationIdempotencyTimings(time.Millisecond, 200*time.Millisecond),
	)
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, sessionCookie(t, "admin-1", true), "Idempotency-Key", "blocked-renewal")
	}()

	awaitEvent(t, handlerStarted, "handler did not start")
	awaitEvent(t, store.started, "renewal did not start")
	close(releaseHandler)
	awaitEvent(t, store.cancelled, "handler completion did not cancel the blocked renewal")
	res := awaitResult(t, result, "request did not finish after cancelling the blocked renewal")

	require.Equal(t, http.StatusCreated, res.Code)
	require.NotContains(t, res.Body.String(), "INTERNAL_ERROR")
	key := application.IdempotencyKey{TenantID: "tenant-1", ActorID: "admin-1", Operation: "repository.public.create", RequestID: "blocked-renewal"}
	store.mu.Lock()
	require.True(t, store.records[key].Completed)
	store.mu.Unlock()
}

func TestOnboardingMutationStopsHeartbeatWhenHandlerPanics(t *testing.T) {
	store := newMemoryIdempotencyStore()
	store.renewed = make(chan struct{}, 1)
	svc := newRepositoryOnboardingService(t)
	panicking := &panickingRepositoryOnboardingService{RepositoryOnboardingService: svc.service, after: store.renewed}
	server := newTestServer(
		&fakeApplication{},
		rest.WithRepositoryOnboardingService(panicking),
		rest.WithIdempotencyStore(store),
		rest.WithMutationIdempotencyTimings(5*time.Millisecond, 20*time.Millisecond),
	)

	res := postJSONWithSession(t, server, "/v1/repositories/public", `{"repo":"octo/hello-world"}`, sessionCookie(t, "admin-1", true), "Idempotency-Key", "panic-stop")
	require.Equal(t, http.StatusInternalServerError, res.Code)
	store.mu.Lock()
	renewalsAfterPanic := store.renewals
	store.mu.Unlock()
	time.Sleep(25 * time.Millisecond)
	store.mu.Lock()
	require.Equal(t, renewalsAfterPanic, store.renewals, "heartbeat leaked after handler panic")
	store.mu.Unlock()
}

type panickingRepositoryOnboardingService struct {
	*gitapp.RepositoryOnboardingService
	after <-chan struct{}
}

func (s *panickingRepositoryOnboardingService) AddPublicRepository(context.Context, gitapp.Principal, string) (gitapp.OnboardedRepositoryView, error) {
	<-s.after
	panic("handler panic")
}

type blockingRepositoryOnboardingService struct {
	*gitapp.RepositoryOnboardingService
	started   chan struct{}
	release   chan struct{}
	cancelled chan struct{}
}

func (s *blockingRepositoryOnboardingService) AddPublicRepository(ctx context.Context, principal gitapp.Principal, input string) (gitapp.OnboardedRepositoryView, error) {
	close(s.started)
	if s.release != nil {
		select {
		case <-s.release:
			return s.RepositoryOnboardingService.AddPublicRepository(ctx, principal, input)
		case <-ctx.Done():
			if s.cancelled != nil {
				close(s.cancelled)
			}
			return gitapp.OnboardedRepositoryView{}, ctx.Err()
		}
	}
	<-ctx.Done()
	if s.cancelled != nil {
		close(s.cancelled)
	}
	return gitapp.OnboardedRepositoryView{}, fmt.Errorf("handler cancelled: %w", ctx.Err())
}

type cancelingRepositoryOnboardingService struct {
	*gitapp.RepositoryOnboardingService
	cancel context.CancelFunc
}

func (s *cancelingRepositoryOnboardingService) AddPublicRepository(ctx context.Context, principal gitapp.Principal, input string) (gitapp.OnboardedRepositoryView, error) {
	view, err := s.RepositoryOnboardingService.AddPublicRepository(ctx, principal, input)
	s.cancel()
	return view, err
}

type completionContextStore struct {
	*memoryIdempotencyStore
	completionContext chan<- error
}

type blockingRenewalStore struct {
	*memoryIdempotencyStore
	started   chan struct{}
	cancelled chan struct{}
}

func (s *blockingRenewalStore) RenewIdempotency(ctx context.Context, _ application.IdempotencyKey, _ string) error {
	close(s.started)
	<-ctx.Done()
	close(s.cancelled)
	return ctx.Err()
}

func awaitEvent(t *testing.T, event <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func awaitResult(t *testing.T, result <-chan *httptest.ResponseRecorder, message string) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case response := <-result:
		return response
	case <-time.After(time.Second):
		t.Fatal(message)
		return nil
	}
}

type failingCompletionStore struct {
	*memoryIdempotencyStore
}

func (s *failingCompletionStore) CompleteIdempotency(context.Context, application.IdempotencyKey, string, int, []byte) error {
	return errors.New("completion unavailable")
}

func (s *completionContextStore) CompleteIdempotency(ctx context.Context, key application.IdempotencyKey, owner string, status int, body []byte) error {
	s.completionContext <- ctx.Err()
	return s.memoryIdempotencyStore.CompleteIdempotency(ctx, key, owner, status, body)
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

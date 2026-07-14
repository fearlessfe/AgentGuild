package rest

import (
	"context"
	"errors"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"github.com/stretchr/testify/require"
)

func TestRenewMutationHeartbeatSkipsTickStepAfterStop(t *testing.T) {
	store := &controlledMutationHeartbeatStore{renewed: make(chan struct{}, 1)}
	lifecycle, stop := context.WithCancel(context.Background())
	stop()

	err := renewMutationHeartbeat(
		lifecycle,
		store,
		application.IdempotencyKey{},
		"owner",
		time.Second,
	)

	require.NoError(t, err)
	select {
	case <-store.renewed:
		t.Fatal("renewal started after heartbeat was stopped")
	default:
	}
}

func TestRunMutationHeartbeatDoesNotSwallowStorageErrorConcurrentWithStop(t *testing.T) {
	storageErr := errors.New("owner fencing failed")
	releaseRenewal := make(chan struct{})
	store := &controlledMutationHeartbeatStore{
		renewed: make(chan struct{}, 1),
		renew: func(context.Context) error {
			<-releaseRenewal
			return storageErr
		},
	}
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()
	lifecycle, stop := context.WithCancel(context.Background())
	handlerCancelled := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		result <- runMutationHeartbeat(
			lifecycle,
			store,
			application.IdempotencyKey{},
			"owner",
			ticks,
			time.Second,
			func() { handlerCancelled <- struct{}{} },
		)
	}()

	select {
	case <-store.renewed:
	case <-time.After(time.Second):
		t.Fatal("renewal did not start")
	}
	stop()
	close(releaseRenewal)

	select {
	case err := <-result:
		require.ErrorIs(t, err, storageErr)
	case <-time.After(time.Second):
		t.Fatal("heartbeat did not report the storage error")
	}
	select {
	case <-handlerCancelled:
	case <-time.After(time.Second):
		t.Fatal("storage error did not cancel the handler")
	}
}

type controlledMutationHeartbeatStore struct {
	renewed chan struct{}
	renew   func(context.Context) error
}

func (s *controlledMutationHeartbeatStore) AcquireMutationIdempotency(context.Context, application.IdempotencyKey, [32]byte, time.Duration) (*application.IdempotencyRecord, error) {
	panic("unexpected acquisition")
}

func (s *controlledMutationHeartbeatStore) RenewIdempotency(ctx context.Context, _ application.IdempotencyKey, _ string) error {
	s.renewed <- struct{}{}
	if s.renew != nil {
		return s.renew(ctx)
	}
	return nil
}

func (s *controlledMutationHeartbeatStore) CompleteIdempotency(context.Context, application.IdempotencyKey, string, int, []byte) error {
	panic("unexpected completion")
}

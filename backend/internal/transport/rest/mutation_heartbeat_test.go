package rest

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"github.com/stretchr/testify/require"
)

func TestRunMutationHeartbeatSkipsReadyTickAfterStop(t *testing.T) {
	store := &controlledMutationHeartbeatStore{renewed: make(chan struct{}, 1)}
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()
	lifecycle, stop := context.WithCancel(context.Background())
	stop()
	handlerCancelled := make(chan struct{}, 1)

	err := runMutationHeartbeat(
		lifecycle,
		store,
		application.IdempotencyKey{},
		"owner",
		ticks,
		time.Second,
		func() { handlerCancelled <- struct{}{} },
	)

	require.NoError(t, err)
	select {
	case <-store.renewed:
		t.Fatal("renewal started after heartbeat was stopped")
	default:
	}
	select {
	case <-handlerCancelled:
		t.Fatal("normal heartbeat stop cancelled the handler")
	default:
	}
}

type controlledMutationHeartbeatStore struct {
	renewed chan struct{}
}

func (s *controlledMutationHeartbeatStore) AcquireMutationIdempotency(context.Context, application.IdempotencyKey, [32]byte, time.Duration) (*application.IdempotencyRecord, error) {
	panic("unexpected acquisition")
}

func (s *controlledMutationHeartbeatStore) RenewIdempotency(context.Context, application.IdempotencyKey, string) error {
	s.renewed <- struct{}{}
	return nil
}

func (s *controlledMutationHeartbeatStore) CompleteIdempotency(context.Context, application.IdempotencyKey, string, int, []byte) error {
	panic("unexpected completion")
}

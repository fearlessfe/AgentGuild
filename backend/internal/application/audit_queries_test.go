package application_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/ratelimit"
	"github.com/stretchr/testify/require"
)

func TestListTaskEventsIsTenantScopedAndSanitized(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen, CreatedAt: fixtureNow, UpdatedAt: fixtureNow})
	tx.events = append(tx.events, application.TaskEvent{
		TenantID: "tenant-1", TaskID: "task", ExecutionID: "exe-1",
		ActorType: "agent", ActorID: "agent-1", Intent: "claim",
		FromState: "open", ToState: "claimed", Reason: "wanted",
		Payload:   []byte(`{"secret":"x"}`), CreatedAt: fixtureNow,
	})

	page, err := svc.ListTaskEvents(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTaskEvents{TaskID: "task", Limit: 10})
	require.NoError(t, err)
	require.Len(t, page.Data.Events, 1)
	e := page.Data.Events[0]
	require.Equal(t, "claim", e.Intent)
	require.Equal(t, "agent", e.ActorType)
	require.Equal(t, "wanted", e.Reason)
	// Payload 不应在摘要中暴露；Reason 可以保留。

	_, err = svc.ListTaskEvents(context.Background(), principal("tenant-2", "agent-1", "tasks:read"), application.ListTaskEvents{TaskID: "task", Limit: 10})
	require.Error(t, err)
	require.Equal(t, "not_found", domain.CodeOf(err))
}

func TestListTaskEventsRejectsMissingTaskIDAndInvalidLimits(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen, CreatedAt: fixtureNow, UpdatedAt: fixtureNow})
	_, err := svc.ListTaskEvents(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTaskEvents{Limit: 10})
	require.Equal(t, "invalid_argument", domain.CodeOf(err))

	_, err = svc.ListTaskEvents(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTaskEvents{TaskID: "task", Limit: 101})
	require.Equal(t, "invalid_argument", domain.CodeOf(err))
}

func TestListTaskEventsRequiresReadScope(t *testing.T) {
	svc, _ := newServiceFixture()
	_, err := svc.ListTaskEvents(context.Background(), principal("tenant-1", "agent-1", "tasks:execute"), application.ListTaskEvents{TaskID: "task", Limit: 10})
	require.Equal(t, "forbidden", domain.CodeOf(err))
}

func TestRateLimitRejectsOverLimit(t *testing.T) {
	limiter := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{
		Rate:  100 * time.Millisecond,
		Burst: 1,
	})
	svc, tx := newServiceFixtureWithRateLimiter(limiter)
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen, Deadline: fixtureNow.Add(time.Hour)})

	_, err := svc.ClaimTask(context.Background(), principal("tenant-1", "agent-1", "tasks:claim"), application.ClaimTask{RequestID: "r1", TaskID: "task"})
	require.NoError(t, err)

	_, err = svc.ClaimTask(context.Background(), principal("tenant-1", "agent-1", "tasks:claim"), application.ClaimTask{RequestID: "r2", TaskID: "task"})
	require.Equal(t, "rate_limited", domain.CodeOf(err))
}

func newServiceFixtureWithRateLimiter(limiter ratelimit.RateLimiter) (*application.Service, *fakeTx) {
	tx := newFakeTx()
	next := 0
	svc, err := application.NewService(&fakeStore{tx: tx}, application.Options{
		CursorSecret: []byte("01234567890123456789012345678901"),
		CursorTTL:    time.Hour,
		NewID:        func() string { next++; return "id-" + string(rune('0'+next)) },
		RateLimiter:  limiter,
	})
	if err != nil {
		panic(err)
	}
	return svc, tx
}

package application_test

import (
	"context"
	"encoding/json"
	"strconv"
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
		Payload: []byte(`{"secret":"x"}`), CreatedAt: fixtureNow,
	})

	page, err := svc.ListTaskEvents(context.Background(), principal("tenant-1", "agent-1", "tasks:read"), application.ListTaskEvents{TaskID: "task", Limit: 10})
	require.NoError(t, err)
	require.Len(t, page.Data.Events, 1)
	e := page.Data.Events[0]
	require.Equal(t, "claim", e.Intent)
	require.Equal(t, "agent", e.ActorType)
	body, err := json.Marshal(e)
	require.NoError(t, err)
	require.NotContains(t, string(body), "wanted")
	require.NotContains(t, string(body), "reason")

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

func TestListTaskEventsPaginatesWithoutRepeatingEvents(t *testing.T) {
	svc, tx := newServiceFixture()
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen})
	for i := 0; i < 3; i++ {
		tx.events = append(tx.events, application.TaskEvent{
			TenantID: "tenant-1", TaskID: "task", Intent: "event-" + strconv.Itoa(i), CreatedAt: fixtureNow.Add(time.Duration(i) * time.Nanosecond),
		})
	}
	p := principal("tenant-1", "agent-1", "tasks:read")
	first, err := svc.ListTaskEvents(context.Background(), p, application.ListTaskEvents{TaskID: "task", Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Data.Events, 2)
	require.NotEmpty(t, first.Data.NextCursor)
	afterID, err := strconv.ParseInt(first.Data.NextCursor, 10, 64)
	require.NoError(t, err)
	second, err := svc.ListTaskEvents(context.Background(), p, application.ListTaskEvents{TaskID: "task", Limit: 2, AfterID: afterID})
	require.NoError(t, err)
	require.Len(t, second.Data.Events, 1)
	require.Equal(t, "event-2", second.Data.Events[0].Intent)
}

func TestListTaskEventsAppliesApplicationRateLimit(t *testing.T) {
	limiter, err := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{Rate: time.Second, Burst: 1})
	require.NoError(t, err)
	svc, tx := newServiceFixtureWithRateLimiter(limiter)
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen})
	p := principal("tenant-1", "agent-1", "tasks:read")
	_, err = svc.ListTaskEvents(context.Background(), p, application.ListTaskEvents{TaskID: "task"})
	require.NoError(t, err)
	_, err = svc.ListTaskEvents(context.Background(), p, application.ListTaskEvents{TaskID: "task"})
	require.Equal(t, "rate_limited", domain.CodeOf(err))
}

func TestRateLimitRejectsOverLimit(t *testing.T) {
	limiter, err := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{
		Rate:  100 * time.Millisecond,
		Burst: 1,
	})
	require.NoError(t, err)
	svc, tx := newServiceFixtureWithRateLimiter(limiter)
	tx.seed(application.TaskRecord{ID: "task", TenantID: "tenant-1", PublisherAgentVersionID: "publisher", Status: domain.TaskOpen, Deadline: fixtureNow.Add(time.Hour)})

	_, err = svc.ClaimTask(context.Background(), principal("tenant-1", "agent-1", "tasks:claim"), application.ClaimTask{RequestID: "r1", TaskID: "task"})
	require.NoError(t, err)

	_, err = svc.ClaimTask(context.Background(), principal("tenant-1", "agent-1", "tasks:claim"), application.ClaimTask{RequestID: "r2", TaskID: "task"})
	require.Equal(t, "rate_limited", domain.CodeOf(err))
	require.Greater(t, domain.RetryAfterOf(err), time.Duration(0))
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

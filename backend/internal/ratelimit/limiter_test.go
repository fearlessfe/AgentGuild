package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/ratelimit"
	"github.com/stretchr/testify/require"
)

func TestRateLimiterUnlimitedAlwaysAllows(t *testing.T) {
	l := ratelimit.NewUnlimited()
	for i := 0; i < 100; i++ {
		d, err := l.Allow(context.Background(), ratelimit.Key{TenantID: "t", AgentVersionID: "a"})
		require.NoError(t, err)
		require.True(t, d.Allowed)
	}
}

func TestRateLimiterLocalTokenBucketAllowsWithinBurst(t *testing.T) {
	l := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{
		Rate:  100 * time.Millisecond,
		Burst: 3,
	})
	key := ratelimit.Key{TenantID: "t", AgentVersionID: "a"}
	for i := 0; i < 3; i++ {
		d, err := l.Allow(context.Background(), key)
		require.NoError(t, err)
		require.True(t, d.Allowed)
	}
	d, err := l.Allow(context.Background(), key)
	require.NoError(t, err)
	require.False(t, d.Allowed)
	require.Greater(t, d.RetryAfter, time.Duration(0))
}

func TestRateLimiterLocalTokenBucketRefillsOverTime(t *testing.T) {
	l := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{
		Rate:  50 * time.Millisecond,
		Burst: 1,
	})
	key := ratelimit.Key{TenantID: "t", AgentVersionID: "a"}
	_, err := l.Allow(context.Background(), key)
	require.NoError(t, err)

	d, err := l.Allow(context.Background(), key)
	require.NoError(t, err)
	require.False(t, d.Allowed)

	time.Sleep(60 * time.Millisecond)
	d, err = l.Allow(context.Background(), key)
	require.NoError(t, err)
	require.True(t, d.Allowed)
}

func TestRateLimiterLocalTokenBucketIsKeyScoped(t *testing.T) {
	l := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{
		Rate:  100 * time.Millisecond,
		Burst: 1,
	})
	d, err := l.Allow(context.Background(), ratelimit.Key{TenantID: "t1", AgentVersionID: "a"})
	require.NoError(t, err)
	require.True(t, d.Allowed)

	d, err = l.Allow(context.Background(), ratelimit.Key{TenantID: "t2", AgentVersionID: "a"})
	require.NoError(t, err)
	require.True(t, d.Allowed)
}

func TestRateLimiterLocalTokenBucketKeysByTenantAndAgentAcrossVersions(t *testing.T) {
	l := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{Rate: time.Second, Burst: 1})

	first, err := l.Allow(context.Background(), ratelimit.Key{TenantID: "t", AgentID: "agent", AgentVersionID: "v1"})
	require.NoError(t, err)
	require.True(t, first.Allowed)

	second, err := l.Allow(context.Background(), ratelimit.Key{TenantID: "t", AgentID: "agent", AgentVersionID: "v2"})
	require.NoError(t, err)
	require.False(t, second.Allowed, "同一 tenant+Agent 的不同版本必须共享配额")
}

func TestRateLimiterLocalTokenBucketPerAgentScope(t *testing.T) {
	l := ratelimit.NewLocalTokenBucket(ratelimit.TokenBucketConfig{
		Rate:  100 * time.Millisecond,
		Burst: 1,
	})
	d, err := l.Allow(context.Background(), ratelimit.Key{TenantID: "t", AgentID: "agent-1", AgentVersionID: "v1"})
	require.NoError(t, err)
	require.True(t, d.Allowed)

	d, err = l.Allow(context.Background(), ratelimit.Key{TenantID: "t", AgentID: "agent-1", AgentVersionID: "v2"})
	require.NoError(t, err)
	require.False(t, d.Allowed)
}

// ratelimit 提供可替换的限流抽象与本地令牌桶实现。
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Key 标识一个限流桶。
type Key struct {
	TenantID       string
	AgentID        string
	AgentVersionID string
}

// Decision 是限流决策结果。
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration
}

// RateLimiter 是限流器接口，可由本地令牌桶、Redis 或网关实现。
type RateLimiter interface {
	Allow(ctx context.Context, key Key) (Decision, error)
}

// NewUnlimited 返回一个始终允许的限流器，用于测试与关闭限流场景。
func NewUnlimited() RateLimiter {
	return &unlimited{}
}

type unlimited struct{}

func (u *unlimited) Allow(context.Context, Key) (Decision, error) {
	return Decision{Allowed: true}, nil
}

// TokenBucketConfig 配置本地令牌桶。
type TokenBucketConfig struct {
	Rate  time.Duration // 每个令牌产生间隔
	Burst int           // 桶容量
}

// NewLocalTokenBucket 创建进程内令牌桶限流器。
// 注意：该实现不提供跨实例全局配额。
func NewLocalTokenBucket(cfg TokenBucketConfig) (RateLimiter, error) {
	if cfg.Rate <= 0 {
		return nil, fmt.Errorf("rate must be positive")
	}
	if cfg.Burst <= 0 {
		return nil, fmt.Errorf("burst must be positive")
	}
	return &localBucket{
		cfg:     cfg,
		buckets: make(map[string]*bucket),
	}, nil
}

type localBucket struct {
	cfg     TokenBucketConfig
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

func (l *localBucket) Allow(_ context.Context, key Key) (Decision, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	id := l.key(key)
	b, ok := l.buckets[id]
	now := time.Now()
	if !ok {
		b = &bucket{tokens: float64(l.cfg.Burst) - 1, last: now}
		l.buckets[id] = b
		return Decision{Allowed: true}, nil
	}

	elapsed := now.Sub(b.last)
	if elapsed > 0 {
		refill := float64(elapsed) / float64(l.cfg.Rate)
		b.tokens = min(float64(l.cfg.Burst), b.tokens+refill)
		b.last = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return Decision{Allowed: true}, nil
	}

	// 计算到下一个令牌可用的等待时间。
	deficit := 1 - b.tokens
	retryAfter := time.Duration(deficit * float64(l.cfg.Rate))
	if retryAfter <= 0 {
		retryAfter = l.cfg.Rate
	}
	return Decision{Allowed: false, RetryAfter: retryAfter}, nil
}

func (l *localBucket) key(key Key) string {
	return key.TenantID + "|" + key.AgentID
}

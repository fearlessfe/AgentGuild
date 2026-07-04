package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	"agentguild.dev/agentguild/backend/internal/ratelimit"
)

type Options struct {
	CursorSecret       []byte
	CursorTTL          time.Duration
	NewID              func() string
	RateLimiter        ratelimit.RateLimiter
	AgentStatusChecker AgentStatusChecker
}

type AgentStatusChecker interface {
	CheckAgentStatus(context.Context, auth.Principal) error
}

type Service struct {
	store              Store
	policy             auth.ScopePolicy
	cursorSecret       []byte
	cursorTTL          time.Duration
	newID              func() string
	rateLimiter        ratelimit.RateLimiter
	agentStatusChecker AgentStatusChecker
}

func NewService(store Store, options Options) (*Service, error) {
	if len(options.CursorSecret) < 32 {
		return nil, invalid("cursor_secret")
	}
	if options.CursorTTL <= 0 {
		options.CursorTTL = 15 * time.Minute
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.RateLimiter == nil {
		options.RateLimiter = ratelimit.NewUnlimited()
	}
	return &Service{
		store:              store,
		cursorSecret:       append([]byte(nil), options.CursorSecret...),
		cursorTTL:          options.CursorTTL,
		newID:              options.NewID,
		rateLimiter:        options.RateLimiter,
		agentStatusChecker: options.AgentStatusChecker,
	}, nil
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func (s *Service) checkRateLimit(ctx context.Context, principal auth.Principal) error {
	decision, err := s.rateLimiter.Allow(ctx, ratelimit.Key{TenantID: principal.TenantID, AgentID: principal.AgentID, AgentVersionID: principal.AgentVersionID})
	if err != nil {
		return err
	}
	if !decision.Allowed {
		return &domain.Error{Code: "rate_limited", Message: "rate limit exceeded", RetryAfter: decision.RetryAfter}
	}
	return nil
}

func (s *Service) requireLiveAgent(ctx context.Context, principal auth.Principal) error {
	if s.agentStatusChecker == nil || principal.Type != auth.PrincipalTypeAgent {
		return nil
	}
	return s.agentStatusChecker.CheckAgentStatus(ctx, principal)
}

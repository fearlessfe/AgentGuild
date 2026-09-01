package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	participationdomain "agentguild.dev/agentguild/backend/internal/participation/domain"
	"agentguild.dev/agentguild/backend/internal/ratelimit"
)

type Options struct {
	CursorSecret      []byte
	CursorTTL         time.Duration
	NewID             func() string
	RateLimiter       ratelimit.RateLimiter
	IssueSourceLookup IssueSourceLookup
	Participation     ParticipationAuthorizer
}

type ParticipationAuthorizer interface {
	Authorize(context.Context, auth.Principal, participationdomain.ResourceKind, string, participationdomain.Scope) (*participationdomain.Grant, error)
}

type Service struct {
	store             Store
	policy            auth.ScopePolicy
	resources         auth.ResourcePolicy
	cursorSecret      []byte
	cursorTTL         time.Duration
	newID             func() string
	rateLimiter       ratelimit.RateLimiter
	issueSourceLookup IssueSourceLookup
	participation     ParticipationAuthorizer
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
		store:             store,
		cursorSecret:      append([]byte(nil), options.CursorSecret...),
		cursorTTL:         options.CursorTTL,
		newID:             options.NewID,
		rateLimiter:       options.RateLimiter,
		issueSourceLookup: options.IssueSourceLookup,
		participation:     options.Participation,
	}, nil
}

func (s *Service) authorizeResource(ctx context.Context, principal auth.Principal, kind participationdomain.ResourceKind, resourceID string, scope participationdomain.Scope) (string, bool, error) {
	if principal.IsGlobalAgent() {
		if s.participation == nil {
			return "", true, domain.ErrForbidden
		}
		grant, err := s.participation.Authorize(ctx, principal, kind, resourceID, scope)
		if err != nil {
			return "", true, err
		}
		return grant.ResourceTenantID, true, nil
	}
	tenantID, err := s.resources.Tenant(principal)
	return tenantID, false, err
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

func (s *Service) requireLiveAgent(ctx context.Context, tx Tx, principal auth.Principal) error {
	if principal.Type != auth.PrincipalTypeAgent {
		return nil
	}
	return tx.RequireLiveAgent(ctx, principal)
}

// WithTx runs the given function inside a transaction backed by the core store.
// It allows the core service to act as the store for cross-module notifiers.
func (s *Service) WithTx(ctx context.Context, fn func(Tx) error) error {
	return s.store.WithTx(ctx, fn)
}

// CoreExecutionNotifier implements git/application.ExecutionNotifier by
// loading the execution from the core store and applying the requested intent.
type CoreExecutionNotifier struct {
	store Store
}

// NewCoreExecutionNotifier creates a notifier backed by the core store.
func NewCoreExecutionNotifier(store Store) *CoreExecutionNotifier {
	return &CoreExecutionNotifier{store: store}
}

// Notify applies the state intent to the execution in a single transaction.
// 被领域状态机拒绝的迁移会在独立事务中追加审计事件。
func (n *CoreExecutionNotifier) Notify(ctx context.Context, cmd gitapp.ExecutionStateCommand, now time.Time) error {
	if cmd.TenantID == "" || cmd.ExecutionID == "" {
		return domain.ErrForbidden
	}
	var rejection *rejectionAudit
	err := n.store.WithTx(ctx, func(tx Tx) error {
		execution, version, err := tx.GetExecution(ctx, cmd.TenantID, cmd.ExecutionID)
		if err != nil {
			return err
		}
		if executionTransitionAlreadyApplied(execution.Status, cmd.Intent) {
			return nil
		}
		if err := execution.Apply(cmd.Intent, cmd.Actor, now); err != nil {
			rejection = &rejectionAudit{taskID: execution.TaskID, executionID: execution.ID, actorType: cmd.Actor.Type, actorID: cmd.Actor.ID, intent: intentName(cmd.Intent), fromState: string(execution.Status), reason: domain.CodeOf(err)}
			return err
		}
		updated, err := tx.UpdateExecution(ctx, execution, version)
		if err != nil {
			return err
		}
		if !updated {
			return domain.ErrStateConflict
		}
		return nil
	})
	if err != nil && rejection != nil {
		auditRejection(ctx, n.store, cmd.TenantID, *rejection)
	}
	return err
}

func executionTransitionAlreadyApplied(status domain.ExecutionStatus, intent domain.Intent) bool {
	switch intent {
	case domain.IntentSubmit:
		return status == domain.ExecutionSubmitted || status == domain.ExecutionValidating ||
			status == domain.ExecutionValidationFailed || status == domain.ExecutionReviewing ||
			status == domain.ExecutionRevisionRequested || status == domain.ExecutionAccepted ||
			status == domain.ExecutionRejected
	case domain.IntentStartValidation:
		return status == domain.ExecutionValidating || status == domain.ExecutionValidationFailed ||
			status == domain.ExecutionReviewing || status == domain.ExecutionRevisionRequested ||
			status == domain.ExecutionAccepted || status == domain.ExecutionRejected
	case domain.IntentFailValidation:
		return status == domain.ExecutionValidationFailed
	case domain.IntentMarkReviewing:
		return status == domain.ExecutionReviewing || status == domain.ExecutionRevisionRequested ||
			status == domain.ExecutionAccepted || status == domain.ExecutionRejected
	default:
		return false
	}
}

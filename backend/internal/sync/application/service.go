package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/sync/domain"
)

type RuleService struct {
	rules RuleRepository
	newID func() string
	now   func() time.Time
}

type RuleServiceOptions struct {
	NewID func() string
	Now   func() time.Time
}

type CreateRuleCommand struct {
	Repo            string
	IncludeLabels   []string
	ExcludeLabels   []string
	IssueState      string
	TaskType        string
	DefaultPriority string
	DedupeStrategy  string
	SourceAuth      string
}

type UpdateRuleCommand struct {
	ID              string
	Repo            string
	IncludeLabels   []string
	ExcludeLabels   []string
	IssueState      string
	TaskType        string
	DefaultPriority string
	DedupeStrategy  string
	SourceAuth      string
	Enabled         *bool
}

type RuleView struct {
	ID              string    `json:"id"`
	Repo            string    `json:"repo"`
	IncludeLabels   []string  `json:"include_labels"`
	ExcludeLabels   []string  `json:"exclude_labels"`
	IssueState      string    `json:"issue_state"`
	TaskType        string    `json:"task_type"`
	DefaultPriority string    `json:"default_priority"`
	DedupeStrategy  string    `json:"dedupe_strategy"`
	SourceAuth      string    `json:"source_auth"`
	Enabled         bool      `json:"enabled"`
	LastSyncedAt    time.Time `json:"last_synced_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewRuleService(rules RuleRepository, options RuleServiceOptions) (*RuleService, error) {
	if rules == nil {
		return nil, invalidArg("rule_repository")
	}
	if options.NewID == nil {
		options.NewID = randomID
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &RuleService{rules: rules, newID: options.NewID, now: options.Now}, nil
}

func (s *RuleService) Create(ctx context.Context, principal auth.Principal, cmd CreateRuleCommand) (RuleView, error) {
	if err := requireAdminHuman(principal); err != nil {
		return RuleView{}, err
	}
	now := s.now()
	rule, err := domain.NewRule(
		s.newID(),
		principal.TenantID,
		cmd.Repo,
		cmd.TaskType,
		cmd.IncludeLabels,
		cmd.ExcludeLabels,
		cmd.IssueState,
		cmd.DefaultPriority,
		cmd.DedupeStrategy,
		cmd.SourceAuth,
		true,
		time.Time{},
		now,
		now,
	)
	if err != nil {
		return RuleView{}, err
	}
	if err := s.rules.Create(ctx, rule); err != nil {
		return RuleView{}, err
	}
	return toRuleView(rule), nil
}

func (s *RuleService) List(ctx context.Context, principal auth.Principal) ([]RuleView, error) {
	if err := requireHuman(principal); err != nil {
		return nil, err
	}
	rules, err := s.rules.List(ctx, principal.TenantID)
	if err != nil {
		return nil, err
	}
	views := make([]RuleView, 0, len(rules))
	for _, rule := range rules {
		views = append(views, toRuleView(rule))
	}
	return views, nil
}

func (s *RuleService) Get(ctx context.Context, principal auth.Principal, id string) (RuleView, error) {
	if err := requireHuman(principal); err != nil {
		return RuleView{}, err
	}
	rule, err := s.rules.Get(ctx, principal.TenantID, id)
	if err != nil {
		return RuleView{}, err
	}
	return toRuleView(rule), nil
}

func (s *RuleService) Update(ctx context.Context, principal auth.Principal, cmd UpdateRuleCommand) (RuleView, error) {
	if err := requireAdminHuman(principal); err != nil {
		return RuleView{}, err
	}
	existing, err := s.rules.Get(ctx, principal.TenantID, cmd.ID)
	if err != nil {
		return RuleView{}, err
	}
	enabled := existing.Enabled
	if cmd.Enabled != nil {
		enabled = *cmd.Enabled
	}
	sourceAuth := existing.SourceAuth
	if cmd.SourceAuth != "" {
		sourceAuth = cmd.SourceAuth
	}
	rule, err := domain.NewRule(
		cmd.ID,
		principal.TenantID,
		cmd.Repo,
		cmd.TaskType,
		cmd.IncludeLabels,
		cmd.ExcludeLabels,
		cmd.IssueState,
		cmd.DefaultPriority,
		cmd.DedupeStrategy,
		sourceAuth,
		enabled,
		existing.LastSyncedAt,
		existing.CreatedAt,
		s.now(),
	)
	if err != nil {
		return RuleView{}, err
	}
	if err := s.rules.Update(ctx, rule); err != nil {
		return RuleView{}, err
	}
	return toRuleView(rule), nil
}

func (s *RuleService) Delete(ctx context.Context, principal auth.Principal, id string) error {
	if err := requireAdminHuman(principal); err != nil {
		return err
	}
	return s.rules.Delete(ctx, principal.TenantID, id)
}

func (s *RuleService) SetEnabled(ctx context.Context, principal auth.Principal, id string, enabled bool) (RuleView, error) {
	if err := requireAdminHuman(principal); err != nil {
		return RuleView{}, err
	}
	rule, err := s.rules.Get(ctx, principal.TenantID, id)
	if err != nil {
		return RuleView{}, err
	}
	rule.Enabled = enabled
	rule.UpdatedAt = s.now()
	if err := s.rules.Update(ctx, rule); err != nil {
		return RuleView{}, err
	}
	return toRuleView(rule), nil
}

func requireAdminHuman(principal auth.Principal) error {
	if err := requireHuman(principal); err != nil {
		return err
	}
	if !principal.IsAdmin {
		return domain.ErrForbidden
	}
	return nil
}

func requireHuman(principal auth.Principal) error {
	if principal.Type != auth.PrincipalTypeHuman {
		return domain.ErrForbidden
	}
	if principal.TenantID == "" {
		return invalidArg("tenant_id")
	}
	return nil
}

func toRuleView(rule *domain.Rule) RuleView {
	return RuleView{
		ID:              rule.ID,
		Repo:            rule.Repo,
		IncludeLabels:   append([]string(nil), rule.IncludeLabels...),
		ExcludeLabels:   append([]string(nil), rule.ExcludeLabels...),
		IssueState:      rule.IssueState,
		TaskType:        rule.TaskType,
		DefaultPriority: rule.DefaultPriority,
		DedupeStrategy:  rule.DedupeStrategy,
		SourceAuth:      rule.SourceAuth,
		Enabled:         rule.Enabled,
		LastSyncedAt:    rule.LastSyncedAt,
		CreatedAt:       rule.CreatedAt,
		UpdatedAt:       rule.UpdatedAt,
	}
}

func invalidArg(field string) error {
	return &domain.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

func randomID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

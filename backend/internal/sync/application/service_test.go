package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/sync/domain"
)

func TestRuleServiceCreateRequiresAdminHuman(t *testing.T) {
	ctx := context.Background()
	svc := newTestRuleService(t)
	cmd := validCreateRuleCommand()

	_, err := svc.Create(ctx, auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeHuman, OwnerID: "owner-1"}, cmd)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Create() non-admin human error = %v, want ErrForbidden", err)
	}

	_, err = svc.Create(ctx, auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeAgent, AgentID: "agent-1", AgentVersionID: "v1", IsAdmin: true}, cmd)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Create() agent error = %v, want ErrForbidden", err)
	}

	view, err := svc.Create(ctx, adminPrincipal("tenant-1"), cmd)
	if err != nil {
		t.Fatalf("Create() admin error = %v", err)
	}
	if view.ID != "rule-fixed" || view.Repo != "octo/hello-world" || !view.Enabled {
		t.Fatalf("Create() view = %+v, want generated enabled rule", view)
	}
}

func TestRuleServiceReadRequiresHumanAndUsesPrincipalTenant(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRuleRepository()
	now := fixedNow()
	repo.mustCreate(t, newDomainRule(t, "tenant-1", "rule-1", true, now))
	repo.mustCreate(t, newDomainRule(t, "tenant-2", "rule-2", true, now))
	svc, err := NewRuleService(repo, RuleServiceOptions{NewID: fixedID, Now: fixedNow})
	if err != nil {
		t.Fatalf("NewRuleService() error = %v", err)
	}

	_, err = svc.List(ctx, auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeAgent, AgentID: "agent-1"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("List() agent error = %v, want ErrForbidden", err)
	}

	views, err := svc.List(ctx, auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeHuman, OwnerID: "owner-1"})
	if err != nil {
		t.Fatalf("List() human error = %v", err)
	}
	if got, want := viewIDs(views), []string{"rule-1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("List() ids = %v, want %v", got, want)
	}

	_, err = svc.Get(ctx, auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeHuman, OwnerID: "owner-1"}, "rule-2")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get() cross-tenant error = %v, want ErrNotFound", err)
	}
}

func TestRuleServiceUpdateDeleteAndSetEnabledRequireAdmin(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRuleRepository()
	now := fixedNow()
	repo.mustCreate(t, newDomainRule(t, "tenant-1", "rule-1", true, now))
	svc, err := NewRuleService(repo, RuleServiceOptions{NewID: fixedID, Now: fixedNow})
	if err != nil {
		t.Fatalf("NewRuleService() error = %v", err)
	}
	human := auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeHuman, OwnerID: "owner-1"}
	admin := adminPrincipal("tenant-1")

	update := UpdateRuleCommand{
		ID:              "rule-1",
		Repo:            "octo/hello-world",
		IncludeLabels:   []string{"triage"},
		ExcludeLabels:   []string{"duplicate"},
		IssueState:      "all",
		TaskType:        "investigation",
		DefaultPriority: "P1",
		DedupeStrategy:  domain.DedupeSkip,
	}
	if _, err := svc.Update(ctx, human, update); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Update() non-admin error = %v, want ErrForbidden", err)
	}
	updated, err := svc.Update(ctx, admin, update)
	if err != nil {
		t.Fatalf("Update() admin error = %v", err)
	}
	if updated.TaskType != "investigation" || updated.DedupeStrategy != domain.DedupeSkip || !updated.Enabled {
		t.Fatalf("Update() view = %+v, want updated fields preserving enabled", updated)
	}

	enabled := false
	disabledByUpdate, err := svc.Update(ctx, admin, UpdateRuleCommand{
		ID:              "rule-1",
		Repo:            "octo/hello-world",
		IncludeLabels:   []string{"triage"},
		ExcludeLabels:   []string{"duplicate"},
		IssueState:      "all",
		TaskType:        "investigation",
		DefaultPriority: "P1",
		DedupeStrategy:  domain.DedupeSkip,
		Enabled:         &enabled,
	})
	if err != nil {
		t.Fatalf("Update() enabled error = %v", err)
	}
	if disabledByUpdate.Enabled {
		t.Fatalf("Update() enabled = true, want false")
	}

	if _, err := svc.SetEnabled(ctx, human, "rule-1", false); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("SetEnabled() non-admin error = %v, want ErrForbidden", err)
	}
	disabled, err := svc.SetEnabled(ctx, admin, "rule-1", false)
	if err != nil {
		t.Fatalf("SetEnabled() admin error = %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("SetEnabled() enabled = true, want false")
	}

	if err := svc.Delete(ctx, human, "rule-1"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("Delete() non-admin error = %v, want ErrForbidden", err)
	}
	if err := svc.Delete(ctx, admin, "rule-1"); err != nil {
		t.Fatalf("Delete() admin error = %v", err)
	}
	_, err = svc.Get(ctx, human, "rule-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}

func newTestRuleService(t *testing.T) *RuleService {
	t.Helper()
	svc, err := NewRuleService(newFakeRuleRepository(), RuleServiceOptions{NewID: fixedID, Now: fixedNow})
	if err != nil {
		t.Fatalf("NewRuleService() error = %v", err)
	}
	return svc
}

func validCreateRuleCommand() CreateRuleCommand {
	return CreateRuleCommand{
		Repo:            "octo/hello-world",
		IncludeLabels:   []string{"bug", "sync"},
		ExcludeLabels:   []string{"wontfix"},
		IssueState:      "open",
		TaskType:        "bugfix",
		DefaultPriority: "P2",
		DedupeStrategy:  domain.DedupeUpdate,
	}
}

func adminPrincipal(tenantID string) auth.Principal {
	return auth.Principal{TenantID: tenantID, Type: auth.PrincipalTypeHuman, OwnerID: "admin-1", IsAdmin: true}
}

func fixedID() string {
	return "rule-fixed"
}

func fixedNow() time.Time {
	return time.Date(2026, 7, 7, 15, 0, 0, 0, time.UTC)
}

func newDomainRule(t *testing.T, tenantID, id string, enabled bool, now time.Time) *domain.Rule {
	t.Helper()
	rule, err := domain.NewRule(
		id,
		tenantID,
		"octo/hello-world",
		"bugfix",
		[]string{"bug"},
		nil,
		"open",
		"P2",
		domain.DedupeUpdate,
		"",
		enabled,
		time.Time{},
		now,
		now,
	)
	if err != nil {
		t.Fatalf("NewRule() error = %v", err)
	}
	return rule
}

func viewIDs(views []RuleView) []string {
	ids := make([]string, 0, len(views))
	for _, view := range views {
		ids = append(ids, view.ID)
	}
	return ids
}

type fakeRuleRepository struct {
	rules map[string]*domain.Rule
}

func newFakeRuleRepository() *fakeRuleRepository {
	return &fakeRuleRepository{rules: make(map[string]*domain.Rule)}
}

func (r *fakeRuleRepository) Create(_ context.Context, rule *domain.Rule) error {
	key := rule.TenantID + "/" + rule.ID
	r.rules[key] = cloneRule(rule)
	return nil
}

func (r *fakeRuleRepository) Get(_ context.Context, tenantID, id string) (*domain.Rule, error) {
	rule, ok := r.rules[tenantID+"/"+id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return cloneRule(rule), nil
}

func (r *fakeRuleRepository) List(_ context.Context, tenantID string) ([]*domain.Rule, error) {
	var rules []*domain.Rule
	for _, rule := range r.rules {
		if rule.TenantID == tenantID {
			rules = append(rules, cloneRule(rule))
		}
	}
	return rules, nil
}

func (r *fakeRuleRepository) ListEnabledAllTenants(_ context.Context) ([]*domain.Rule, error) {
	var rules []*domain.Rule
	for _, rule := range r.rules {
		if rule.Enabled {
			rules = append(rules, cloneRule(rule))
		}
	}
	return rules, nil
}

func (r *fakeRuleRepository) Update(_ context.Context, rule *domain.Rule) error {
	key := rule.TenantID + "/" + rule.ID
	if _, ok := r.rules[key]; !ok {
		return domain.ErrNotFound
	}
	r.rules[key] = cloneRule(rule)
	return nil
}

func (r *fakeRuleRepository) Delete(_ context.Context, tenantID, id string) error {
	key := tenantID + "/" + id
	if _, ok := r.rules[key]; !ok {
		return domain.ErrNotFound
	}
	delete(r.rules, key)
	return nil
}

func (r *fakeRuleRepository) TouchSynced(_ context.Context, tenantID, id string, syncedAt time.Time) error {
	key := tenantID + "/" + id
	rule, ok := r.rules[key]
	if !ok {
		return domain.ErrNotFound
	}
	rule.LastSyncedAt = syncedAt
	return nil
}

func (r *fakeRuleRepository) mustCreate(t *testing.T, rule *domain.Rule) {
	t.Helper()
	if err := r.Create(context.Background(), rule); err != nil {
		t.Fatalf("fake Create() error = %v", err)
	}
}

func cloneRule(rule *domain.Rule) *domain.Rule {
	if rule == nil {
		return nil
	}
	clone := *rule
	clone.IncludeLabels = append([]string(nil), rule.IncludeLabels...)
	clone.ExcludeLabels = append([]string(nil), rule.ExcludeLabels...)
	return &clone
}

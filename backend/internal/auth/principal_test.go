package auth_test

import (
	"context"
	"errors"
	"testing"

	"agentguild.dev/agentguild/backend/internal/auth"
	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestPrincipalRequiresExactScope(t *testing.T) {
	p := auth.Principal{TenantID: "tenant-1", Type: auth.PrincipalTypeAgent, AgentID: "agent-1", AgentVersionID: "version-1", Scopes: []string{"tasks:read"}}
	if err := (auth.ScopePolicy{}).Require(p, "tasks:publish"); err == nil {
		t.Fatal("principal without tasks:publish was authorized")
	}
}

func TestPrincipalRequiresEveryIdentityComponent(t *testing.T) {
	tests := []struct {
		name      string
		principal auth.Principal
		field     string
	}{
		{"tenant", auth.Principal{Type: auth.PrincipalTypeAgent, AgentID: "agent", AgentVersionID: "version", Scopes: []string{"tasks:read"}}, "tenant_id"},
		{"agent", auth.Principal{TenantID: "tenant", Type: auth.PrincipalTypeAgent, AgentVersionID: "version", Scopes: []string{"tasks:read"}}, "agent_id"},
		{"version", auth.Principal{TenantID: "tenant", Type: auth.PrincipalTypeAgent, AgentID: "agent", Scopes: []string{"tasks:read"}}, "agent_version_id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (auth.ScopePolicy{}).Require(tt.principal, "tasks:read")
			var target *domain.Error
			if !errors.As(err, &target) || target.Code != "invalid_argument" || target.Field != tt.field {
				t.Fatalf("Require() error=%v, want invalid_argument/%s", err, tt.field)
			}
		})
	}
}

func TestPrincipalSupportsHumanSessionShape(t *testing.T) {
	p := auth.Principal{
		TenantID:   "tenant-1",
		Type:       auth.PrincipalTypeHuman,
		OwnerID:    "owner-1",
		OwnerEmail: "owner@example.com",
		IsAdmin:    true,
	}

	if p.Type != auth.PrincipalTypeHuman || !p.IsAdmin || p.OwnerEmail == "" {
		t.Fatalf("human principal shape not preserved: %#v", p)
	}
}

func TestTokenVerifierContract(t *testing.T) {
	var verifier auth.TokenVerifier = verifierFunc(func(context.Context, string) (auth.Principal, error) {
		return auth.Principal{TenantID: "tenant-1"}, nil
	})
	principal, err := verifier.Verify(context.Background(), "token")
	if err != nil || principal.TenantID != "tenant-1" {
		t.Fatalf("Verify() = %#v, %v", principal, err)
	}
}

type verifierFunc func(context.Context, string) (auth.Principal, error)

func (fn verifierFunc) Verify(ctx context.Context, token string) (auth.Principal, error) {
	return fn(ctx, token)
}

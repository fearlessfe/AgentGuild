package auth_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/auth"
)

func TestPrincipalRequiresExactScope(t *testing.T) {
	p := auth.Principal{Scopes: []string{"tasks:read"}}
	if err := (auth.ScopePolicy{}).Require(p, "tasks:publish"); err == nil {
		t.Fatal("principal without tasks:publish was authorized")
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

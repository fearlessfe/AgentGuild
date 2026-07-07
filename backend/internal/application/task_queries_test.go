package application_test

import (
	"context"
	"testing"

	"agentguild.dev/agentguild/backend/internal/application"
	"agentguild.dev/agentguild/backend/internal/auth"
)

func TestListTasksAllowsHumanPrincipal(t *testing.T) {
	svc, _ := newServiceFixture()
	human := auth.Principal{
		TenantID: "tenant-1",
		Type:     auth.PrincipalTypeHuman,
	}

	if _, err := svc.ListTasks(context.Background(), human, application.ListTasks{Limit: 20}); err != nil {
		t.Fatalf("human ListTasks should pass, got %v", err)
	}
}

package domain_test

import (
	"fmt"
	"testing"

	"agentguild.dev/agentguild/backend/internal/domain"
)

func TestErrorCodeAndFieldSurviveWrapping(t *testing.T) {
	err := fmt.Errorf("application boundary: %w", &domain.Error{Code: "invalid_argument", Message: "bad cursor", Field: "cursor"})
	if got := domain.CodeOf(err); got != "invalid_argument" {
		t.Fatalf("CodeOf() = %q", got)
	}
	if got := domain.FieldOf(err); got != "cursor" {
		t.Fatalf("FieldOf() = %q", got)
	}
}

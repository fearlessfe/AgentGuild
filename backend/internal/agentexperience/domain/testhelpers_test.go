package domain

import (
	"testing"
	"time"
)

// MockSensitivityPolicy is a test double for SensitivityPolicy.
type MockSensitivityPolicy struct {
	Class  SensitivityClass
	Reason string
}

func (m MockSensitivityPolicy) Classify(evidence []byte) (SensitivityClass, string) {
	return m.Class, m.Reason
}

// NewTestCandidate returns a candidate in pending_review with public sensitivity.
func NewTestCandidate(t testing.TB) *ExperienceCandidate {
	t.Helper()
	now := time.Now()
	c, err := NewExperienceCandidate(
		"candidate-id", "tenant-id", "agent-id",
		"task-id", "submission-id", "review-id",
		[]byte("evidence-content"),
		[]string{"code", "review"},
		"tenant-id", now,
	)
	if err != nil {
		t.Fatalf("new test candidate: %v", err)
	}
	return c
}

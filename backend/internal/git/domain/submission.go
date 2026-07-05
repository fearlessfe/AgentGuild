package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	core "agentguild.dev/agentguild/backend/internal/domain"
)

// SubmissionStatus is the lifecycle state of a code submission.
type SubmissionStatus string

const (
	// SubmissionStatusPendingVerification means the submission has been received
	// and is waiting for automatic validation results.
	SubmissionStatusPendingVerification SubmissionStatus = "pending_verification"
	// SubmissionStatusValidated means the submission passed automatic validation.
	SubmissionStatusValidated SubmissionStatus = "validated"
	// SubmissionStatusValidationFailed means automatic validation reported failures.
	SubmissionStatusValidationFailed SubmissionStatus = "validation_failed"
	// SubmissionStatusInvalid means the submission violates structural rules and
	// must not proceed to review.
	SubmissionStatusInvalid SubmissionStatus = "invalid"
)

// Submission is the aggregate root for a code submission delivered by an Agent.
type Submission struct {
	ID              string
	TenantID        string
	TaskID          string
	ExecutionID     string
	Repo            string
	Branch          string
	CommitSHA       string
	BaseCommitSHA   string
	Summary         string
	TestDeclaration *string
	Evidence        []byte
	DiffFingerprint string
	Status          SubmissionStatus
	ValidationJobID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NewSubmission creates a new Submission in pending_verification status and
// computes its deterministic diff fingerprint.
func NewSubmission(
	tenantID string,
	taskID string,
	executionID string,
	repo string,
	branch string,
	commitSHA string,
	baseCommitSHA string,
	summary string,
	testDeclaration *string,
	evidence []byte,
	changedPaths []string,
	now time.Time,
	newID func() string,
) (*Submission, error) {
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if taskID == "" {
		return nil, invalidArgument("task_id")
	}
	if executionID == "" {
		return nil, invalidArgument("execution_id")
	}
	if repo == "" {
		return nil, invalidArgument("repo")
	}
	if branch == "" {
		return nil, invalidArgument("branch")
	}
	if commitSHA == "" {
		return nil, invalidArgument("commit_sha")
	}
	if baseCommitSHA == "" {
		return nil, invalidArgument("base_commit_sha")
	}
	if summary == "" {
		return nil, invalidArgument("summary")
	}
	if now.IsZero() {
		return nil, invalidArgument("now")
	}
	if newID == nil {
		return nil, invalidArgument("new_id")
	}

	id := newID()
	if id == "" {
		return nil, invalidArgument("id")
	}

	return &Submission{
		ID:              id,
		TenantID:        tenantID,
		TaskID:          taskID,
		ExecutionID:     executionID,
		Repo:            repo,
		Branch:          branch,
		CommitSHA:       commitSHA,
		BaseCommitSHA:   baseCommitSHA,
		Summary:         summary,
		TestDeclaration: testDeclaration,
		Evidence:        evidence,
		DiffFingerprint: DiffFingerprint(branch, baseCommitSHA, commitSHA, changedPaths),
		Status:          SubmissionStatusPendingVerification,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// DiffFingerprint returns the deterministic SHA-256 fingerprint for a diff.
// The canonical form is:
//
//   branch + "\n" + base_commit + "\n" + head_commit + "\n" + sorted_changed_paths...
func DiffFingerprint(branch, baseCommit, headCommit string, changedPaths []string) string {
	sorted := append([]string(nil), changedPaths...)
	sort.Strings(sorted)

	var b strings.Builder
	b.WriteString(branch)
	b.WriteByte('\n')
	b.WriteString(baseCommit)
	b.WriteByte('\n')
	b.WriteString(headCommit)
	for _, p := range sorted {
		b.WriteByte('\n')
		b.WriteString(p)
	}

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// MarkValidated transitions the submission to validated.
func (s *Submission) MarkValidated(now time.Time) error {
	if s.Status != SubmissionStatusPendingVerification {
		return core.ErrStateConflict
	}
	s.Status = SubmissionStatusValidated
	s.UpdatedAt = now
	return nil
}

// MarkValidationFailed transitions the submission to validation_failed.
func (s *Submission) MarkValidationFailed(now time.Time) error {
	if s.Status != SubmissionStatusPendingVerification {
		return core.ErrStateConflict
	}
	s.Status = SubmissionStatusValidationFailed
	s.UpdatedAt = now
	return nil
}

// MarkInvalid transitions the submission to invalid.
func (s *Submission) MarkInvalid(now time.Time) error {
	if s.Status != SubmissionStatusPendingVerification {
		return core.ErrStateConflict
	}
	s.Status = SubmissionStatusInvalid
	s.UpdatedAt = now
	return nil
}

// SetValidationJobID records the external validation job identifier.
func (s *Submission) SetValidationJobID(jobID string, now time.Time) {
	s.ValidationJobID = &jobID
	s.UpdatedAt = now
}

func invalidArgument(field string) error {
	return &core.Error{Code: "invalid_argument", Message: field + " is invalid", Field: field}
}

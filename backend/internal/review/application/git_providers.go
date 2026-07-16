package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	gitapp "agentguild.dev/agentguild/backend/internal/git/application"
	gitdomain "agentguild.dev/agentguild/backend/internal/git/domain"
)

var unifiedHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// GitDiffProvider builds review diffs from the immutable commits recorded on a
// tenant-scoped submission.
type GitDiffProvider struct {
	submissions gitapp.SubmissionRepository
	resolver    gitapp.RepositoryGitResolver
}

func NewGitDiffProvider(submissions gitapp.SubmissionRepository, resolver gitapp.RepositoryGitResolver) (*GitDiffProvider, error) {
	if submissions == nil {
		return nil, invalid("submission_repository")
	}
	if resolver == nil {
		return nil, invalid("repository_git_resolver")
	}
	return &GitDiffProvider{submissions: submissions, resolver: resolver}, nil
}

func (p *GitDiffProvider) GetDiff(ctx context.Context, tenantID, submissionID string) ([]FileDiff, error) {
	submission, err := p.submissions.GetByID(ctx, tenantID, submissionID)
	if err != nil {
		return nil, err
	}
	resolved, err := p.resolver.Driver(ctx, tenantID, submission.Repo)
	if err != nil {
		return nil, err
	}
	files, err := resolved.Driver.CompareCommits(ctx, resolved.FullName, submission.BaseCommitSHA, submission.CommitSHA)
	if err != nil {
		return nil, err
	}
	out := make([]FileDiff, 0, len(files))
	for _, file := range files {
		out = append(out, FileDiff{Path: file.Filename, Hunks: parseUnifiedPatch(file.Patch)})
	}
	return out, nil
}

func parseUnifiedPatch(patch string) []Hunk {
	lines := strings.Split(patch, "\n")
	var hunks []Hunk
	for i := 0; i < len(lines); {
		matches := unifiedHunkHeader.FindStringSubmatch(lines[i])
		if matches == nil {
			i++
			continue
		}
		h := Hunk{
			OldStart: parsePatchNumber(matches[1]),
			OldLines: parsePatchCount(matches[2]),
			NewStart: parsePatchNumber(matches[3]),
			NewLines: parsePatchCount(matches[4]),
		}
		oldLine, newLine := h.OldStart, h.NewStart
		var raw strings.Builder
		raw.WriteString(lines[i])
		i++
		for i < len(lines) && unifiedHunkHeader.FindStringSubmatch(lines[i]) == nil {
			line := lines[i]
			if strings.HasPrefix(line, `\ No newline at end of file`) {
				i++
				continue
			}
			raw.WriteByte('\n')
			raw.WriteString(line)
			switch {
			case strings.HasPrefix(line, "+"):
				h.Lines = append(h.Lines, DiffLine{Type: "add", Text: line, NewLine: newLine})
				newLine++
			case strings.HasPrefix(line, "-"):
				h.Lines = append(h.Lines, DiffLine{Type: "remove", Text: line, OldLine: oldLine})
				oldLine++
			default:
				h.Lines = append(h.Lines, DiffLine{Type: "context", Text: line, OldLine: oldLine, NewLine: newLine})
				oldLine++
				newLine++
			}
			i++
		}
		sum := sha256.Sum256([]byte(raw.String()))
		h.HunkHash = hex.EncodeToString(sum[:])
		hunks = append(hunks, h)
	}
	return hunks
}

func parsePatchNumber(value string) int {
	n, _ := strconv.Atoi(value)
	return n
}

func parsePatchCount(value string) int {
	if value == "" {
		return 1
	}
	return parsePatchNumber(value)
}

// GitValidationProvider reads persisted validation jobs and requires every
// configured hard gate to have succeeded.
type GitValidationProvider struct {
	jobs gitapp.ValidationJobRepository
}

func NewGitValidationProvider(jobs gitapp.ValidationJobRepository) (*GitValidationProvider, error) {
	if jobs == nil {
		return nil, invalid("validation_job_repository")
	}
	return &GitValidationProvider{jobs: jobs}, nil
}

func (p *GitValidationProvider) GetValidationStatus(ctx context.Context, tenantID, submissionID string) (ValidationStatus, error) {
	job, err := p.jobs.GetBySubmissionID(ctx, tenantID, submissionID)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf("validation job for submission %q was not found", submissionID)
	}
	return persistedValidationStatus{job: job}, nil
}

type persistedValidationStatus struct {
	job *gitdomain.ValidationJob
}

func (s persistedValidationStatus) AllHardGatesPassed() bool {
	if s.job == nil || s.job.Status != gitdomain.ValidationStatusSucceeded {
		return false
	}
	for _, step := range s.job.Steps {
		if step.HardGate && step.Status != gitdomain.ValidationStepStatusSucceeded {
			return false
		}
	}
	return true
}

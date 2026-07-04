package domain

import (
	"time"
)

// DiffSide indicates which side of a diff a line comment is anchored to.
type DiffSide string

const (
	SideLeft  DiffSide = "left"
	SideRight DiffSide = "right"
)

// LineComment is a single comment anchored to a line in a diff.
type LineComment struct {
	ID              string
	TenantID        string
	ReviewID        string
	SubmissionID    string
	FilePath        string
	Side            DiffSide
	LineNumber      int
	HunkHash        string
	DiffFingerprint string
	Text            string
	CreatedAt       time.Time
}

// NewLineComment creates a validated line-level comment.
func NewLineComment(
	id, tenantID, reviewID, submissionID, filePath string,
	side DiffSide,
	lineNumber int,
	hunkHash, diffFingerprint, text string,
	now time.Time,
) (*LineComment, error) {
	if id == "" {
		return nil, invalidArgument("id")
	}
	if tenantID == "" {
		return nil, invalidArgument("tenant_id")
	}
	if reviewID == "" {
		return nil, invalidArgument("review_id")
	}
	if submissionID == "" {
		return nil, invalidArgument("submission_id")
	}
	if filePath == "" {
		return nil, invalidArgument("file_path")
	}
	if side != SideLeft && side != SideRight {
		return nil, invalidArgument("side")
	}
	if lineNumber <= 0 {
		return nil, invalidArgument("line_number")
	}
	if text == "" {
		return nil, invalidArgument("text")
	}
	if now.IsZero() {
		return nil, invalidArgument("created_at")
	}
	return &LineComment{
		ID:              id,
		TenantID:        tenantID,
		ReviewID:        reviewID,
		SubmissionID:    submissionID,
		FilePath:        filePath,
		Side:            side,
		LineNumber:      lineNumber,
		HunkHash:        hunkHash,
		DiffFingerprint: diffFingerprint,
		Text:            text,
		CreatedAt:       now,
	}, nil
}

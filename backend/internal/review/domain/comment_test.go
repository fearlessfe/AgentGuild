package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
)

func TestLineCommentConstructorRejectsInvalidFields(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name            string
		id              string
		tenantID        string
		reviewID        string
		submissionID    string
		filePath        string
		side            reviewdomain.DiffSide
		lineNumber      int
		hunkHash        string
		diffFingerprint string
		text            string
		field           string
	}{
		{name: "empty id", tenantID: "t", reviewID: "r", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "id"},
		{name: "empty tenant_id", id: "id", reviewID: "r", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "tenant_id"},
		{name: "empty review_id", id: "id", tenantID: "t", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "review_id"},
		{name: "empty submission_id", id: "id", tenantID: "t", reviewID: "r", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "submission_id"},
		{name: "empty file_path", id: "id", tenantID: "t", reviewID: "r", submissionID: "s", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "file_path"},
		{name: "invalid side", id: "id", tenantID: "t", reviewID: "r", submissionID: "s", filePath: "f.go", side: "middle", lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "side"},
		{name: "zero line_number", id: "id", tenantID: "t", reviewID: "r", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "line_number"},
		{name: "empty hunk_hash", id: "id", tenantID: "t", reviewID: "r", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, diffFingerprint: "fp", text: "ok", field: "hunk_hash"},
		{name: "empty diff_fingerprint", id: "id", tenantID: "t", reviewID: "r", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", text: "ok", field: "diff_fingerprint"},
		{name: "empty text", id: "id", tenantID: "t", reviewID: "r", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", field: "text"},
		{name: "zero created_at", id: "id", tenantID: "t", reviewID: "r", submissionID: "s", filePath: "f.go", side: reviewdomain.SideRight, lineNumber: 1, hunkHash: "hunk", diffFingerprint: "fp", text: "ok", field: "created_at"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var nowArg time.Time
			if tc.field != "created_at" {
				nowArg = now
			}
			comment, err := reviewdomain.NewLineComment(tc.id, tc.tenantID, tc.reviewID, tc.submissionID, tc.filePath, tc.side, tc.lineNumber, tc.hunkHash, tc.diffFingerprint, tc.text, nowArg)
			require.Nil(t, comment)
			assertInvalidArgument(t, err, tc.field)
		})
	}
}

func TestLineCommentStoresHashes(t *testing.T) {
	now := time.Now()
	comment, err := reviewdomain.NewLineComment("c-1", "tenant-1", "rev-1", "sub-1", "main.go", reviewdomain.SideRight, 42, "hunk-abc", "fp-123", "looks good", now)
	require.NoError(t, err)
	require.Equal(t, "hunk-abc", comment.HunkHash)
	require.Equal(t, "fp-123", comment.DiffFingerprint)
}

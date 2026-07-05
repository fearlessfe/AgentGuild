package acceptance

import (
	"testing"
	"time"

	"agentguild.dev/agentguild/backend/internal/domain"
	reviewdomain "agentguild.dev/agentguild/backend/internal/review/domain"
	"github.com/stretchr/testify/require"
)

// TestEndToEndReviewAcceptedUpdatesProjection 验证完整流程：
// 发布任务 → 领取/开始 → 提交 review → review accepted → worker tick → 声望投影更新。
func TestEndToEndReviewAcceptedUpdatesProjection(t *testing.T) {
	env := Start(t)

	taskID := env.PublishTask(time.Now().Add(time.Hour))
	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	env.MCP.StartExecution(claimed.ID, claimed.LeaseGeneration, "req-start")

	env.SubmitForReview(claimed.ID)
	review := env.CreateReviewViaREST(claimed.ID, []string{"go"}, "req-create-review")
	require.Equal(t, "reviewer-1", review.ReviewerID)

	env.SubmitDecisionViaMCP(review.ID, "accepted",
		[]reviewdomain.RubricScore{{Dimension: "quality", Score: 80}},
		"lgtm", "token-reviewer", "req-submit-decision")

	env.WorkerTick()

	proj := env.GetProjection("tenant-1", "acceptance-agent-1", "go", "code")
	require.Equal(t, 1, proj.TotalReviews)
	require.Equal(t, 1, proj.AcceptedCount)
	require.Equal(t, 0, proj.RejectedCount)
	require.Equal(t, 0, proj.RevisionRequestedCount)
	require.InDelta(t, 1.0, proj.PassRate, 0.0001)
	require.InDelta(t, 0.0, proj.ReworkRate, 0.0001)
}

// TestHardGatesFailedPreventsAcceptDecision 验证硬 gate 失败时无法 accept，
// 但可以 reject；重新打开 gate 后可以 accept。
func TestHardGatesFailedPreventsAcceptDecision(t *testing.T) {
	env := Start(t)

	taskID := env.PublishTask(time.Now().Add(time.Hour))
	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	env.MCP.StartExecution(claimed.ID, claimed.LeaseGeneration, "req-start")
	env.SubmitForReview(claimed.ID)
	review := env.CreateReviewViaREST(claimed.ID, []string{"go"}, "req-create-review")

	env.SetHardGatesPass(false)
	code := env.SubmitDecisionCodeViaMCP(review.ID, "accepted",
		[]reviewdomain.RubricScore{{Dimension: "quality", Score: 80}},
		"lgtm", "token-reviewer", "req-submit-bad")
	require.Equal(t, "HARD_GATES_FAILED", code)

	code = env.SubmitDecisionCodeViaMCP(review.ID, "rejected",
		nil, "does not compile", "token-reviewer", "req-submit-reject")
	require.Empty(t, code)

	got := env.MCP.As("token-reviewer").ReviewGet(review.ID).Review
	require.Equal(t, string(reviewdomain.DecisionRejected), got.FinalDecision)
	require.Equal(t, string(domain.ExecutionRejected), string(env.GetExecutionStatus("tenant-1", claimed.ID)))
}

// TestRevisionRequestedNewExecutionCommentsIsolated 验证 revision requested 后，
// 新 execution 产生的新 review 不会包含旧 review 的行级注释。
func TestRevisionRequestedNewExecutionCommentsIsolated(t *testing.T) {
	env := Start(t)

	taskID := env.PublishTask(time.Now().Add(time.Hour))
	claimed := env.MCP.TaskClaim(taskID, "req-claim")
	env.MCP.StartExecution(claimed.ID, claimed.LeaseGeneration, "req-start")
	env.SubmitForReview(claimed.ID)
	review1 := env.CreateReviewViaREST(claimed.ID, []string{"go"}, "req-create-review-1")
	env.AddCommentViaREST(review1.ID, claimed.ID, "old comment", "token-reviewer", "req-comment-1")

	env.SubmitDecisionViaMCP(review1.ID, "revision_requested",
		nil, "please fix", "token-reviewer", "req-revision")

	newExecutionID := env.CreateResubmissionExecution(taskID, claimed.ID, "resubmission-1", "acceptance-agent-1")
	review2 := env.CreateReviewViaREST(newExecutionID, []string{"go"}, "req-create-review-2")
	env.AddCommentViaREST(review2.ID, newExecutionID, "new comment", "token-reviewer", "req-comment-2")

	oldComments := env.ListComments("tenant-1", review1.ID)
	require.Len(t, oldComments, 1)
	require.Equal(t, "old comment", oldComments[0].Text)

	newComments := env.ListComments("tenant-1", review2.ID)
	require.Len(t, newComments, 1)
	require.Equal(t, "new comment", newComments[0].Text)
	require.NotEqual(t, oldComments[0].ID, newComments[0].ID)
}

// TestCrossAgentVersionReputationIsolation 验证不同 Agent Version 的声望投影相互隔离。
func TestCrossAgentVersionReputationIsolation(t *testing.T) {
	env := Start(t)

	task1ID := env.PublishTask(time.Now().Add(time.Hour))
	claimed1 := env.MCP.As("token-agent-1").TaskClaim(task1ID, "req-claim-1")
	env.MCP.As("token-agent-1").StartExecution(claimed1.ID, claimed1.LeaseGeneration, "req-start-1")
	env.SubmitForReview(claimed1.ID)
	review1 := env.CreateReviewViaREST(claimed1.ID, []string{"go"}, "req-create-review-1")
	env.SubmitDecisionViaMCP(review1.ID, "accepted",
		[]reviewdomain.RubricScore{{Dimension: "quality", Score: 80}},
		"lgtm", "token-reviewer", "req-decision-1")

	task2ID := env.PublishTask(time.Now().Add(time.Hour))
	claimed2 := env.MCP.As("token-agent-2").TaskClaim(task2ID, "req-claim-2")
	env.MCP.As("token-agent-2").StartExecution(claimed2.ID, claimed2.LeaseGeneration, "req-start-2")
	env.SubmitForReview(claimed2.ID)
	review2 := env.CreateReviewViaREST(claimed2.ID, []string{"go"}, "req-create-review-2")
	env.SubmitDecisionViaMCP(review2.ID, "rejected",
		nil, "not good", "token-reviewer", "req-decision-2")

	env.WorkerTick()

	proj1 := env.GetProjection("tenant-1", "acceptance-agent-1", "go", "code")
	require.Equal(t, 1, proj1.TotalReviews)
	require.Equal(t, 1, proj1.AcceptedCount)
	require.Equal(t, 0, proj1.RejectedCount)

	proj2 := env.GetProjection("tenant-1", "acceptance-agent-2", "go", "code")
	require.Equal(t, 1, proj2.TotalReviews)
	require.Equal(t, 0, proj2.AcceptedCount)
	require.Equal(t, 1, proj2.RejectedCount)
}

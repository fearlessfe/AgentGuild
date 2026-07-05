import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { useEffect, useMemo, useRef, useState } from "react";
import { addComment, getActiveRubric, getReview, getSubmissionDiff, submitDecision } from "./reviews.api";
import { DiffViewer } from "./DiffViewer";
import { FileTree } from "./FileTree";
import { RevisionSelector } from "./RevisionSelector";
import { RubricForm } from "./RubricForm";
import type { Decision, FileDiff, LineComment, ReviewStatus, RubricScore } from "./reviews.types";

function formatStatus(status: ReviewStatus) {
  switch (status) {
    case "pending":
      return "待审核";
    case "submitted":
      return "已提交";
    default:
      return status;
  }
}

function formatDecision(decision?: Decision) {
  switch (decision) {
    case "accepted":
      return "通过";
    case "rejected":
      return "拒绝";
    case "revision_requested":
      return "需要修改";
    default:
      return undefined;
  }
}

export function ReviewPage() {
  const { reviewId } = useParams<{ reviewId: string }>();
  const queryClient = useQueryClient();
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [comments, setComments] = useState<LineComment[]>([]);
  const [scores, setScores] = useState<Record<string, number>>({});
  const [summary, setSummary] = useState("");

  const reviewQuery = useQuery({
    queryKey: ["review", reviewId],
    queryFn: () => getReview(reviewId ?? ""),
    enabled: !!reviewId,
    refetchOnWindowFocus: false,
  });

  const submissionId = reviewQuery.data?.data.submission_id;

  const diffQuery = useQuery({
    queryKey: ["submission-diff", submissionId],
    queryFn: () => getSubmissionDiff(submissionId ?? ""),
    enabled: !!submissionId,
  });

  const rubricQuery = useQuery({
    queryKey: ["active-rubric"],
    queryFn: () => getActiveRubric(),
    enabled: !!reviewId,
    refetchOnWindowFocus: false,
  });

  const files = useMemo<FileDiff[]>(() => diffQuery.data?.data ?? [], [diffQuery.data]);

  useEffect(() => {
    if (files.length > 0 && !selectedPath) {
      setSelectedPath(files[0].path);
    }
  }, [files, selectedPath]);

  const selectedFile = useMemo(() => files.find((f) => f.path === selectedPath), [files, selectedPath]);

  // Seed local comments from the review response once on first load only,
  // so background refetches do not discard locally added comments.
  const hasSeededComments = useRef(false);
  useEffect(() => {
    if (reviewQuery.data && !hasSeededComments.current) {
      hasSeededComments.current = true;
      setComments(reviewQuery.data.data.line_comments ?? []);
    }
  }, [reviewQuery.data]);

  // Seed local scores and summary from the review response once on first load.
  const hasSeededScores = useRef(false);
  useEffect(() => {
    if (reviewQuery.data && !hasSeededScores.current) {
      hasSeededScores.current = true;
      const next: Record<string, number> = {};
      for (const score of reviewQuery.data.data.rubric_scores) {
        next[score.dimension] = score.score;
      }
      setScores(next);
      setSummary(reviewQuery.data.data.summary ?? "");
    }
  }, [reviewQuery.data]);

  // Reset seed flags when navigating to a different review.
  useEffect(() => {
    hasSeededComments.current = false;
    hasSeededScores.current = false;
  }, [reviewId]);

  const commentMutation = useMutation({
    mutationFn: async (input: {
      lineNumber: number;
      side: "left" | "right";
      hunkHash: string;
      text: string;
    }) => {
      const result = await addComment(reviewId ?? "", {
        submission_id: reviewQuery.data?.data.submission_id ?? "",
        file_path: selectedFile?.path ?? "",
        side: input.side,
        line_number: input.lineNumber,
        hunk_hash: input.hunkHash,
        diff_fingerprint: `${selectedFile?.path ?? ""}:${input.side}:${input.lineNumber}:${input.hunkHash}`,
        text: input.text,
      });
      return result.data;
    },
    onSuccess: (comment) => {
      setComments((prev) => [...prev, comment]);
    },
  });

  const decisionMutation = useMutation({
    mutationFn: async (decision: Decision) => {
      const dimensions = rubricQuery.data?.data.dimensions ?? [];
      const scoresArray: RubricScore[] = dimensions.map((dimension) => ({
        dimension: dimension.id,
        score: scores[dimension.id] ?? 0,
      }));
      return submitDecision(reviewId ?? "", { decision, scores: scoresArray, summary });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["review", reviewId] });
    },
  });

  if (!reviewId) {
    return <div className="review-page empty">请选择一次审核</div>;
  }

  const isDiffPending = !!submissionId && diffQuery.isPending;

  if (reviewQuery.isPending || isDiffPending) {
    return <div className="review-page loading">正在加载审核详情…</div>;
  }

  if (reviewQuery.isError) {
    return <div className="review-page error">无法加载审核：{reviewQuery.error.message}</div>;
  }

  if (diffQuery.isError) {
    return <div className="review-page error">无法加载 Diff：{diffQuery.error.message}</div>;
  }

  const review = reviewQuery.data.data;
  const decisionLabel = formatDecision(review.final_decision);
  const isSubmitted = review.status === "submitted";
  const mutationError = commentMutation.error ?? decisionMutation.error;

  function handleAddComment(input: {
    lineNumber: number;
    side: "left" | "right";
    hunkHash: string;
    text: string;
  }) {
    return commentMutation.mutateAsync(input);
  }

  return (
    <div className="review-page">
      <header className="review-header">
        <h1>审核 {review.id}</h1>
        <div className="review-meta">
          <span>状态：<strong>{formatStatus(review.status)}</strong></span>
          {decisionLabel ? <span>结论：<strong>{decisionLabel}</strong></span> : null}
          <span>Reviewer：{review.reviewer_id}</span>
          <span>Submission：{review.submission_id}</span>
          <RevisionSelector
            revisions={[{ id: review.submission_id, label: review.submission_id }]}
            selected={review.submission_id}
            onSelect={() => {
              // Revision 切换需要后端提供 revisions 列表；当前仅展示当前 Submission。
            }}
            label="Revision"
          />
        </div>
      </header>

      {mutationError ? (
        <div className="review-error" role="alert">
          {mutationError.message}
        </div>
      ) : null}

      <div className="review-decision-panel">
        {rubricQuery.data ? (
          <RubricForm
            dimensions={rubricQuery.data.data.dimensions}
            weights={rubricQuery.data.data.weights}
            scores={scores}
            onChange={setScores}
            readOnly={isSubmitted}
          />
        ) : rubricQuery.isPending ? (
          <div className="rubric-loading">正在加载评分表…</div>
        ) : rubricQuery.isError ? (
          <div className="rubric-error">无法加载评分表：{rubricQuery.error.message}</div>
        ) : null}

        <div className="review-summary-field">
          <label htmlFor="review-summary">审核总结</label>
          <textarea
            id="review-summary"
            rows={3}
            value={summary}
            disabled={isSubmitted}
            onChange={(e) => setSummary(e.target.value)}
            placeholder="输入审核总结…"
          />
        </div>

        {!isSubmitted ? (
          <div className="review-actions">
            <button
              type="button"
              className="accept"
              disabled={decisionMutation.isPending || rubricQuery.isPending}
              onClick={() => decisionMutation.mutate("accepted")}
            >
              通过
            </button>
            <button
              type="button"
              className="revision"
              disabled={decisionMutation.isPending || rubricQuery.isPending}
              onClick={() => decisionMutation.mutate("revision_requested")}
            >
              退回修改
            </button>
            <button
              type="button"
              className="reject"
              disabled={decisionMutation.isPending || rubricQuery.isPending}
              onClick={() => decisionMutation.mutate("rejected")}
            >
              拒绝
            </button>
          </div>
        ) : null}
      </div>

      {isSubmitted && review.rubric_scores.length > 0 ? (
        <div className="review-rubric">
          <h3>评分</h3>
          <ul>
            {review.rubric_scores.map((score) => (
              <li key={score.dimension}>
                {score.dimension}：<strong>{score.score}</strong>
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="review-body">
        <aside className="review-file-tree">
          <FileTree files={files} selected={selectedPath ?? ""} onSelect={setSelectedPath} />
        </aside>
        <main className="review-diff-pane">
          {selectedFile ? (
            <DiffViewer
              diff={selectedFile}
              comments={comments.filter((c) => c.file_path === selectedFile.path)}
              onAddComment={handleAddComment}
            />
          ) : (
            <div className="empty-diff">选择左侧文件查看 Diff</div>
          )}
        </main>
      </div>
    </div>
  );
}

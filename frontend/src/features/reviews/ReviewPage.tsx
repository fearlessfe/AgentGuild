import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { useEffect, useMemo, useState } from "react";
import { getReview, getSubmissionDiff } from "./reviews.api";
import { DiffViewer } from "./DiffViewer";
import { FileTree } from "./FileTree";
import type { Decision, FileDiff, LineComment, ReviewStatus } from "./reviews.types";

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
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [comments, setComments] = useState<LineComment[]>([]);

  const reviewQuery = useQuery({
    queryKey: ["review", reviewId],
    queryFn: () => getReview(reviewId ?? ""),
    enabled: !!reviewId,
  });

  const submissionId = reviewQuery.data?.data.submission_id;

  const diffQuery = useQuery({
    queryKey: ["submission-diff", submissionId],
    queryFn: () => getSubmissionDiff(submissionId ?? ""),
    enabled: !!submissionId,
  });

  const files = useMemo<FileDiff[]>(() => diffQuery.data?.data ?? [], [diffQuery.data]);

  useEffect(() => {
    if (files.length > 0 && !selectedPath) {
      setSelectedPath(files[0].path);
    }
  }, [files, selectedPath]);

  const selectedFile = useMemo(() => files.find((f) => f.path === selectedPath), [files, selectedPath]);

  // Seed local comments from the review response once it loads.
  useEffect(() => {
    if (reviewQuery.data) {
      setComments(reviewQuery.data.data.line_comments ?? []);
    }
  }, [reviewQuery.data]);

  if (!reviewId) {
    return <div className="review-page empty">请选择一次审核</div>;
  }

  if (reviewQuery.isPending || diffQuery.isPending) {
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

  function handleAddComment(line: number, side: "left" | "right", text: string) {
    const next: LineComment = {
      id: `local-${Date.now()}`,
      review_id: review.id,
      submission_id: review.submission_id,
      file_path: selectedFile?.path ?? "",
      side,
      line_number: line,
      hunk_hash: "",
      diff_fingerprint: "",
      text,
      created_at: new Date().toISOString(),
    };
    setComments((prev) => [...prev, next]);
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
        </div>
        {review.summary ? <p className="review-summary">{review.summary}</p> : null}
        {review.rubric_scores.length > 0 ? (
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
      </header>

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

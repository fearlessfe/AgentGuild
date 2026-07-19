import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { useEffect, useMemo, useRef, useState } from "react";
import { getSubmission, getTask, listExecutionSubmissions } from "../../api/client";
import { addComment, getActiveRubric, getReview, getSubmissionDiff, getSubmissionValidation, submitDecision } from "./reviews.api";
import { DiffViewer } from "./DiffViewer";
import { FileTree } from "./FileTree";
import { RevisionSelector } from "./RevisionSelector";
import { RubricForm } from "./RubricForm";
import { ValidationPanel } from "./ValidationPanel";
import { Card } from "../../ui";
import type { Decision, FileDiff, LineComment, ReviewStatus, Revision, RubricScore } from "./reviews.types";
import type { SubmissionStatus } from "../../api/client";

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

function formatSubmissionStatus(status: SubmissionStatus) {
  switch (status) {
    case "pending_verification":
      return "待验证";
    case "validated":
      return "已验证";
    case "validation_failed":
      return "验证失败";
    case "invalid":
      return "已失效";
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
  const [selectedSubmissionId, setSelectedSubmissionId] = useState<string | null>(null);
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
  const activeSubmissionId = selectedSubmissionId ?? submissionId;

  const submissionQuery = useQuery({
    queryKey: ["submission", submissionId],
    queryFn: () => getSubmission(submissionId ?? ""),
    enabled: !!submissionId,
    refetchOnWindowFocus: false,
  });

  const executionId = submissionQuery.data?.data.execution_id;
  const taskId = submissionQuery.data?.data.task_id;

  const revisionsQuery = useQuery({
    queryKey: ["execution-submissions", executionId],
    queryFn: () => listExecutionSubmissions(executionId ?? ""),
    enabled: !!executionId,
    refetchOnWindowFocus: false,
  });

  const taskQuery = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => getTask(taskId ?? ""),
    enabled: !!taskId,
    refetchOnWindowFocus: false,
  });

  const diffQuery = useQuery({
    queryKey: ["submission-diff", activeSubmissionId],
    queryFn: () => getSubmissionDiff(activeSubmissionId ?? ""),
    enabled: !!activeSubmissionId,
    placeholderData: keepPreviousData,
  });

  const validationQuery = useQuery({
    queryKey: ["submission-validation", activeSubmissionId],
    queryFn: () => getSubmissionValidation(activeSubmissionId ?? ""),
    enabled: !!activeSubmissionId,
    placeholderData: keepPreviousData,
  });

  const rubricQuery = useQuery({
    queryKey: ["active-rubric"],
    queryFn: () => getActiveRubric(),
    enabled: !!reviewId,
    refetchOnWindowFocus: false,
  });

  const files = useMemo<FileDiff[]>(() => diffQuery.data?.data ?? [], [diffQuery.data]);

  // 首个文件自动选中；切换修订后若所选文件不在新 Diff 中则回退到第一个文件。
  useEffect(() => {
    if (files.length > 0 && (!selectedPath || !files.some((f) => f.path === selectedPath))) {
      setSelectedPath(files[0].path);
    }
  }, [files, selectedPath]);

  const selectedFile = useMemo(() => files.find((f) => f.path === selectedPath), [files, selectedPath]);

  const revisions = useMemo<Revision[]>(() => {
    const submissions = revisionsQuery.data?.data ?? [];
    return [...submissions]
      .sort((a, b) => a.created_at.localeCompare(b.created_at))
      .map((submission, index) => ({
        id: submission.id,
        label: `R${index + 1} · ${submission.commit_sha.slice(0, 7)} · ${formatSubmissionStatus(submission.status)}`,
        created_at: submission.created_at,
      }));
  }, [revisionsQuery.data]);

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
    setSelectedSubmissionId(null);
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
  function handleAddComment(input: {
    lineNumber: number;
    side: "left" | "right";
    hunkHash: string;
    text: string;
  }) {
    return commentMutation.mutateAsync(input);
  }

  return (
    <div className="review-page stack">
      <header className="review-header card card-pad">
        <h1 className="card-title">审核 {review.id}</h1>
        <div className="review-meta">
          <span>状态：<strong>{formatStatus(review.status)}</strong></span>
          {decisionLabel ? <span>结论：<strong>{decisionLabel}</strong></span> : null}
          <span>Reviewer：{review.reviewer_id}</span>
          <span>Submission：{review.submission_id}</span>
          <RevisionSelector
            revisions={revisions.length > 0 ? revisions : [{ id: review.submission_id, label: review.submission_id }]}
            selected={activeSubmissionId ?? review.submission_id}
            onSelect={setSelectedSubmissionId}
            label="Revision"
          />
        </div>
      </header>

      <div className="split-3">
        <aside className="col scroll review-file-tree">
          <Card title="文件" pad={false}>
            <div className="card-pad">
              <FileTree files={files} selected={selectedPath ?? ""} onSelect={setSelectedPath} />
            </div>
          </Card>
        </aside>

        <main className="col scroll review-diff-pane">
          {selectedFile ? (
            <DiffViewer
              diff={selectedFile}
              comments={comments.filter((c) => c.file_path === selectedFile.path)}
              onAddComment={handleAddComment}
            />
          ) : (
            <div className="empty-diff muted">选择左侧文件查看 Diff</div>
          )}
        </main>

        <div className="col scroll">
          <Card title="任务验收条件">
            {taskQuery.isPending ? (
              <div className="muted">正在加载任务…</div>
            ) : taskQuery.isError ? (
              <div className="error">无法加载任务：{taskQuery.error.message}</div>
            ) : taskQuery.data ? (
              <div className="stack review-requirements">
                <div className="muted">{taskQuery.data.data.title}</div>
                {taskQuery.data.data.requirements && taskQuery.data.data.requirements.length > 0 ? (
                  <ul className="perm-list">
                    {taskQuery.data.data.requirements.map((requirement) => (
                      <li key={requirement}>{requirement}</li>
                    ))}
                  </ul>
                ) : (
                  <div className="muted">该任务未设置验收条件</div>
                )}
              </div>
            ) : null}
          </Card>

          <Card title="Rubric 评分">
            <div className="review-decision-panel stack">
              {decisionMutation.isError ? (
                <div className="form-error" role="alert">
                  {decisionMutation.error.message}
                </div>
              ) : null}

              {rubricQuery.data ? (
                <RubricForm
                  dimensions={rubricQuery.data.data.dimensions}
                  weights={rubricQuery.data.data.weights}
                  scores={scores}
                  onChange={setScores}
                  readOnly={isSubmitted}
                />
              ) : rubricQuery.isPending ? (
                <div className="rubric-loading muted">正在加载评分表…</div>
              ) : rubricQuery.isError ? (
                <div className="rubric-error error">无法加载评分表：{rubricQuery.error.message}</div>
              ) : null}

              <div className="review-summary-field field">
                <label className="field-label" htmlFor="review-summary">审核总结</label>
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
                <div className="review-actions row">
                  <button
                    type="button"
                    className="btn btn--primary accept"
                    disabled={decisionMutation.isPending || rubricQuery.isPending}
                    onClick={() => decisionMutation.mutate("accepted")}
                  >
                    {decisionMutation.isPending ? "提交中…" : "通过"}
                  </button>
                  <button
                    type="button"
                    className="btn revision"
                    disabled={decisionMutation.isPending || rubricQuery.isPending}
                    onClick={() => decisionMutation.mutate("revision_requested")}
                  >
                    {decisionMutation.isPending ? "提交中…" : "退回修改"}
                  </button>
                  <button
                    type="button"
                    className="btn btn--danger reject"
                    disabled={decisionMutation.isPending || rubricQuery.isPending}
                    onClick={() => decisionMutation.mutate("rejected")}
                  >
                    {decisionMutation.isPending ? "提交中…" : "拒绝"}
                  </button>
                </div>
              ) : null}
            </div>
          </Card>

          {isSubmitted && review.rubric_scores.length > 0 ? (
            <Card title="评分">
              <ul className="perm-list">
                {review.rubric_scores.map((score) => (
                  <li key={score.dimension}>
                    {score.dimension}：<strong>{score.score}</strong>
                  </li>
                ))}
              </ul>
            </Card>
          ) : null}

          <Card title="验证证据" sub={validationQuery.data ? `Submission ${validationQuery.data.data.submission_id}` : undefined}>
            {validationQuery.isPending ? (
              <div className="muted">正在加载验证结果…</div>
            ) : validationQuery.isError ? (
              <div className="error">无法加载验证结果：{validationQuery.error.message}</div>
            ) : validationQuery.data ? (
              <ValidationPanel job={validationQuery.data.data} />
            ) : null}
          </Card>
        </div>
      </div>
    </div>
  );
}

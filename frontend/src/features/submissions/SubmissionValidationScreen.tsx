import { useQuery } from "@tanstack/react-query";
import { GitPullRequest, RefreshCcw } from "lucide-react";
import { useRef, useState, type ReactNode } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { createIdempotencyKey, getSubmission, shouldRetainMutationKey, type SubmissionStatus } from "../../api/client";
import { PageHeader, Card, Button, ButtonLink, CheckRow, StatusChip } from "../../ui";
import type { CheckState } from "../../ui";
import { createReview } from "../reviews/reviews.api";

const STATUS: Record<SubmissionStatus, { label: string; tone: "success" | "warning" | "danger" | "info"; state: CheckState }> = {
  pending_verification: { label: "等待自动验证", tone: "info", state: "run" },
  validated: { label: "验证通过", tone: "success", state: "pass" },
  validation_failed: { label: "验证失败", tone: "danger", state: "fail" },
  invalid: { label: "提交无效", tone: "danger", state: "fail" },
};

export function SubmissionValidationScreen() {
  const { submissionId } = useParams<{ submissionId: string }>();
  const navigate = useNavigate();
  const [capabilities, setCapabilities] = useState("code-review");
  const [reviewError, setReviewError] = useState<string | null>(null);
  const [creatingReview, setCreatingReview] = useState(false);
  const reviewKey = useRef<string | null>(null);
  const query = useQuery({
    queryKey: ["submission", submissionId],
    queryFn: () => getSubmission(submissionId ?? ""),
    enabled: Boolean(submissionId),
    refetchInterval: (state) => state.state.data?.data.status === "pending_verification" ? 5_000 : false,
  });

  if (!submissionId) return <StateCard title="未指定提交" message="请从执行详情进入一次提交。" />;
  if (query.isPending) return <StateCard title="加载提交" message="正在读取提交与验证状态..." status />;
  if (query.isError) return <StateCard title="无法加载提交" message={query.error.message} retry={() => void query.refetch()} />;

  const submission = query.data.data;
  const status = STATUS[submission.status];
  const handleCreateReview = async () => {
    if (creatingReview || submission.status !== "validated") return;
    setCreatingReview(true);
    setReviewError(null);
    reviewKey.current ??= createIdempotencyKey();
    try {
      const result = await createReview(submission.id, parseCapabilities(capabilities), reviewKey.current);
      reviewKey.current = null;
      navigate(`/reviews/${encodeURIComponent(result.data.id)}`);
    } catch (err) {
      if (!shouldRetainMutationKey(err)) reviewKey.current = null;
      setReviewError(err instanceof Error ? err.message : "创建审核失败");
    } finally {
      setCreatingReview(false);
    }
  };

  return (
    <div className="stack">
      <PageHeader
        title="提交与验证"
        sub={`${submission.task_id} · ${submission.id}`}
        actions={<Button icon={<RefreshCcw size={14} />} onClick={() => void query.refetch()} disabled={query.isFetching}>{query.isFetching ? "刷新中..." : "刷新"}</Button>}
      />
      <div className="split-2">
        <div className="col">
          <Card title="提交信息" head={<StatusChip tone={status.tone}>{status.label}</StatusChip>}>
            <div className="fact-grid">
              <Fact label="repo"><code>{submission.repo}</code></Fact>
              <Fact label="branch"><code>{submission.branch}</code></Fact>
              <Fact label="base"><code>{submission.base_commit_sha}</code></Fact>
              <Fact label="commit"><code>{submission.commit_sha}</code></Fact>
              <Fact label="Execution"><code>{submission.execution_id}</code></Fact>
              <Fact label="Validation job"><code>{submission.validation_job_id || "尚未创建"}</code></Fact>
              <Fact label="创建时间">{formatDate(submission.created_at)}</Fact>
              <Fact label="更新时间">{formatDate(submission.updated_at)}</Fact>
            </div>
          </Card>
          <Card title="变更摘要">
            <p className="text-sm">{submission.summary}</p>
            {submission.tests ? <p className="field-hint">Agent 声明测试：{submission.tests}</p> : null}
          </Card>
          <div className="row">
            <ButtonLink to={`/executions/${encodeURIComponent(submission.execution_id)}`}>查看执行</ButtonLink>
            <ButtonLink to={`/tasks/${encodeURIComponent(submission.task_id)}`}>查看任务</ButtonLink>
          </div>
        </div>
        <div className="col">
          <Card title="验证状态" sub="以平台持久化结果为准" pad={false}>
            <CheckRow name="提交结构与 Commit 校验" state={submission.status === "invalid" ? "fail" : "pass"} meta={submission.status === "invalid" ? "提交不满足结构约束" : "提交已被平台接收"} />
            <CheckRow name="自动验证" state={status.state} meta={status.label} value={submission.validation_job_id || "—"} />
          </Card>
          <Card title="进入人工审核" sub="自动验证通过后，由平台分配 Reviewer。">
            <div className="stack-sm">
              <label className="field" htmlFor="review-capabilities">
                <span className="field-label">Reviewer 能力</span>
                <input id="review-capabilities" value={capabilities} onChange={(event) => { setCapabilities(event.target.value); reviewKey.current = null; }} placeholder="code-review, go" disabled={submission.status !== "validated" || creatingReview} />
                <span className="field-hint">用逗号分隔；留空时由平台按默认能力分配。</span>
              </label>
              {reviewError ? <p role="alert" className="form-error">{reviewError}</p> : null}
              <div className="row">
                <Button icon={<GitPullRequest size={14} />} variant="primary" onClick={() => void handleCreateReview()} disabled={submission.status !== "validated" || creatingReview}>
                  {creatingReview ? "创建中..." : "创建审核"}
                </Button>
              </div>
              {submission.status !== "validated" ? <p className="field-hint">当前状态为“{status.label}”，暂不能进入人工审核。</p> : null}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return <div className="fact"><span className="ctx-label">{label}</span><span>{children}</span></div>;
}

function StateCard({ title, message, retry, status = false }: { title: string; message: string; retry?: () => void; status?: boolean }) {
  return <Card title={title}><div className="stack-sm"><p className="text-sm" role={status ? "status" : retry ? "alert" : undefined}>{message}</p>{retry ? <div className="row"><Button onClick={retry}>重试</Button></div> : null}</div></Card>;
}

function parseCapabilities(value: string): string[] { return value.split(",").map((item) => item.trim()).filter(Boolean); }
function formatDate(value: string): string { return new Date(value).toLocaleString("zh-CN"); }

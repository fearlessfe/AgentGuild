import { StatusChip } from "../../ui";
import type { ValidationJobView, ValidationResourceUsage } from "./reviews.types";

type Tone = "action" | "success" | "warning" | "danger" | "info" | "neutral";

export function validationStatusTone(status: string): Tone {
  switch (status) {
    case "succeeded":
      return "success";
    case "failed":
      return "danger";
    case "running":
      return "info";
    case "skipped":
    case "cancelled":
      return "warning";
    default:
      return "neutral";
  }
}

export function formatValidationStatus(status: string) {
  switch (status) {
    case "pending":
      return "等待中";
    case "running":
      return "运行中";
    case "succeeded":
      return "通过";
    case "failed":
      return "失败";
    case "skipped":
      return "跳过";
    case "cancelled":
      return "已取消";
    default:
      return status;
  }
}

export function formatResourceUsage(usage?: ValidationResourceUsage): string | undefined {
  if (!usage) return undefined;
  const parts: string[] = [];
  if (typeof usage.elapsed_ms === "number") parts.push(`耗时 ${(usage.elapsed_ms / 1000).toFixed(1)}s`);
  if (typeof usage.cpu_seconds === "number") parts.push(`CPU ${usage.cpu_seconds.toFixed(1)}s`);
  if (typeof usage.peak_memory_mb === "number") parts.push(`峰值内存 ${usage.peak_memory_mb}MB`);
  return parts.length > 0 ? parts.join(" · ") : undefined;
}

export function ValidationPanel({ job }: { job: ValidationJobView }) {
  return (
    <div className="validation-panel stack" data-testid="validation-panel">
      <div className="review-meta">
        <span>
          状态：
          <StatusChip tone={validationStatusTone(job.status)}>{formatValidationStatus(job.status)}</StatusChip>
        </span>
        <span>尝试次数：{job.attempt}</span>
        <span>配置版本：{job.config_version}</span>
      </div>
      {job.steps.length > 0 ? (
        <ul className="perm-list validation-steps">
          {job.steps.map((step) => {
            const usage = formatResourceUsage(step.resource_usage);
            return (
              <li key={step.step} className="validation-step">
                <div className="row validation-step-head">
                  <strong>{step.step}</strong>
                  <StatusChip tone={validationStatusTone(step.status)}>{formatValidationStatus(step.status)}</StatusChip>
                  {step.hard_gate ? <StatusChip tone="warning">硬门槛</StatusChip> : null}
                </div>
                {step.log_summary ? <div className="muted">{step.log_summary}</div> : null}
                {usage ? <div className="muted">资源：{usage}</div> : null}
              </li>
            );
          })}
        </ul>
      ) : (
        <div className="muted">尚无验证步骤记录</div>
      )}
    </div>
  );
}

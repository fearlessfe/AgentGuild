import type { ReactNode } from "react";

type Tone = "action" | "success" | "warning" | "danger" | "info" | "neutral";

export function StatusChip({ children, tone = "neutral" }: { children: ReactNode; tone?: Tone }) {
  return <span className={`status-chip status-chip--${tone}`}>{children}</span>;
}

export type ApiStatus = "available" | "partial" | "planned" | "auth-fix";

const API_LABELS: Record<ApiStatus, string> = {
  available: "API · available",
  partial: "API · partial",
  planned: "API · planned",
  "auth-fix": "API · auth fix",
};

export function ApiNote({ status, children }: { status: ApiStatus; children: ReactNode }) {
  return (
    <p className="api-note" data-annotation="true">
      <span className={`api-chip api-chip--${status}`}>{API_LABELS[status]}</span>
      <span>{children}</span>
    </p>
  );
}

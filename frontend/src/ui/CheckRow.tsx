import type { ReactNode } from "react";

export type CheckState = "pass" | "fail" | "warn" | "run";

const MARKS: Record<CheckState, string> = { pass: "✓", fail: "✕", warn: "!", run: "…" };

export function CheckRow({
  name,
  state,
  meta,
  value,
}: {
  name: ReactNode;
  state: CheckState;
  meta?: ReactNode;
  value?: ReactNode;
}) {
  return (
    <div className="check-row">
      <span className={`check-mark check-mark--${state}`} aria-hidden="true">
        {MARKS[state]}
      </span>
      <div>
        <div className="check-name">{name}</div>
        {meta != null ? <div className="check-meta">{meta}</div> : null}
      </div>
      {value != null ? <span className="check-meta">{value}</span> : null}
    </div>
  );
}

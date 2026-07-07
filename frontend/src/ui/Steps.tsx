import type { ReactNode } from "react";

export type StepState = "done" | "current" | "pending";

export type Step = {
  title: ReactNode;
  state: StepState;
  desc?: ReactNode;
};

export function Steps({ items }: { items: Step[] }) {
  return (
    <ol className="steps">
      {items.map((item, index) => (
        <li className="step" data-state={item.state} key={index}>
          <span className="step-num">{item.state === "done" ? "✓" : index + 1}</span>
          <div>
            <div className="step-title">{item.title}</div>
            {item.desc != null ? <div className="step-desc">{item.desc}</div> : null}
          </div>
        </li>
      ))}
    </ol>
  );
}

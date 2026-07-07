import type { ReactNode } from "react";

export type Metric = {
  label: ReactNode;
  value: ReactNode;
  positive?: boolean;
};

export function MetricGrid({ metrics }: { metrics: Metric[] }) {
  return (
    <div className="metric-grid">
      {metrics.map((metric, index) => (
        <div className="metric" key={index}>
          <span className="metric-label">{metric.label}</span>
          <span className={`metric-value${metric.positive ? " metric-value--pos" : ""}`}>{metric.value}</span>
        </div>
      ))}
    </div>
  );
}

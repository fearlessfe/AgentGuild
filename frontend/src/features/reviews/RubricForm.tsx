import { useMemo } from "react";
import type { RubricDimension } from "./reviews.types";

export type RubricFormProps = {
  dimensions: RubricDimension[];
  weights: Record<string, number>;
  scores: Record<string, number>;
  onChange: (scores: Record<string, number>) => void;
  readOnly?: boolean;
};

export function RubricForm({ dimensions, weights, scores, onChange, readOnly = false }: RubricFormProps) {
  const total = useMemo(() => {
    return dimensions.reduce((sum, dimension) => {
      const score = scores[dimension.id] ?? 0;
      const weight = weights[dimension.id] ?? 0;
      return sum + score * weight;
    }, 0);
  }, [dimensions, scores, weights]);

  function handleChange(dimensionId: string, value: string) {
    if (readOnly) return;
    const parsed = value === "" ? 0 : Math.min(100, Math.max(0, Number(value)));
    onChange({ ...scores, [dimensionId]: Number.isNaN(parsed) ? 0 : parsed });
  }

  return (
    <div className="rubric-form" data-testid="rubric-form">
      <h3>Rubric 评分</h3>
      {dimensions.map((dimension) => (
        <div key={dimension.id} className="rubric-dimension">
          <label htmlFor={`rubric-${dimension.id}`}>{dimension.name}</label>
          <input
            id={`rubric-${dimension.id}`}
            type="number"
            min={0}
            max={100}
            value={scores[dimension.id] ?? ""}
            disabled={readOnly}
            onChange={(e) => handleChange(dimension.id, e.target.value)}
            aria-label={`${dimension.name} 分数`}
          />
          <span className="rubric-weight">权重 {(weights[dimension.id] ?? 0) * 100}%</span>
        </div>
      ))}
      <div className="rubric-total" data-testid="rubric-total">
        总分：{total.toFixed(1)}
      </div>
    </div>
  );
}

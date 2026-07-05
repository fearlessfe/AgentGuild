import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { RubricForm } from "./RubricForm";
import type { RubricDimension } from "./reviews.types";

const dimensions: RubricDimension[] = [
  { id: "correctness", name: "正确性" },
  { id: "readability", name: "可读性" },
  { id: "testing", name: "测试覆盖" },
];

const weights = {
  correctness: 0.5,
  readability: 0.3,
  testing: 0.2,
};

describe("RubricForm", () => {
  it("renders dimensions with labels and weights", () => {
    render(<RubricForm dimensions={dimensions} weights={weights} scores={{}} onChange={vi.fn()} />);

    expect(screen.getByLabelText("正确性 分数")).toBeVisible();
    expect(screen.getByLabelText("可读性 分数")).toBeVisible();
    expect(screen.getByLabelText("测试覆盖 分数")).toBeVisible();
    expect(screen.getByText("权重 50%")).toBeVisible();
    expect(screen.getByText("权重 30%")).toBeVisible();
    expect(screen.getByText("权重 20%")).toBeVisible();
  });

  it("calculates weighted total from scores", () => {
    render(
      <RubricForm
        dimensions={dimensions}
        weights={weights}
        scores={{ correctness: 80, readability: 60, testing: 50 }}
        onChange={vi.fn()}
      />
    );

    // 80*0.5 + 60*0.3 + 50*0.2 = 40 + 18 + 10 = 68.0
    expect(screen.getByTestId("rubric-total")).toHaveTextContent("总分：68.0");
  });

  it("calls onChange when a score changes", async () => {
    const onChange = vi.fn();
    render(<RubricForm dimensions={dimensions} weights={weights} scores={{}} onChange={onChange} />);

    fireEvent.change(screen.getByLabelText("正确性 分数"), { target: { value: "90" } });

    expect(onChange).toHaveBeenCalledWith({ correctness: 90 });
  });

  it("clamps scores outside 0-100", async () => {
    const onChange = vi.fn();
    render(
      <RubricForm
        dimensions={dimensions}
        weights={weights}
        scores={{ correctness: 0 }}
        onChange={onChange}
      />
    );

    fireEvent.change(screen.getByLabelText("正确性 分数"), { target: { value: "150" } });

    expect(onChange).toHaveBeenCalledWith({ correctness: 100 });
  });

  it("disables inputs when readOnly", () => {
    render(
      <RubricForm
        dimensions={dimensions}
        weights={weights}
        scores={{ correctness: 70 }}
        onChange={vi.fn()}
        readOnly
      />
    );

    expect(screen.getByLabelText("正确性 分数")).toBeDisabled();
  });
});

import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { OnboardingScreen } from "./OnboardingScreen";

describe("OnboardingScreen", () => {
  it("links GitHub setup to the git integration screen", () => {
    render(
      <MemoryRouter>
        <OnboardingScreen />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "连接 GitHub" })).toHaveAttribute("href", "/git-integration");
    expect(screen.getByRole("link", { name: "配置同步" })).toHaveAttribute("href", "/sync");
    expect(screen.queryByRole("button", { name: "保存并继续" })).not.toBeInTheDocument();
    expect(screen.queryByText("进度已保存")).not.toBeInTheDocument();
  });
});

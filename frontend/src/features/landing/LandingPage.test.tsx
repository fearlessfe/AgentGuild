import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LandingPage } from "./LandingPage";

describe("LandingPage", () => {
  beforeEach(() => {
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
    });
  });

  it("explains the open network and exposes the agent onboarding prompt", () => {
    render(<LandingPage />);

    expect(screen.getByRole("heading", { name: /让 Agent/ })).toBeInTheDocument();
    expect(screen.getByText("开放的任务网络")).toBeInTheDocument();
    expect(screen.getByText(/阅读接入协议/)).toBeInTheDocument();
  });

  it("confirms when the onboarding prompt is copied", async () => {
    render(<LandingPage />);

    await userEvent.click(screen.getByRole("button", { name: "复制接入指令" }));

    expect(screen.getByRole("button", { name: "复制接入指令" })).toHaveTextContent("已复制");
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining("AgentGuild"));
  });
});

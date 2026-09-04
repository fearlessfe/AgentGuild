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
    expect(screen.getByText(/生成密钥.*注册 Agent/)).toBeInTheDocument();
  });

  it("links public task and agent browsing to unauthenticated network pages", () => {
    render(<LandingPage />);

    expect(screen.getAllByRole("link", { name: /任务/ }).every((link) => link.getAttribute("href") === "/network/tasks" || link.getAttribute("href") === "/network" || link.getAttribute("href")?.startsWith("#"))).toBe(true);
    expect(screen.getAllByRole("link", { name: /Agents/ }).every((link) => link.getAttribute("href") === "/network/agents")).toBe(true);
  });

  it("keeps the landing navigation focused on the public network", () => {
    render(<LandingPage />);

    expect(screen.getAllByRole("link", { name: "网络概览" }).every((link) => link.getAttribute("href") === "/network")).toBe(true);
    expect(screen.getAllByRole("link", { name: "开放任务" }).every((link) => link.getAttribute("href") === "/network/tasks")).toBe(true);
    expect(screen.getAllByRole("link", { name: "管理入口" }).every((link) => link.getAttribute("href") === "/login")).toBe(true);
    expect(screen.getByRole("link", { name: "Agent 自助接入" })).toHaveAttribute("href", "/protocol#registration");
    expect(screen.queryByRole("link", { name: "工作台" })).not.toBeInTheDocument();
  });

  it("confirms when the onboarding prompt is copied", async () => {
    render(<LandingPage />);

    await userEvent.click(screen.getByRole("button", { name: "复制接入指令" }));

    expect(screen.getByRole("button", { name: "复制接入指令" })).toHaveTextContent("已复制");
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining("AgentGuild"));
  });
});

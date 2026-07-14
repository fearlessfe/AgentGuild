import { render, screen, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { beforeEach, describe, expect, it } from "vitest";
import { AppShell } from "../../app/AppShell";
import { OnboardingScreen } from "../onboarding/OnboardingScreen";
import { SyncRuleScreen } from "./SyncRuleScreen";

describe("Navigation and sync integration", () => {
  beforeEach(() => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
  });

  it("surfaces repository onboarding from the persistent rail", async () => {
    render(
      <MemoryRouter initialEntries={["/sync"]}>
        <AppShell />
      </MemoryRouter>,
    );

    expect(within(screen.getByRole("navigation", { name: "主导航" })).getByRole("link", { name: "仓库接入" })).toHaveAttribute(
      "href",
      "/repositories",
    );
  });

  it("routes /repositories to the repository onboarding screen", async () => {
    render(
      <MemoryRouter initialEntries={["/repositories"]}>
        <AppShell />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("heading", { name: "仓库接入" })).toBeVisible();
    expect(screen.getByRole("group", { name: "仓库来源" })).toBeVisible();
  });

  it("keeps GitHub App setup reachable while sending repository setup to /repositories", () => {
    render(
      <MemoryRouter>
        <OnboardingScreen />
      </MemoryRouter>,
    );

    expect(screen.getByRole("link", { name: "连接 GitHub" })).toHaveAttribute("href", "/git-integration");
    expect(screen.getByRole("link", { name: "设置仓库" })).toHaveAttribute("href", "/repositories");
  });

  it("keeps sync rules focused on rule management and links to repository setup", async () => {
    render(
      <MemoryRouter>
        <SyncRuleScreen />
      </MemoryRouter>,
    );

    expect(await screen.findByRole("table", { name: "同步规则列表" })).toBeVisible();
    expect(screen.queryByText("公共仓库同步")).toBeNull();
    expect(screen.queryByRole("button", { name: "添加公共规则" })).toBeNull();
    expect(screen.getByRole("link", { name: "仓库接入" })).toHaveAttribute("href", "/repositories");
  });
});

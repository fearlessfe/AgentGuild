import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import * as client from "../../api/client";
import { RepositoryOnboardingScreen } from "./RepositoryOnboardingScreen";

describe("RepositoryOnboardingScreen", () => {
  beforeEach(() => {
    (import.meta.env as Record<string, string | undefined>).VITE_DEMO_MODE = "true";
  });

  it("renders the two-step onboarding areas and added repositories from demo mode", async () => {
    render(<RepositoryOnboardingScreen />);

    expect(await screen.findByRole("heading", { name: "仓库接入" })).toBeVisible();
    expect(screen.getByText("Step 1 · GitHub App")).toBeVisible();
    expect(screen.getByText("Step 2 · 添加仓库")).toBeVisible();
    expect(await screen.findByText("acme/event-gateway")).toBeVisible();

    const table = await screen.findByRole("table", { name: "已接入仓库列表" });
    expect(within(table).getByText("acme/billing-service")).toBeVisible();
    expect(within(table).getByText("vercel/next.js")).toBeVisible();
    expect(within(table).getByText("GitHub App")).toBeVisible();
    expect(within(table).getByText("公开仓库")).toBeVisible();
  });

  it("adds a GitHub App repository candidate to the added repository list", async () => {
    render(<RepositoryOnboardingScreen />);

    const candidateRow = await screen.findByRole("row", { name: /acme\/event-gateway/ });
    await userEvent.click(within(candidateRow).getByRole("button", { name: "添加" }));

    const table = await screen.findByRole("table", { name: "已接入仓库列表" });
    await waitFor(() => expect(within(table).getByText("acme/event-gateway")).toBeVisible());
  });

  it("adds a public GitHub URL without creating an issue sync rule", async () => {
    const createSyncRuleSpy = vi.spyOn(client, "createSyncRule");

    render(<RepositoryOnboardingScreen />);

    await userEvent.type(await screen.findByLabelText("公共仓库 URL 或 owner/repo"), "https://github.com/rust-lang/rust");
    await userEvent.click(screen.getByRole("button", { name: "添加公开仓库" }));

    const table = await screen.findByRole("table", { name: "已接入仓库列表" });
    await waitFor(() => expect(within(table).getByText("rust-lang/rust")).toBeVisible());
    expect(createSyncRuleSpy).not.toHaveBeenCalled();
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import * as client from "../../api/client";
import { GitIntegrationScreen } from "./GitIntegrationScreen";

describe("GitIntegrationScreen", () => {
  const originalLocation = window.location;

  beforeEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: { ...originalLocation, href: "" },
    });
    vi.spyOn(client, "githubManifestUrl").mockReturnValue("/api/oauth/github/app/manifest");
    vi.spyOn(client, "githubInstallUrl").mockReturnValue("/api/oauth/github/app/install");
  });

  afterEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
    vi.restoreAllMocks();
  });

  it("shows install action after GitHub App creation before installation", async () => {
    vi.spyOn(client, "getGitHubApp").mockResolvedValue({
      data: {
        app_id: 123,
        app_slug: "agentguild-test",
        installation_id: 0,
        configured: true,
      },
      meta: { server_time: "", resource_version: 0 },
    });

    render(<GitIntegrationScreen />);

    expect(await screen.findByText("已创建，待安装")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "安装 GitHub App" }));

    expect(window.location.href).toBe("/api/oauth/github/app/install");
  });

  it("keeps connection test visible after GitHub App installation", async () => {
    vi.spyOn(client, "getGitHubApp").mockResolvedValue({
      data: {
        app_id: 123,
        app_slug: "agentguild-test",
        installation_id: 456,
        configured: true,
      },
      meta: { server_time: "", resource_version: 0 },
    });

    render(<GitIntegrationScreen />);

    expect(await screen.findByText("已安装")).toBeVisible();
    expect(screen.getByRole("button", { name: "检测连接" })).toBeVisible();
  });

  it("asks admins to reconnect when an existing App is missing an installable slug", async () => {
    vi.spyOn(client, "getGitHubApp").mockResolvedValue({
      data: {
        app_id: 123,
        installation_id: 0,
        configured: true,
      },
      meta: { server_time: "", resource_version: 0 },
    });

    render(<GitIntegrationScreen />);

    expect(await screen.findByText("已创建，待安装")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "重新连接 GitHub" }));

    expect(window.location.href).toBe("/api/oauth/github/app/manifest");
  });
});

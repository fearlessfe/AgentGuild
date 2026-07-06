import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  const originalLocation = window.location;

  beforeEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: { ...originalLocation, href: "" },
    });
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: originalLocation,
    });
    vi.unstubAllEnvs();
    vi.restoreAllMocks();
  });

  it("renders the OIDC login button", () => {
    render(<LoginPage />);
    expect(screen.getByRole("button", { name: /使用企业 OIDC 登录/i })).toBeInTheDocument();
  });

  it("redirects to the OIDC login endpoint when clicked", async () => {
    render(<LoginPage />);
    await userEvent.click(screen.getByRole("button", { name: /使用企业 OIDC 登录/i }));
    expect(window.location.href).toBe("/oauth/oidc/login");
  });

  it("redirects to the home page in demo mode", async () => {
    vi.stubEnv("VITE_DEMO_MODE", "true");
    render(<LoginPage />);
    await userEvent.click(screen.getByRole("button", { name: /使用企业 OIDC 登录/i }));
    expect(window.location.href).toBe("/agents");
  });

  it("submits local password and redirects on success", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: true, json: async () => ({ data: { authenticated: true } }) });
    render(<LoginPage />);
    await userEvent.type(screen.getByPlaceholderText(/Local admin password/i), "secret");
    await userEvent.click(screen.getByRole("button", { name: /本地登录/i }));
    expect(fetch).toHaveBeenCalledWith(
      "/oauth/local/login",
      expect.objectContaining({ method: "POST", body: JSON.stringify({ password: "secret" }) })
    );
    await vi.waitFor(() => expect(window.location.href).toBe("/agents"));
  });

  it("shows error when local password is wrong", async () => {
    (fetch as ReturnType<typeof vi.fn>).mockResolvedValue({ ok: false });
    render(<LoginPage />);
    await userEvent.type(screen.getByPlaceholderText(/Local admin password/i), "wrong");
    await userEvent.click(screen.getByRole("button", { name: /本地登录/i }));
    await vi.waitFor(() => expect(screen.getByText(/密码错误或未启用本地登录/i)).toBeInTheDocument());
  });
});

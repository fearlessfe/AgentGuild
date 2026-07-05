import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  const originalLocation = window.location;
  const originalEnv = import.meta.env;

  beforeEach(() => {
    delete (window as Window & { location?: Location }).location;
    window.location = { ...originalLocation, href: "" } as Location;
  });

  afterEach(() => {
    window.location = originalLocation;
    import.meta.env = originalEnv;
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
});

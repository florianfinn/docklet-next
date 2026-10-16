// Source: docklet-next/docs/design/identity.md
// Revision: 7f4c05631ea6c769bb1cfae85c081b9b8b83df17
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Mode matrix for connected route guards.

import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Outlet } from "react-router";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { renderRoutes } from "@/test/render";

import { AuthGate } from "./AuthGate";
import ProductLoginPage from "@/features/auth/login/ProductLoginPage";

afterEach(() => vi.restoreAllMocks());

const denied = () => new AdapterOutcomeError({ kind: "error", code: "http.error", status: 401 });

describe.each(["none", "local", "proxy"] as const)("%s sign-in routing", (mode) => {
  it("handles a fresh load and a rejected session", async () => {
    const adapter = createDemoAdapter();
    adapter.getAuthMode = vi.fn(async () => mode);
    adapter.getCurrentUser = vi.fn(async () => { if (mode === "local") throw denied(); return { id: "admin", username: "admin", requiresPasswordChange: false }; });
    const { router } = renderRoutes([
      { path: "/login", element: <ProductLoginPage /> },
      { element: <AuthGate />, children: [{ element: <Outlet />, children: [{ path: "/dashboard", element: <p>Dashboard</p> }] }] },
    ], ["/dashboard"], adapter);
    if (mode === "local") await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    else expect(await screen.findByText("Dashboard")).not.toBe(null);

    if (mode !== "local") {
      adapter.getCurrentUser = vi.fn(async () => { throw denied(); });
      await router.navigate("/login");
      if (mode === "none") await waitFor(() => expect(router.state.location.pathname).toBe("/dashboard"));
      else expect(await screen.findByRole("alert")).not.toBe(null);
    }
  });
});

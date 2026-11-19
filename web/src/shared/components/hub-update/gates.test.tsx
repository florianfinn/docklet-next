// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original real gate classifier and automatic retry behaviour coverage.

import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { CancelledError } from "@tanstack/react-query";
import { IntlProvider } from "use-intl";
import { afterEach, expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { AuthGate } from "@/app/AuthGate";
import { SetupGate } from "@/app/SetupGate";
import ProductLoginPage from "@/features/auth/login/ProductLoginPage";
import { AdapterOutcomeError } from "@/data/capability";
import { createDisconnectedAdapter } from "@/data/disconnected-adapter";
import { createHttpAdapter } from "@/data/http/http-adapter";
import { renderRoutes } from "@/test/render";
import { UnreachableGate } from "../gate-screen";
import { isHubUnreachable, useHubRetry } from "./gate-retry";

afterEach(() => { cleanup(); vi.useRealTimers(); vi.restoreAllMocks(); });
it.each([undefined, 502, 503, 504, 400, 401, 403, 422, 500])("B11 classifies actual auth HTTP errors with status %s", async status => {
  const adapter = createHttpAdapter({ fetch: async () => { if (status === undefined) throw new TypeError("offline"); return new Response("<html>Gateway</html>", { status }); } });
  const error = await adapter.getAuthMode().catch(error => error);
  expect(isHubUnreachable(error)).toBe(status === undefined || [502, 503, 504].includes(status));
});
it.each([new CancelledError(), new Error("auth.invalid-response"), new TypeError("offline"), new AdapterOutcomeError({ kind: "error", code: "backup.maintenance", status: 503 }), new AdapterOutcomeError({ kind: "error", code: "activity.invalid-data" })])("B11 excludes cancellation, invalid data and maintenance %s", error => { expect(isHubUnreachable(error)).toBe(false); });
it.each([["auth", AuthGate], ["setup", SetupGate], ["login", ProductLoginPage]] as const)("B11 real gate %s shows an unreachable sibling before pending mode", async (_name, Gate) => {
  const adapter = { ...createDisconnectedAdapter(), getAuthMode: () => new Promise<"local">(() => {}), getAuthConfiguration: async () => ({ mode: "local" as const, setupPending: false, forced: false }), getCurrentUser: async () => { throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: 503 }); } };
  const result = renderRoutes([{ path: "/", element: <Gate /> }], ["/"], adapter);
  expect(await screen.findByText("Der Hub ist nicht erreichbar. Wird automatisch erneut versucht…") !== null).toBe(true);
  expect(document.querySelector('[data-slot="gate-screen"] [data-slot="brand-logo"] svg') !== null).toBe(true);
  result.router.dispose();
});
it("B-1 the sign-in page keeps its title as the page heading, not as an alert", async () => {
  const adapter = { ...createDisconnectedAdapter(), getAuthMode: async () => "local" as const, getAuthConfiguration: async () => ({ mode: "local" as const, setupPending: false, forced: false }), getCurrentUser: async () => { throw new AdapterOutcomeError({ kind: "error", code: "auth.unauthorized", status: 401 }); } };
  const result = renderRoutes([{ path: "/", element: <ProductLoginPage /> }], ["/"], adapter);
  await screen.findByRole("button", { name: "Anmelden" });
  const headings = screen.queryAllByRole("heading", { level: 1 }).map((h) => h.textContent);
  expect([headings.length === 1 && /Docklet/.test(headings[0] ?? ""), screen.queryByRole("alert") === null]).toEqual([true, true]);
  result.router.dispose();
});
it("B11 backoff is 5/10/20/30 seconds, manual retry never overlaps and unmount cancels", async () => {
  vi.useFakeTimers(); let release: (() => void) | undefined;
  const refetch = vi.fn(async () => { await new Promise<void>(resolve => { release = resolve; }); });
  function Probe() { const retry = useHubRetry([{ error: new AdapterOutcomeError({ kind: "error", code: "http.error" }), isFetching: false, refetch }]); return <UnreachableGate {...retry} />; }
  const f = render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><Probe /></IntlProvider>);
  const tick = async (ms: number) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });
  await tick(4999); expect(refetch).not.toHaveBeenCalled(); await tick(1); expect(refetch).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Reload" })); await tick(30_000); expect(refetch).toHaveBeenCalledTimes(1);
  await act(async () => release?.()); await tick(9999); expect(refetch).toHaveBeenCalledTimes(1); await tick(1); expect(refetch).toHaveBeenCalledTimes(2);
  await act(async () => release?.()); await tick(19_999); expect(refetch).toHaveBeenCalledTimes(2); await tick(1); expect(refetch).toHaveBeenCalledTimes(3);
  await act(async () => release?.()); await tick(29_999); expect(refetch).toHaveBeenCalledTimes(3); await tick(1); expect(refetch).toHaveBeenCalledTimes(4);
  f.unmount(); await act(async () => release?.()); await tick(60_000); expect(refetch).toHaveBeenCalledTimes(4);
});

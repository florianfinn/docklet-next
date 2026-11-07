// Source: docklet-next/issues/360
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original shell navigation read-count proofs under the real HTTP adapter.

import { act, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useContainers } from "@/data/queries";
import { hardeningFixture } from "@/data/http/web-hardening.test-support";
import { renderRoutes } from "@/test/render";
import { setAppLocale } from "@/app/i18n/locale-store";
import { AppShell } from "./AppShell";

function Page() { const rows = useContainers(); return <h1>{rows.data?.map((c) => c.id).join(",") ?? "loading"}</h1>; }
afterEach(() => { vi.useRealTimers(); setAppLocale("de"); });
it.each(["/hosts", "/containers"])("WH-B2 shell counts at %s use lists without topology detail fan-out", async (path) => {
  setAppLocale("en-US"); const f = hardeningFixture();
  renderRoutes([{ element: <AppShell />, children: [{ path, element: <Page /> }] }], [path], f.adapter);
  await screen.findByRole("heading", { name: "first~web,first~db,second~web,second~db" });
  await waitFor(() => expect(document.querySelector('a[href="/networks"]')?.closest('[data-sidebar="menu-item"]')?.querySelector('[data-sidebar="menu-badge"]')?.textContent).toBe("2"));
  for (const [path, count] of [["/stacks", "2"], ["/containers", "4"]]) expect(document.querySelector(`a[href="${path}"]`)?.closest('[data-sidebar="menu-item"]')?.querySelector('[data-sidebar="menu-badge"]')?.textContent).toBe(count);
  expect(f.calls.filter((p) => /\/networks\/[^/?]+/.test(p))).toEqual([]);
  expect(f.calls.some((p) => /\/networks\?/.test(p))).toBe(true);
  expect(f.calls.filter((p) => /\/projects\/project(?:\?|$)/.test(p))).toEqual([]);
});

it("PX-B5 held host shell reads settle at the logical 30 s budget while healthy inventory renders", async () => {
  vi.useFakeTimers(); const f = hardeningFixture(); f.state.heldHost = "second";
  const view = renderRoutes([{ element: <AppShell />, children: [{ path: "/containers", element: <Page /> }] }], ["/containers"], f.adapter);
  try {
    await act(async () => { await vi.advanceTimersByTimeAsync(100); });
    expect(screen.getByRole("heading").textContent).toBe("first~web,first~db");
    expect(f.state.heldSignals.length > 0).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(29_899); });
    expect(f.state.heldSignals.every((s) => !s.aborted)).toBe(true);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    const expired = f.state.heldSignals.every((s) => s.aborted);
    await act(async () => { await vi.advanceTimersByTimeAsync(1); });
    expect(document.querySelector('a[href="/networks"]')?.closest('[data-sidebar="menu-item"]')?.querySelector('[data-sidebar="menu-badge"]')?.textContent).toBe("1");
    expect(expired).toBe(true);
    expect(screen.getByRole("heading").textContent).toBe("first~web,first~db");
    expect((await f.adapter.listHosts()).find((h) => h.id === "second")?.inventory?.stale).toBe(true);
  } finally { view.unmount(); }
});

it("WH-B2 the cheap count preserves the topology system-network exclusion", async () => {
  const f = hardeningFixture();
  f.inventory.state.networks.push({ ...f.inventory.state.networks[0]!, id: "agent-network", labels: { "io.docklet.system": "agent" } });
  expect((await f.adapter.listNetworkSummaries()).map((row) => row.id)).toEqual(["net", "net"]);
  expect(f.calls.filter((call) => /\/networks\/[^/?]+/.test(call))).toEqual([]);
});

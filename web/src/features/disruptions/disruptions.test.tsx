// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original producer-shaped timeline and log-read regression tests for #693.

import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/data/query-keys";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderWithApp, renderRoutes } from "@/test/render";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { logFixture, settleLogs } from "@/data/http/logs.test-support";
import { AuthGate } from "@/app/AuthGate";
import { AppShell } from "@/app/shell/AppShell";
import { decodeSidebar, sidebarStore, useSidebarOpen } from "@/shared/preferences/sidebar-store";
import type { HostContainerHistory, HistoryBucket } from "@/domain/history";
import { mapHostContainerHistory } from "@/data/http/history";
import DisruptionsPage from "./DisruptionsPage";
import { CombinedWindow } from "./CombinedWindow";
import type { LogsApi } from "@/data/api";
import { selectWindow, type WindowSelection } from "./participants";
const bucket: HistoryBucket = { start: "2026-01-02T03:00:00Z", end: "2026-01-02T03:15:00Z", state: "down", minutes: { running: 0, down: 1, unhealthy: 0, stopped: 0 } };
const history = (hostId: string, name: string): HostContainerHistory => ({ hostId, range: "24h", truncated: false, containers: [{ name, buckets: [bucket] }] });
const selection: WindowSelection = { since: "2026-01-02T02:55:00Z", until: "2026-01-02T03:20:00Z", omitted: 0, participants: [{ hostId: "atlas", host: "Atlas", name: "web", id: "atlas~old", minutes: 1 }, { hostId: "boreal", host: "Boreal", name: "api", id: "boreal~api", minutes: 1 }] };
beforeEach(() => { setAppLocale("en-US"); simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { vi.unstubAllGlobals(); setAppLocale("de"); simulationStore.reset(); sidebarStore.reset(); });
it("the error jump visits warnings too and starts at the last marked line going back", async () => {
  const user = userEvent.setup();
  const readLogs = vi.fn<LogsApi["readLogs"]>(async (_ref, _options, sink) => {
    sink([{ id: "1", service: "web", ts: "2026-01-02T03:00:00Z", level: "error", message: "first failure" }, { id: "2", service: "web", ts: "2026-01-02T03:01:00Z", level: "info", message: "calm" }, { id: "3", service: "web", ts: "2026-01-02T03:02:00Z", level: "warn", message: "late warning" }]);
    return { reason: "end", lines: 3, bytes: 30 };
  });
  renderRoutes([{ path: "/", element: <CombinedWindow selection={{ ...selection, participants: [selection.participants[0]!] }} /> }], ["/"], { ...createDemoAdapter(), readLogs });
  await screen.findByText(/late warning/);
  await user.click(screen.getByRole("button", { name: "Previous error/warning" }));
  expect(screen.getByText(/late warning/).className.includes("bg-muted")).toBe(true);
});

it("S2.6 combines real transport reads, drops an out-of-window middle frame and retains later frames", async () => {
  const f = logFixture(), adapter = { ...createDemoAdapter(), readLogs: f.owner.readLogs };
  renderWithApp(<CombinedWindow selection={selection} />, adapter);
  await act(async () => { await settleLogs(); });
  expect(f.sockets).toHaveLength(2);
  await act(async () => {
    for (const [i, socket] of f.sockets.entries()) {
      socket.onopen?.(new Event("open"));
      socket.send([{ seq: 1, level: "info", message: `ERROR first-${i}`, timestamp: "2026-01-02T03:01:00Z" }, { seq: 2, level: "info", message: "outside", timestamp: "2026-01-01T00:00:00Z" }, { seq: 3, level: "info", message: `last-${i}`, timestamp: "2026-01-02T03:02:00Z" }]); socket.disconnect();
    }
    await settleLogs();
  });
  expect(screen.queryByText(/outside/) === null).toBe(true);
  expect(screen.getByText(/last-1/).textContent).toContain("api · Boreal [boreal~api]");
  const logs = [...document.querySelectorAll('li[id^="disruption-log-"]')].map((x) => x.textContent);
  expect(logs).toHaveLength(4); expect(logs[0]).toContain("first-0"); expect(logs[1]).toContain("first-1"); expect(logs[2]).toContain("last-0");
  for (const url of f.urls) { const u = new URL(url); expect(u.searchParams.get("until")).toBe(selection.until); expect(u.searchParams.get("tail")).toBe("all"); }
});
it("S2.8 never shows a held old range under the new range", async () => {
  const adapter = createDemoAdapter(); const hosts = (await adapter.listHosts()).slice(0, 1); adapter.listHosts = async () => hosts;
  let release!: (h: HostContainerHistory) => void;
  const old = new Promise<HostContainerHistory>((resolve) => { release = resolve; });
  adapter.getHostContainerHistory = vi.fn((_id, range) => range === "24h" ? old : Promise.resolve({ ...history(hosts[0]!.id, "new-range"), range }));
  renderWithApp(<DisruptionsPage />, adapter);
  await waitFor(() => expect(adapter.getHostContainerHistory).toHaveBeenCalled());
  await userEvent.click(screen.getByRole("radio", { name: "7 d" }));
  await act(async () => { release(history(hosts[0]!.id, "old-range")); });
  await waitFor(() => expect(screen.queryByText("Loading…") === null).toBe(true));
  expect(screen.queryByText("old-range") === null).toBe(true); await screen.findByText("new-range");
});
it("S2.11 freezes the read identity when inventory recreates a container", async () => {
  const adapter = createDemoAdapter(); const hosts = await adapter.listHosts(), containers = await adapter.listContainers(); const c = { ...containers[0]!, id: `${containers[0]!.hostId}~old` }; containers[0] = c;
  const h = history(c.hostId, c.name);
  const f = logFixture(); adapter.readLogs = f.owner.readLogs;
  adapter.listHosts = async () => hosts.filter((h) => h.id === c.hostId);
  adapter.listContainers = async () => containers;
  adapter.getHostContainerHistory = async () => h;
  function InventoryChange() {
    const client = useQueryClient();
    return <button onClick={() => client.setQueryData(queryKeys.containers, containers.map((row) => row.id === c.id ? { ...row, id: `${c.hostId}~replacement` } : row))}>Recreate fixture</button>;
  }
  renderWithApp(<><DisruptionsPage /><InventoryChange /></>, adapter);
  await userEvent.click(await screen.findByRole("button", { name: /Down, 2026/ }));
  await act(async () => { await settleLogs(); });
  expect(f.sockets).toHaveLength(1);
  await userEvent.click(screen.getByRole("button", { name: "Recreate fixture" }));
  await waitFor(() => expect(screen.getByRole("link", { name: c.name }).getAttribute("href")).toContain("replacement"));
  await act(async () => {
    f.sockets[0]!.onopen?.(new Event("open")); f.sockets[0]!.send({ seq: 1, level: "info", message: "held result", timestamp: "2026-01-02T03:01:00Z" }); f.sockets[0]!.disconnect(); await settleLogs(); });
  expect(screen.getByText(/held result/).textContent).toContain(`[${c.id}]`);
  expect(screen.getByText(/held result/).textContent?.includes("replacement")).toBe(false);
  expect(new URL(f.urls[0]!).pathname).not.toContain("replacement");
});
it("S2.12 leaves the timeline and other participant visible after unavailable", async () => {
  const adapter = createDemoAdapter(); const hosts = (await adapter.listHosts()).slice(0, 2); adapter.listHosts = async () => hosts;
  const containers = await adapter.listContainers(); const participants = hosts.map((h) => containers.find((c) => c.hostId === h.id)!);
  adapter.getHostContainerHistory = async (id) => history(id, participants.find((c) => c.hostId === id)!.name);
  adapter.readLogs = async (ref, _opts, sink) => { if (ref.id === participants[0]!.id) return { reason: "unavailable", lines: 0, bytes: 0 }; sink([{ id: "ok", ts: "2026-01-02T03:01:00Z", service: "", level: "info", message: "other participant survives" }]); return { reason: "end", lines: 1, bytes: 20 }; };
  renderWithApp(<DisruptionsPage />, adapter);
  await userEvent.click((await screen.findAllByRole("button", { name: /Down, 2026/ }))[0]!);
  await screen.findByText(/other participant survives/); expect(screen.queryByText("Logs not available for this container") !== null).toBe(true);
  expect(screen.getAllByRole("button", { name: /Down, 2026/ })).toHaveLength(2);
});
it("S2.13 missing and invalid preferences default off without changing open", () => {
  expect(decodeSidebar({ open: false })).toEqual({ open: false, disruptions: false });
  expect(decodeSidebar({ open: true, disruptions: "true" })).toEqual({ open: true, disruptions: false });
  expect(decodeSidebar({ disruptions: true })).toEqual({ open: true, disruptions: true });
});
it("S2.14 denies the actual simple-user shell route and never mounts data consumers", async () => {
  const adapter = { ...createDemoAdapter(), kind: "http" as const, getCurrentUser: async () => ({ id: "member", username: "member", isAdmin: false, assignments: [], requiresPasswordChange: false }) };
  const mount = vi.spyOn(adapter, "getHostContainerHistory"); sidebarStore.set({ open: true, disruptions: true });
  renderRoutes([{ element: <AuthGate />, children: [{ element: <AppShell />, children: [{ path: "/disruptions", element: <DisruptionsPage /> }] }] }], ["/disruptions"], adapter);
  await waitFor(() => expect(mount.mock.calls.length > 0 || screen.queryByRole("heading", { name: "Not available" }) !== null).toBe(true));
  expect(mount).not.toHaveBeenCalled(); expect(screen.queryByRole("heading", { name: "Not available" }) !== null).toBe(true); expect(document.querySelector('a[href="/disruptions"]') === null).toBe(true);
});
it("validates host lane responses before rendering", () => {
  const wire = { ...history("atlas", "web"), containers: [{ name: "web", buckets: [{ ...bucket, exitCode: 1 }] }] };
  expect(mapHostContainerHistory(wire, "atlas", "24h").containers[0]!.buckets[0]!.end).toBe("2026-01-02T03:15:00.000Z");
  for (const invalid of [{ ...wire, range: "7d" }, { ...wire, hostId: "boreal" }, { ...wire, containers: [{}] }, { ...wire, containers: undefined }, { ...wire, containers: [{ name: "web", buckets: [{ ...bucket, start: 12, exitCode: 1 }] }] }, { ...wire, containers: [{ name: "web", buckets: [{ ...bucket, exitCode: 1, minutes: { down: -1 } }] }] }]) expect(() => mapHostContainerHistory(invalid, "atlas", "24h")).toThrow();
});
it("selects overlapping cross-host participants by disrupted minutes, capped at 20", async () => {
  const demo = createDemoAdapter({ logHealthInsights: true }); const hosts = await demo.listHosts(), containers = await demo.listContainers();
  const histories = await Promise.all(hosts.map((h) => demo.getHostContainerHistory(h.id, "24h")));
  const affected = histories.filter((h) => h.containers.length); expect(affected.length).toBeGreaterThanOrEqual(2);
  const selected = selectWindow(affected[0]!.containers[0]!.buckets.at(-1)!, histories, hosts, containers);
  expect(new Set(selected.participants.map((p) => p.hostId)).size).toBeGreaterThanOrEqual(2);
  for (const p of selected.participants) { const sink = vi.fn(); await demo.readLogs({ kind: "container", id: p.id! }, { since: selected.since, until: selected.until, tail: "all", maxLines: 5000, maxBytes: 2097152 }, sink, new AbortController().signal); expect(sink.mock.calls[0]![0].length).toBeGreaterThan(0); }
  const many = Array.from({ length: 25 }, (_, i) => history("atlas", `web-${i}`));
  expect(selectWindow(bucket, many, hosts, []).participants).toHaveLength(20);
  expect(selectWindow(bucket, many, hosts, []).omitted).toBe(5);
});
it("aborts all participant reads when the combined window leaves the page", async () => {
  const f = logFixture();
  const ui = renderWithApp(<CombinedWindow selection={selection} />, { ...createDemoAdapter(), readLogs: f.owner.readLogs });
  await act(async () => { await settleLogs(); });
  expect(f.sockets).toHaveLength(2);
  ui.unmount();
  for (const socket of f.sockets) expect(socket.close).toHaveBeenCalledOnce();
});
it("keeps both entry links admin-only and preserves the preference when collapsing the sidebar", async () => {
  const { DisruptionsLink } = await import("./DisruptionsLink");
  const { AppearanceSection } = await import("@/features/settings/sections/AppearanceSection");
  function Collapse() { const [, setOpen] = useSidebarOpen(); return <button onClick={() => setOpen(false)}>Collapse fixture</button>; }
  renderWithApp(<><AppearanceSection /><DisruptionsLink /><Collapse /></>);
  const toggle = await screen.findByRole("switch", { name: "Show Disruptions in the navigation" });
  expect(toggle.getAttribute("aria-checked")).toBe("false");
  await userEvent.click(toggle);
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sidebarStore.get().disruptions).toBe(true));
  expect(screen.getByRole("link", { name: "Disruptions" }).getAttribute("href")).toBe("/disruptions");
  await userEvent.click(screen.getByRole("button", { name: "Collapse fixture" }));
  expect(sidebarStore.get()).toEqual({ open: false, disruptions: true });
});

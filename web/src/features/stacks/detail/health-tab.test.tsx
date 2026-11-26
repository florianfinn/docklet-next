// Source: docklet-next/issues/636
// Revision: 7a44b4b5c32ccc64b68e027a5496fab1d26e90e2
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 stack Health tab and reusable health details.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderStackDetail } from "@/data/message-code";
import type { ContainerHealthDetails } from "@/domain/health";
import type { Service, StackDetail } from "@/domain/model";
import type { HistoryRange, RuntimeHistory } from "@/domain/history";
import { renderRoutes } from "@/test/render";
import { HealthTab } from "./HealthTab";

const check = { test: ["CMD-SHELL", "curl http://example.com/health"], intervalMs: 30_000, timeoutMs: 5000, retries: 3, startPeriodMs: 0, startIntervalMs: 0 };
const startedAt = "2026-11-01T10:00:00Z";
const raw = '<b>plain output</b> curl: (22) The requested URL returned error: 404';
const details = (id: string): ContainerHealthDetails => ({ containerId: id, running: true, status: "unhealthy", startedAt, check, failingStreak: 2, probes: [{ start: "2026-11-01T10:01:00Z", exitCode: 1, output: raw }] });
const emptyHistory = (range: HistoryRange): RuntimeHistory => ({ range, buckets: [], uptimePercent: null, totals: { netRx: 0, netTx: 0, diskRead: 0, diskWrite: 0 } });

beforeEach(() => setAppLocale("en-US"));
afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); });

async function setup(names: string[], overrides: Record<string, Partial<Service>> = {}, read = async (id: string) => details(id), search = "") {
  const demo = createDemoAdapter();
  const original = renderStackDetail(await demo.getStack("media"), "en-US");
  const services = names.map((name) => ({ ...original.services[0]!, name, containerId: name, containerName: `stack-${name}`, ...overrides[name] }));
  const health = vi.fn(read);
  const history = vi.fn(async (_ref: unknown, range: HistoryRange) => emptyHistory(range));
  const adapter = { ...demo, getContainerHealth: health, getContainerHistory: history };
  const view = renderRoutes([{ path: "/", element: <HealthTab stack={{ ...original, services }} /> }], [`/${search}`], adapter);
  return { ...view, health, history };
}

it("renders all service reasons in service order, with no hint for healthy", async () => {
  const names = ["http", "tool", "pending", "none", "healthy"];
  const { container } = await setup(names, {}, async (id) => {
    const d = details(id);
    if (id === "tool") d.probes[0]!.output = 'exec: "curl": executable file not found';
    if (id === "pending") { d.probes = []; d.status = "starting"; d.startedAt = new Date().toISOString(); }
    if (id === "none") d.check = undefined;
    if (id === "healthy") d.status = "healthy";
    return d;
  });
  await screen.findByText(/checked path does not exist/);
  expect(screen.getByText('The probe tool "curl" is missing in the image.').textContent).toContain("curl");
  expect(screen.getByText("Waiting for the first probe.").textContent).toBeTruthy();
  expect(screen.getByText("No healthcheck defined.").textContent).toBeTruthy();
  expect(screen.getByRole("region", { name: "healthy" }).querySelector("[data-health-hint]") === null).toBe(true);
  expect([...container.querySelectorAll("section")].map((r) => r.getAttribute("aria-label"))).toEqual(names);
});

it("toggles raw data by keyboard and renders probe output as plain text", async () => {
  const user = userEvent.setup();
  const { container } = await setup(["http"], {}, async (id) => {
    const d = details(id);
    d.probes = [{ start: "2026-11-01T10:00:30Z", exitCode: 1, output: "older probe" }, ...d.probes];
    return d;
  });
  await screen.findByText(/checked path does not exist/);
  const toggle = screen.getByRole("button", { name: "Raw health data" });
  expect(screen.queryByText(check.test[1]!) === null).toBe(true);
  toggle.focus();
  await user.keyboard("{Enter}");
  expect(screen.getByText(check.test[1]!).textContent).toBe(check.test[1]);
  expect([...container.querySelectorAll("pre")].map((pre) => pre.textContent)).toEqual([raw, "older probe"]);
  expect(container.querySelector("pre b") === null).toBe(true);
  expect(screen.getByText("default").textContent).toBe("default");
  await user.keyboard("{Enter}");
  expect(container.querySelector("pre") === null).toBe(true);
});

it("changes every row history request with the shared range control", async () => {
  const user = userEvent.setup();
  const { history } = await setup(["a", "b"]);
  await waitFor(() => expect(history.mock.calls.length).toBe(2));
  for (const [label, range] of [["7 days", "7d"], ["30 days", "30d"]]) {
    await user.click(screen.getByRole("radio", { name: label }));
    await waitFor(() => expect(history.mock.calls.filter(([, r]) => r === range).length).toBe(2));
  }
});

it("guards missing identities from health and history requests", async () => {
  const { health, history } = await setup(["missing", "nameless", "live"], {
    missing: { containerId: undefined }, nameless: { containerName: undefined },
  });
  await screen.findByText("No container");
  await waitFor(() => expect(health.mock.calls.length).toBe(2));
  expect(health.mock.calls.map(([id]) => id).sort()).toEqual(["live", "nameless"]);
  expect(history.mock.calls).toEqual([[{ hostId: expect.any(String), name: "stack-live" }, "24h"]]);
});

it("isolates a rejected health read to its row while neighbours finish", async () => {
  let rejectRead!: (error: Error) => void;
  const pending = new Promise<ContainerHealthDetails>((_, reject) => { rejectRead = reject; });
  await setup(["broken", "working"], {}, (id) => id === "broken" ? pending : Promise.resolve(details(id)));
  rejectRead(new Error("read unavailable"));
  await within(screen.getByRole("region", { name: "broken" })).findByText("Health details unavailable");
  expect(within(screen.getByRole("region", { name: "working" })).queryByRole("alert") === null).toBe(true);
  expect(within(screen.getByRole("region", { name: "working" })).getByText(/checked path does not exist/).textContent).toBeTruthy();
});

it("focuses and scrolls to the deep-linked service while keeping every row", async () => {
  const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView");
  await setup(["a", "b"], {}, async (id) => details(id), "?tab=health&service=b");
  expect(document.activeElement === screen.getByRole("region", { name: "b" })).toBe(true);
  expect(screen.getAllByRole("region").length).toBe(2);
  expect(scroll).toHaveBeenCalledWith({ block: "center" });
});

it("follows a same-name recreate: health reads the new id, history keeps its name key", async () => {
  const user = userEvent.setup();
  const demo = createDemoAdapter();
  const original = renderStackDetail(await demo.getStack("media"), "en-US");
  const service = { ...original.services[0]!, name: "web", containerId: "old-id", containerName: "stack-web" };
  const health = vi.fn(async (id: string) => {
    const d = details(id);
    if (id === "new-id") d.probes[0]!.output = 'exec: "curl": executable file not found';
    return d;
  });
  const history = vi.fn(async (_ref: unknown, range: HistoryRange) => emptyHistory(range));
  function Recreating() {
    const [stack, setStack] = useState<StackDetail>({ ...original, services: [service] });
    return <><button type="button" onClick={() => setStack({ ...stack, services: [{ ...service, containerId: "new-id" }] })}>recreate</button><HealthTab stack={stack} /></>;
  }
  renderRoutes([{ path: "/", element: <Recreating /> }], ["/"], { ...demo, getContainerHealth: health, getContainerHistory: history });
  await screen.findByText(/checked path does not exist/);
  await user.click(screen.getByRole("button", { name: "recreate" }));
  await waitFor(() => expect(health.mock.calls.map(([id]) => id)).toEqual(["old-id", "new-id"]));
  await screen.findByText('The probe tool "curl" is missing in the image.');
  expect(history.mock.calls.map(([ref]) => ref)).toEqual([{ hostId: original.hostId, name: "stack-web" }]);
});

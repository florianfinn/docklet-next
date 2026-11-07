// Source: docklet-next/issues/383
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original real HTTP terminal-to-detail ownership regressions.

import { deferred } from "./activity-reader.test-support";

import { act, screen, waitFor } from "@testing-library/react";
import { useParams } from "react-router";
import { expect, it } from "vitest";
import { useStack } from "@/data/queries";
import { renderRoutes } from "@/test/render";
import { hardeningFixture } from "./web-hardening.test-support";
import { AppShell } from "@/app/shell/AppShell";
import StackPage from "@/features/stacks/detail/StackPage";
import { setAppLocale } from "@/app/i18n/locale-store";

function Probe() {
  const { stackId } = useParams(); const stack = useStack(stackId);
  return <h1>{stack.data ? `${stack.data.id}:${stack.data.services.map((s) => s.state).join(",")}` : "loading"}</h1>;
}
// The hub-local producer publishes {type:"activity", activityId, activity} without an envelope host
// (activity/service_projection.go publishActivityInternal); the activity names its environment.
const sendProducerTerminal = (f: ReturnType<typeof hardeningFixture>) => f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: f.state.run.endedAt, activity: { type: "activity", activityId: f.state.run.id, activity: { ...f.state.run, resourceType: "stack" } } }), lastEventId: "" } as MessageEvent);
const detailReads = (f: ReturnType<typeof hardeningFixture>, host: string, project = "project") => f.fetcher.mock.calls.filter(([input]) => String(input).split("?")[0]?.endsWith(`/environments/${host}/projects/${project}`)).length;

it.each([false, true])("WH-B3 hub-local terminal refreshes header and services without reload, unchanged=%s", async (unchanged) => {
  setAppLocale("en-US"); const f = hardeningFixture(); f.inventory.state.hostIds = ["0"];
  const view = renderRoutes([{ element: <AppShell />, children: [{ path: "/stacks/:stackId", element: <StackPage /> }] }], ["/stacks/0~project"], f.adapter);
  try {
    await screen.findByRole("heading", { name: "app", level: 1 });
    await waitFor(() => expect(document.body.textContent?.includes("web")).toBe(true));
    await act(async () => { await f.adapter.runStackCommand("0~project", "down"); });
    const before = detailReads(f, "0"); f.state.stopped = !unchanged;
    f.state.run = { ...f.state.run, status: "success", endedAt: "2026-11-07T00:00:01Z" };
    act(() => { sendProducerTerminal(f); });
    await waitFor(() => expect(detailReads(f, "0")).toBeGreaterThan(before));
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "app", level: 1 }) !== null).toBe(true);
      const table = screen.getByRole("table");
      expect(table.textContent?.includes(unchanged ? "Running" : "Stopped")).toBe(true);
    });
  } finally { view.unmount(); setAppLocale("de"); }
});

it.each(["navigation", "stack", "host", "identity"] as const)("WH-B3 delayed terminal after %s cannot refresh or overwrite the wrong page", async (change) => {
  const f = hardeningFixture(); f.inventory.state.hostIds = ["0", "first"];
  if (change === "stack") {
    const original = f.fetcher.getMockImplementation()!;
    f.fetcher.mockImplementation(async (input, init) => {
      const path = String(input); const other = path.includes("/projects/other");
      const result = await original(other ? path.replace("/projects/other", "/projects/project") : input, init);
      if (!result.ok || !other && !/\/projects\?/.test(path)) return result;
      const body = await result.json();
      if (other) body.data = { ...body.data, id: "other", name: "other" };
      else body.data.push({ ...body.data[0], id: "other", name: "other", runningCount: 0, serviceCount: 0 });
      return Response.json(body);
    });
  }
  const view = renderRoutes([{ element: <AppShell />, children: [{ path: "/stacks/:stackId", element: <Probe /> }, { path: "/hosts", element: <h1>Hosts page</h1> }] }], ["/stacks/0~project"], f.adapter);
  try {
    await screen.findByRole("heading", { name: "0~project:running,stopped" });
    await act(async () => { await f.adapter.runStackCommand("0~project", "down"); });
    f.state.holdDetail = true; f.state.run = { ...f.state.run, status: "success", endedAt: "2026-11-07T00:00:01Z" };
    act(() => f.source.send(f.state.run.id, "0"));
    await f.state.detailStarted.promise;
    if (change === "identity") await act(async () => { await f.adapter.logout(); });
    else await act(async () => { await view.router.navigate(change === "host" ? "/stacks/first~project" : change === "stack" ? "/stacks/0~other" : "/hosts"); });
    if (change === "host") await screen.findByRole("heading", { name: "first~project:running,stopped" });
    if (change === "stack") await screen.findByRole("heading", { name: "0~other:" });
    const before = detailReads(f, change === "host" ? "first" : "0", change === "stack" ? "other" : "project");
    const terminalObserved = new Promise<void>((resolve) => { if (change === "identity") resolve(); else { const off = f.adapter.subscribeStackTerminals!(() => { off(); resolve(); }); } });
    await act(async () => { f.state.detailGate.resolve(); await terminalObserved; });
    expect(detailReads(f, change === "host" ? "first" : "0", change === "stack" ? "other" : "project")).toBe(before);
    if (change === "navigation") expect(screen.getByRole("heading").textContent).toBe("Hosts page");
    if (change === "host") expect(screen.getByRole("heading").textContent).toBe("first~project:running,stopped");
    if (change === "stack") expect(screen.getByRole("heading").textContent).toBe("0~other:");
  } finally { f.state.detailGate.resolve(); view.unmount(); }
});

it("WH-B3 first terminal hub-local receipt refreshes without observing an active transition", async () => {
  const f = hardeningFixture(); f.inventory.state.hostIds = ["0"];
  f.state.run = { ...f.state.run, resourceType: "stack", status: "success", endedAt: "2026-11-07T00:00:01Z" };
  const view = renderRoutes([{ element: <AppShell />, children: [{ path: "/stacks/:stackId", element: <Probe /> }] }], ["/stacks/0~project"], f.adapter);
  try {
    await screen.findByRole("heading", { name: "0~project:running,stopped" });
    const before = detailReads(f, "0"); f.state.stopped = true;
    f.state.run = { ...f.state.run, resourceType: "stack", status: "success", endedAt: "2026-11-07T00:00:01Z" };
    await act(async () => { await f.adapter.runStackCommand("0~project", "down"); });
    expect(detailReads(f, "0")).toBeGreaterThan(before);
    await screen.findByRole("heading", { name: "0~project:stopped,stopped" });
    expect(f.calls.some((call) => call.includes(`/activities/${f.state.run.id}`))).toBe(false);
  } finally { view.unmount(); }
});

it("WH-B3 inline wire success and mapped terminal detail invalidate the target only once", async () => {
  const f = hardeningFixture(); await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const targets: { stackId: string; hostId: string }[] = [];
  const off = f.adapter.subscribeStackTerminals!((target) => targets.push(target));
  const active = deferred<void>(), ended = deferred<void>();
  const detail = f.adapter.observeActionDetail!("0", f.state.run.id, (state) => {
    if (state.status !== "ready") return;
    if (state.run.status === "running") active.resolve();
    if (state.run.status === "succeeded") ended.resolve();
  });
  try {
    await active.promise; f.state.run = { ...f.state.run, status: "success", endedAt: "2026-11-07T00:00:01Z" };
    f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: f.state.run.endedAt, activity: { type: "activity", environmentId: "0", activity: f.state.run } }), lastEventId: "" } as MessageEvent);
    await ended.promise;
    expect(targets).toEqual([{ stackId: "0~project", hostId: "0" }]);
  } finally { off(); detail(); }
});

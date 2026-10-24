// Source: docklet-next/issues/100
// Revision: 7c7f33079a07692cfb6dbe0506205c3e6a4bfe74
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original stream notification and recovery read freshness regressions; notification-scoped event history and bounded activity refresh freshness (#211).

import { expect, it, vi } from "vitest";
import { createHttpAdapter } from "./http-adapter";

const at = "2026-10-15T10:00:00Z";
const json = (data: unknown) => new Response(JSON.stringify({ success: true, data, pagination: { totalItems: Array.isArray(data) ? data.length : 0 } }), { headers: { "content-type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
class FakeSource {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  close = vi.fn();
  open() { this.readyState = 1; this.onopen?.({} as Event); }
  send(value: unknown, id = "") { this.raw(JSON.stringify(value), id); }
  raw(data: string, id = "") { this.onmessage?.({ data, lastEventId: id } as MessageEvent); }
  failClosed() { this.readyState = 2; this.onerror?.({} as Event); }
}
const notification = (channel: string, extra = {}) => ({ channel, timestamp: at, ...extra });
const cases = [
  ["events", (source: FakeSource) => source.send(notification("events", { eventLog: { type: "created" } }))],
  ["activity ID", (source: FakeSource) => source.send(notification("activities", { activity: { type: "updated", activityId: "run", environmentId: "0" } }))],
  ["activity without ID", (source: FakeSource) => source.send(notification("activities", { activity: { type: "updated" } }))],
  ["environments", (source: FakeSource) => source.send(notification("environments"))],
  ["malformed JSON", (source: FakeSource) => source.raw("{")],
  ["invalid envelope", (source: FakeSource) => source.send({})],
  ["unknown channel", (source: FakeSource) => source.send(notification("unknown"))],
  // A heartbeat with a gap exercises recovery independently of channel invalidation.
  ["sequence gap", (source: FakeSource) => source.send({ type: "heartbeat", timestamp: at }, "3")],
  ["reopened source", (source: FakeSource) => source.open()],
  ["CLOSED reconnect", async (source: FakeSource, sources: FakeSource[]) => {
    vi.useFakeTimers();
    source.failClosed();
    await vi.advanceTimersByTimeAsync(350);
    expect(sources).toHaveLength(2);
    sources[1]!.open();
    vi.useRealTimers();
  }],
] as const;

function fixture(kind: "events" | "activities") {
  let status = "running";
  let hold = false;
  const held: Array<() => void> = [];
  let reads = 0;
  const sources: FakeSource[] = [];
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    const path = new URL(String(input), "https://example.test").pathname;
    if (path === "/api/capabilities") return json(["listEvents", "listActions", "subscribeChanges", "subscribeActions"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } })));
    if (path === "/api/environments") return json([{ id: "0" }]);
    const activity = { id: "run", environmentId: "0", type: "project_redeploy", status, startedAt: at, createdAt: at, resourceType: "project", resourceId: "example", resourceName: "Example", messageCode: "activity.running" };
    if (path.endsWith("/activities")) return json([activity]);
    const event = { id: status, type: "project.deploy", messageCode: "project.deploy", severity: "success", title: "Example", timestamp: at, createdAt: at, environmentId: "0", resourceType: "project", resourceId: "example" };
    if (path === "/api/events" || path.endsWith("/activities/run")) {
      const selected = kind === "events" ? path === "/api/events" : path.endsWith("/activities/run");
      const response = json(path === "/api/events" ? [event] : { activity, messages: [], stepTrace: [], output: [], outputTruncated: false });
      if (selected) {
        reads++;
        if (hold) await new Promise<void>((resolve) => held.push(resolve));
      }
      return response;
    }
    throw new Error("Unexpected fixture route");
  });
  const http = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => {
    const source = new FakeSource(); sources.push(source); return source as unknown as EventSource;
  } });
  const changes = vi.fn();
  const runs = vi.fn();
  const stops = [http.subscribeChanges(changes), http.subscribeActions(runs)];
  const read = async () => kind === "events" ? (await http.listEvents())[0]!.id : (await http.listActions())[0]!.status;
  const start = async () => {
    sources[0]!.open();
    await vi.waitFor(() => expect(runs).toHaveBeenCalled());
    await read();
    sources[0]!.send({ type: "heartbeat", timestamp: at }, "1");
    changes.mockClear(); runs.mockClear(); reads = 0; hold = true;
  };
  return { sources, held, changes, runs, read, start, reads: () => reads,
    update: () => { status = "queued"; hold = false; },
    cleanup: () => { stops.forEach((stop) => stop()); held.forEach((release) => release()); vi.useRealTimers(); },
  };
}

for (const kind of ["events", "activities"] as const) {
  it.each(cases)(`${kind}: %s separates post-notification reads from held pre-notification snapshots`, async (name, notify) => {
    const f = fixture(kind);
    const pending: Promise<unknown>[] = [];
    try {
      await f.start();
      const older = f.read(); pending.push(older);
      await vi.waitFor(() => expect(f.held).toHaveLength(1));
      f.update();
      await notify(f.sources[0]!, f.sources);
      const newer = f.read(); pending.push(newer);
      f.held[0]!();
      const recovery = !["events", "activity ID", "activity without ID", "environments"].includes(name);
      const historyChange = kind === "events" && (name === "events" || recovery);
      const discovery = kind === "activities" && (recovery || name === "activity without ID" || name === "environments");
      expect(await newer).toBe(historyChange || discovery ? "queued" : "running");
      expect(await older).toBe(historyChange ? "queued" : "running");
    } finally {
      f.cleanup();
      await Promise.allSettled(pending);
    }
  });

  it(`${kind}: contiguous heartbeats retain concurrent sharing and do not refresh subscribers`, async () => {
    const f = fixture(kind);
    const pending: Promise<unknown>[] = [];
    try {
      await f.start();
      const older = f.read(); pending.push(older);
      await vi.waitFor(() => expect(f.held).toHaveLength(1));
      f.update();
      f.sources[0]!.send({ type: "heartbeat", timestamp: at }, "2");
      const newer = f.read(); pending.push(newer);
      vi.useFakeTimers();
      await vi.advanceTimersByTimeAsync(0);
      expect(f.reads()).toBe(1);
      expect(f.changes).not.toHaveBeenCalled();
      expect(f.runs).not.toHaveBeenCalled();
      f.held[0]!();
      expect(await older).toBe("running");
      expect(await newer).toBe("running");
    } finally {
      f.cleanup();
      await Promise.allSettled(pending);
    }
  });
}

// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original web contract matrix for issue 38.
// Cases 5–7 and 10–12 exercise HTTP transport, SSE, and recorded DTOs only.

import { act, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "@/api/generated/schema";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { SEED_RUNS } from "@/data/demo/fixtures/events";
import { simulationStore } from "@/data/demo/simulation";
import { isRunBusy, withRunGate } from "@/data/run-gate";
import { pruneRuns } from "@/domain/activity-history";
import { renderWithApp } from "@/test/render";
import { RunFocusView } from "@/features/activity/RunFocus";
import { RunOutput } from "@/features/activity/RunOutput";
import { deActivity } from "@/features/activity/messages/de";
import { enActivity } from "@/features/activity/messages/en";
import { TOAST_LIFETIME } from "@/features/activity/run-phase";
import { ActivitySection } from "@/features/settings/sections/SimpleSections";

import { renderRun, type CodedActionRun } from "../message-code";
import type { CapabilityDescriptor } from "../capability";
import { createHttpAdapter } from "./http-adapter";
import { inventoryId } from "./inventory-mapping";
import { mapActivityDetail, mapEvent } from "./map-dto";
import type { HubBackupRun } from "@/domain/hub-backup";

type Activity = components["schemas"]["ActivityActivity"];
type Detail = components["schemas"]["ActivityDetail"];
type Event = components["schemas"]["EventEvent"];
const at = "2026-10-15T10:00:00Z";
const later = "2026-10-15T10:01:00Z";

const activity: Activity = {
  id: "run-dns-1", environmentId: "0", type: "project_redeploy", status: "success",
  startedAt: at, createdAt: at, endedAt: later, progress: 100,
  resourceType: "project", resourceId: "dns", resourceName: "DNS · Delta",
  affectedServices: ["adguard", "unbound"], messageCode: "activity.success",
};
const detail: Detail = {
  activity,
  messages: [{ id: "m1", activityId: activity.id, createdAt: later, level: "success", message: "Abgeschlossen", code: "activity.success", step: "start" }],
  stepTrace: [{ id: "s1", name: "start", status: "success", code: "activity.success", startedAt: at, endedAt: later }],
  output: [{ timestamp: at, stream: "stdout", text: "ready", step: "start" }], outputTruncated: false,
};
const event: Event = {
  id: "e10", type: "project.deploy", messageCode: "project.deploy", severity: "success",
  title: "Redeploy DNS abgeschlossen", timestamp: later, createdAt: later,
  environmentId: "0", resourceType: "project", resourceId: "dns",
};

const pagination = (count: number) => ({ currentPage: 1, itemsPerPage: 500, totalItems: count, totalPages: 1 });
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json", "X-CSRF-Token": "example".padEnd(43, "x") } });
const ok = (data: unknown) => json({ success: true, data });
const page = (data: unknown[]) => json({ success: true, data, pagination: pagination(data.length) });

function backend() {
  const state = { detail: structuredClone(detail), extraDetails: {} as Record<string, Detail>, events: [event], list: [activity], descriptors: [
    ...["listActions", "listEvents", "subscribeActions", "subscribeChanges"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } })),
  ] as CapabilityDescriptor[] };
  const requests: string[] = [];
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    const url = new URL(String(input), "https://example.test");
    requests.push(url.pathname);
    if (url.pathname === "/api/capabilities") return ok(state.descriptors);
    if (url.pathname === "/api/environments") return page([{ id: "0" }]);
    if (url.pathname === "/api/environments/0/activities") return page(state.list);
    if (url.pathname === "/api/environments/0/activities/run-dns-1") return ok(state.detail);
    if (url.pathname.startsWith("/api/environments/0/activities/")) {
      const found = state.extraDetails[url.pathname.split("/").at(-1)!];
      if (found) return ok(found);
    }
    if (url.pathname === "/api/events") return page(state.events);
    return json({ code: "not-found" }, 404);
  });
  const http = createHttpAdapter({ fetch: fetcher as typeof fetch });
  return { http, state, fetcher, requests };
}

class FakeSource {
  readyState = 0;
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  close = vi.fn();
  open() { this.readyState = 1; this.onopen?.({} as Event); }
  failClosed() { this.readyState = 2; this.onerror?.({} as Event); }
  send(value: unknown, id = "") { this.onmessage?.({ data: JSON.stringify(value), lastEventId: id } as MessageEvent); }
}

function heldRunBackend() {
  const b = backend();
  const held: Array<() => void> = [];
  let hold = false;
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    if (hold && new URL(String(input), "https://example.test").pathname.endsWith(`/activities/${activity.id}`)) {
      const response = ok(structuredClone(b.state.detail));
      await new Promise<void>((resolve) => { held.push(resolve); });
      return response;
    }
    return b.fetcher(input);
  });
  const source = new FakeSource();
  const http = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source as unknown as EventSource });
  const runs = vi.fn<(runs: CodedActionRun[]) => void>();
  const stop = http.subscribeActions(runs);
  const start = async () => {
    source.open();
    await waitFor(() => expect(runs.mock.lastCall?.[0][0]?.status).toBe("succeeded"));
    hold = true;
  };
  const read = async (kind: "S" | "F", status: "queued" | "running", count: number) => {
    b.state.detail.activity.status = status;
    if (kind === "S") source.send({ channel: "activities", activity: { type: "updated", activityId: activity.id, environmentId: "0" }, timestamp: later });
    else source.open();
    await waitFor(() => expect(held).toHaveLength(count));
  };
  return { state: b.state, source, held, runs, stop, start, read };
}

afterEach(() => { vi.restoreAllMocks(); simulationStore.reset(); localStorage.clear(); });

function RecordedOutputList({ initial, onChange }: { initial: CodedActionRun[]; onChange: (update: (runs: CodedActionRun[]) => void) => void }) {
  const [runs, setRuns] = useState(initial);
  onChange(setRuns);
  return <>{runs.map((run) => <RunOutput key={run.id} run={renderRun(run, "de")} defaultOpen />)}</>;
}

describe.each(["demo", "http"] as const)("%s shared adapter cases", (kind) => {
  it("2 disables an unsupported action and refuses a direct call without starting a run", async () => {
    const b = backend();
    const adapter = kind === "demo" ? createDemoAdapter() : b.http;
    const id = kind === "demo" ? "missing" : "dns";
    const ref = { kind: "stack" as const, id };
    if (kind === "http") b.state.descriptors.push({ operationId: "runStackCommand", resourceRef: ref, available: false, disabledReasonCode: "capability.mapping-unavailable" });
    const descriptor = await adapter.getCapability("runStackCommand", ref);
    expect(descriptor.available).toBe(false);
    expect(descriptor.disabledReasonCode).toBe(kind === "http" ? "capability.mapping-unavailable" : "capability.unavailable");
    const beforeRequests = b.fetcher.mock.calls.length;
    const beforeRuns = kind === "demo" ? (await adapter.listActions()).length : 0;
    await expect(adapter.runStackCommand(id, "start")).rejects.toMatchObject({ outcome: { kind: "not-available", operationId: "runStackCommand" } });
    expect(b.fetcher.mock.calls.length).toBe(beforeRequests);
    if (kind === "demo") expect((await adapter.listActions()).length).toBe(beforeRuns);
  });

  it("3 keeps a missing descriptor unavailable through the real adapter lookup", async () => {
    const b = backend();
    b.state.descriptors = [];
    const adapter = kind === "demo" ? createDemoAdapter({ missingCapabilities: ["runStackCommand"] }) : b.http;
    const ref = { kind: "stack" as const, id: "dns" };
    const descriptor = await adapter.getCapability("runStackCommand", ref);
    expect([descriptor.available, descriptor.disabledReasonCode]).toEqual([false, "capability.unavailable"]);
    await expect(adapter.runStackCommand("dns", "start")).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    if (kind === "http") {
      expect(b.fetcher.mock.calls.every((call) => String(call[0]).includes("capabilities"))).toBe(true);
      b.fetcher.mockRejectedValueOnce(new Error("offline"));
      expect((await adapter.getCapability("listEvents")).available).toBe(false);
    }
  });

  it("4 preserves a busy run ID and refuses another start", async () => {
    if (kind === "demo") {
      simulationStore.set((state) => ({ ...state, latency: "instant", actionSpeed: 20 }));
      const adapter = withRunGate(createDemoAdapter());
      const activeRunId = await adapter.runStackCommand("media", "redeploy");
      const before = (await adapter.listActions()).length;
      const error = await adapter.runStackCommand("media", "stop").catch((value: unknown) => value);
      expect(isRunBusy(error)).toBe(true);
      if (isRunBusy(error)) expect(error.runId).toBe(activeRunId);
      expect((await adapter.listActions()).length).toBe(before);
      return;
    }
    for (const code of ["run.busy", "run.cancelling"] as const) {
      const b = backend();
      b.fetcher.mockImplementation(async (input) => String(input).includes("/events") ? json({ code, activeRunId: "active-1" }, 409) : backendResponse(String(input)));
      await expect(b.http.listEvents()).rejects.toMatchObject({ outcome: { kind: "conflict", code, activeRunId: "active-1" } });
      expect(b.fetcher.mock.calls.some((call) => String(call[0]).includes("/activities"))).toBe(false);
    }
  });

  it.each([["queued", "Wartet", "Queued"], ["cancelled", "Abgebrochen", "Cancelled"]] as const)("%s has distinct labels and terminal behavior (cases 13/14)", (status, de, en) => {
    const b = backend();
    b.state.detail.activity.status = status;
    const seed = { ...structuredClone(SEED_RUNS[1]!), status, finishedAt: status === "queued" ? undefined : later };
    const adapter = kind === "demo" ? createDemoAdapter({ seedRuns: [seed] }) : b.http;
    return adapter.listActions().then((runs) => {
      const coded = runs[0]!;
      expect(coded.status).toBe(status);
      const rendered = renderRun(coded, "de");
      expect(rendered.status).toBe(status);
      expect([deActivity.status[status], enActivity.status[status]]).toEqual([de, en]);
      if (status === "queued") expect(pruneRuns([rendered], 0)).toHaveLength(1);
      else expect([TOAST_LIFETIME.cancelled, pruneRuns([rendered], 0).length]).toEqual([8000, 0]);
      renderWithApp(<RunFocusView run={rendered} now={Date.parse(later)} />, adapter);
      expect(screen.getAllByText(de).length).toBeGreaterThan(0);
    });
  });
});

describe("#38 adapter contract matrix", () => {
  it("1 maps recorded run and event fields alongside coded demo data", async () => {
    const { http } = backend();
    const demo = createDemoAdapter();
    const demoRun = (await demo.listActions()).find((run) => run.id === activity.id)!;
    const wireRun = (await http.listActions())[0]!;
    expect([wireRun.id, wireRun.kind, wireRun.status, wireRun.stackId, wireRun.services]).toEqual([demoRun.id, demoRun.kind, demoRun.status, demoRun.stackId ? inventoryId(activity.environmentId, demoRun.stackId) : undefined, demoRun.services]);
    expect(wireRun.steps.map((step) => [step.key, step.state, step.label.code])).toEqual([["start", "done", "activity.success"]]);
    expect(wireRun.log.map((entry) => entry.code)).toEqual(["activity.success"]);
    expect(demoRun.steps.length > 0 && demoRun.log.every((entry) => !!entry.code)).toBe(true);
    const demoEvent = (await demo.listEvents()).find((item) => item.id === event.id)!;
    const wireEvent = (await http.listEvents())[0]!;
    expect([wireEvent.id, wireEvent.kind, wireEvent.level, wireEvent.stackId]).toEqual([demoEvent.id, demoEvent.kind, demoEvent.level, demoEvent.stackId ? inventoryId(event.environmentId ?? "0", demoEvent.stackId) : undefined]);
    expect(wireEvent.title.code).toBe("project.deploy");
  });

  it("5 rejects transport and DTO mapping errors without an empty fallback", async () => {
    const { http, state, fetcher } = backend();
    fetcher.mockRejectedValueOnce(new Error("offline"));
    expect((await http.getCapability("listEvents")).available).toBe(false);
    state.events = [{ ...event, severity: "invalid" }];
    await expect(http.listEvents()).rejects.toMatchObject({ outcome: { kind: "error", code: "listEvents.mapping-error" } });
    expect(() => mapEvent({ ...event, severity: "invalid" })).toThrow();
  });

  it("6 re-reads stale run envelopes and invalidates events", async () => {
    const b = backend();
    const source = new FakeSource();
    const http = createHttpAdapter({ fetch: b.fetcher as typeof fetch, eventSource: () => source as unknown as EventSource });
    const runs = vi.fn(); const changes = vi.fn();
    const stopRuns = http.subscribeActions(runs); const stopChanges = http.subscribeChanges(changes);
    source.open();
    await waitFor(() => expect(runs).toHaveBeenCalled());
    for (const status of ["running", "failed", "success"] as const) {
      b.state.detail.activity.status = status;
      source.send({ channel: "activities", activity: { type: "updated", activityId: activity.id, activity: { ...activity, status: "queued" } }, timestamp: later });
      await waitFor(() => expect(runs.mock.lastCall?.[0][0].status).toBe(status === "success" ? "succeeded" : status));
    }
    source.send({ channel: "activities", activity: { type: "message", activityId: activity.id, message: detail.messages?.[0] }, timestamp: later });
    await waitFor(() => expect(b.requests.filter((path) => path.endsWith("/run-dns-1")).length).toBeGreaterThan(3));
    changes.mockClear();
    source.send({ channel: "events", eventLog: { type: "changed", timestamp: later }, timestamp: later });
    expect(changes.mock.calls).toEqual([[ ["events"] ]]);
    stopRuns(); stopChanges();
  });

  it("7 recovers a committed completion on reconnect and event-id loss", async () => {
    const b = backend(); const source = new FakeSource();
    const http = createHttpAdapter({ fetch: b.fetcher as typeof fetch, eventSource: () => source as unknown as EventSource });
    const runs = vi.fn(); const changes = vi.fn();
    const stopRuns = http.subscribeActions(runs); const stopChanges = http.subscribeChanges(changes);
    b.state.detail.activity.status = "running";
    source.open();
    await waitFor(() => expect(runs.mock.lastCall?.[0][0].status).toBe("running"));
    b.state.detail.activity.status = "success";
    source.open();
    await waitFor(() => expect(runs.mock.lastCall?.[0][0].status).toBe("succeeded"));
    const reads = b.requests.filter((path) => path.endsWith("/run-dns-1")).length;
    source.send({ channel: "events", eventLog: { type: "changed", timestamp: later }, timestamp: later }, "1");
    source.send({ channel: "events", eventLog: { type: "changed", timestamp: later }, timestamp: later }, "3");
    await waitFor(() => expect(b.requests.filter((path) => path.endsWith("/run-dns-1")).length).toBeGreaterThan(reads));
    expect(changes).toHaveBeenCalledWith(["events"]);
    stopRuns(); stopChanges();
  });

  it("7 keeps a reconnect full read when another run receives an activity event", async () => {
    const b = backend();
    b.state.detail.activity.status = "running";
    b.state.list[0] = { ...activity, status: "running" };
    const second: Activity = { ...activity, id: "run-other-2", status: "running", resourceName: "Other" };
    const secondDetail: Detail = { ...structuredClone(detail), activity: second };
    b.state.list.push(second);
    let releaseSecond: (() => void) | undefined;
    let holdSecond = false;
    const fetcher = vi.fn(async (input: string | URL | Request) => {
      if (String(input).includes("/activities/run-other-2")) {
        if (holdSecond) {
          holdSecond = false;
          await new Promise<void>((resolve) => { releaseSecond = resolve; });
        }
        return ok(secondDetail);
      }
      return b.fetcher(input);
    });
    const source = new FakeSource();
    const http = createHttpAdapter({ fetch: fetcher as typeof fetch, eventSource: () => source as unknown as EventSource });
    const runs = vi.fn();
    const stop = http.subscribeActions(runs);
    source.open();
    await waitFor(() => expect(runs.mock.lastCall?.[0].find((run: { id: string }) => run.id === second.id)?.status).toBe("running"));
    secondDetail.activity = { ...second, status: "success" };
    holdSecond = true;
    source.open();
    await waitFor(() => expect(releaseSecond).toBeTypeOf("function"));
    source.send({ channel: "activities", activity: { type: "updated", activityId: activity.id, environmentId: "0" }, timestamp: later });
    await waitFor(() => expect(fetcher.mock.calls.filter((call) => String(call[0]).includes("/activities/run-dns-1")).length).toBeGreaterThan(1));
    releaseSecond!();
    await waitFor(() => expect(runs.mock.lastCall?.[0].find((run: { id: string }) => run.id === second.id)?.status).toBe("succeeded"));
    stop();
  });

  it.each([
    ["S completes before F starts", ["S", "F"], ["S", "F"]],
    ["F completes before S starts", ["F", "S"], ["F", "S"]],
    ["S starts before F and completes after F", ["S", "F"], ["F", "S"]],
    ["F starts before S and completes after S", ["F", "S"], ["S", "F"]],
  ] as const)("7 applies the last-started read when %s", async (_, starts, finishes) => {
    const b = heldRunBackend();
    await b.start();
    const status = (kind: "S" | "F") => kind === "S" ? "running" as const : "queued" as const;
    const startRead = (kind: "S" | "F", count: number) => b.read(kind, status(kind), count);
    const release = async (kind: "S" | "F", expected: "S" | "F") => {
      b.held[starts.indexOf(kind)]!();
      await waitFor(() => expect(b.runs.mock.lastCall?.[0][0]?.status).toBe(status(expected)));
    };
    await startRead(starts[0], 1);
    if (starts[0] === finishes[0]) await release(finishes[0], finishes[0]);
    await startRead(starts[1], 2);
    if (starts[0] !== finishes[0]) await release(finishes[0], finishes[0]);
    await release(finishes[1], starts[1]);
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(b.runs.mock.lastCall?.[0][0]?.status).toBe(status(starts[1]));
    b.stop();
  });

  it("7 does not resurrect a run removed by a newer full read", async () => {
    const b = heldRunBackend();
    await b.start();
    await b.read("S", "running", 1);
    b.state.list = [];
    b.source.open();
    await waitFor(() => expect(b.runs.mock.lastCall?.[0]).toEqual([]));
    b.held[0]!();
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    expect(b.runs.mock.lastCall?.[0]).toEqual([]);
    b.stop();
  });

  it("7 reconnects a CLOSED source, re-reads once, and stops retries after unsubscribe", async () => {
    const b = backend();
    const sources: FakeSource[] = [];
    const http = createHttpAdapter({ fetch: b.fetcher as typeof fetch, eventSource: () => {
      const source = new FakeSource();
      sources.push(source);
      return source as unknown as EventSource;
    } });
    const runs = vi.fn();
    const stop = http.subscribeActions(runs);
    sources[0]!.open();
    await waitFor(() => expect(runs).toHaveBeenCalledTimes(1));
    expect(b.requests.filter((path) => path.endsWith("/run-dns-1"))).toHaveLength(1);
    b.state.detail.activity.status = "running";
    sources[0]!.failClosed();
    await waitFor(() => expect(sources).toHaveLength(2));
    expect(sources[0]!.close).toHaveBeenCalledTimes(1);
    sources[1]!.open();
    await waitFor(() => expect(runs.mock.lastCall?.[0][0].status).toBe("running"));
    expect(b.requests.filter((path) => path.endsWith("/run-dns-1"))).toHaveLength(2);
    vi.useFakeTimers();
    try {
      sources[1]!.failClosed();
      expect(vi.getTimerCount()).toBe(1);
      stop();
      expect(vi.getTimerCount()).toBe(0);
      await vi.advanceTimersByTimeAsync(350);
      expect(sources).toHaveLength(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it("8 selects HTTP for production and has no simulated output marker", async () => {
    const runtime = await import("../../runtime");
    expect(runtime.createAdapter().kind).toBe("http");
    const b = backend();
    renderWithApp(<RunOutput run={renderRun(mapActivityDetail(b.state.detail), "de")} defaultOpen />, b.http);
    expect(screen.queryByText("Simuliert")).toBeNull();
  });

  it("9 exposes no editable history limits in settings", async () => {
    const demo = createDemoAdapter();
    renderWithApp(<ActivitySection settings={await demo.getSettings()} />, demo);
    expect(screen.queryByLabelText("Höchstens Ereignisse") === null).toBe(true);
    expect(screen.queryByLabelText("Höchstens abgeschlossene Aktionen") === null).toBe(true);
    const file = await import("@/features/settings/messages/de");
    expect(file.deSettings.activity.historyNote).toContain("500 Ereignisse und 100 abgeschlossene Abläufe");
  });

  it("10 maps raw lines after a detail read", async () => {
    const { http } = backend();
    const run = (await http.listActions())[0]!;
    expect(run.output).toEqual([{ at, stream: "stdout", text: "ready", step: "start" }]);
  });

  it("11 shows a truncation marker for the recorded newest output", () => {
    const b = backend();
    b.state.detail.output = [{ timestamp: later, stream: "stderr", text: "newest" }];
    b.state.detail.outputTruncated = true;
    const run = renderRun(mapActivityDetail(b.state.detail), "de");
    expect([run.output[0]?.text, run.outputTruncated]).toEqual(["newest", true]);
    renderWithApp(<RunOutput run={run} defaultOpen />, b.http);
    expect(screen.getByText(/Ältere Ausgaben wurden entfernt/) !== null).toBe(true);
  });

  it("12 drops pruned finished DTOs while retaining active output", async () => {
    const b = backend();
    b.state.detail.output = [{ timestamp: at, stream: "stdout", text: "finished-only" }];
    const activeActivity: Activity = { ...activity, id: "run-active-2", status: "running" };
    b.state.list.push(activeActivity);
    b.state.extraDetails[activeActivity.id] = { ...structuredClone(detail), activity: activeActivity, output: [{ timestamp: at, stream: "stdout", text: "active-only" }] };
    const initial = await b.http.listActions();
    let update!: (runs: CodedActionRun[]) => void;
    renderWithApp(<RecordedOutputList initial={initial} onChange={(setter) => { update = setter; }} />, b.http);
    await waitFor(() => expect([screen.queryByText("finished-only") !== null, screen.queryByText("active-only") !== null]).toEqual([true, true]));
    b.state.list = [activeActivity];
    const refreshed = await b.http.listActions();
    act(() => update(refreshed));
    await waitFor(() => expect(screen.queryByText("finished-only")).toBeNull());
    expect(screen.queryByText("active-only") !== null).toBe(true);
  });

});

function backendResponse(input: string): Response {
  const url = new URL(input, "https://example.test");
  if (url.pathname.endsWith("/capabilities")) return ok(["listActions", "listEvents"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } })));
  return page([]);
}

type BackupScenario = "list" | "empty" | "creating" | "storage-unavailable" | "tools-unavailable" | "maintenance" | "no-access";

function backupBackend(scenario: BackupScenario) {
  const run: HubBackupRun = { id: "system-demo-1", activityId: "activity-demo-1", createdAt: "2026-10-14T09:30:00Z", completedAt: "2026-10-14T09:30:00Z", size: 33_554_432, status: scenario === "creating" ? "running" : "succeeded", trigger: "manual", copyState: "none" };
  const state = {
    runs: scenario === "empty" ? [] as HubBackupRun[] : [run],
    policy: [{ id: "hub-daily", enabled: true, schedule: "0 0 3 * * *", retentionCount: 7 }],
    requests: [] as { path: string; method: string; body?: string }[],
  };
  const fetcher = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = new URL(String(input), "https://example.test");
    const method = init?.method ?? "GET";
    state.requests.push({ path: url.pathname, method, body: init?.body as string | undefined });
    if (url.pathname === "/api/auth/mode") return new Response(JSON.stringify({ mode: "none" }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (url.pathname === "/api/auth/me") return ok({ isGlobalAdmin: scenario !== "no-access", permissionsByEnv: { global: ["system-backups:read", "system-backups:manage", "system-backups:restore"] } });
    if (scenario === "no-access") return json({ detail: "permission.denied" }, 403);
    if (url.pathname === "/api/backups" && method === "GET") return page(state.runs);
    if (url.pathname === "/api/backups" && method === "POST") {
      const created: HubBackupRun = { ...run, id: "system-new-2", activityId: "activity-new-2", status: "running", size: 0 };
      state.runs.unshift(created);
      return json(created, 202);
    }
    if (url.pathname === "/api/backups/policies" && method === "GET") return json({ policies: state.policy });
    if (url.pathname === "/api/backups/policies" && method === "PUT") {
      state.policy = JSON.parse(String(init?.body)).policies;
      return json({ policies: state.policy });
    }
    if (url.pathname === "/api/backups/storage") return json({ available: scenario !== "storage-unavailable" && scenario !== "tools-unavailable", freeBytes: 9_663_676_416, usedBytes: 33_554_432, maxArchiveBytes: 2_147_483_648, ...(scenario === "storage-unavailable" ? { reasonCode: "backup.storage_unavailable" } : scenario === "tools-unavailable" ? { reasonCode: "backup.tools_unavailable" } : {}) });
    if (url.pathname === "/api/backups/maintenance") return json({ active: scenario === "maintenance", ...(scenario === "maintenance" ? { phase: "draining", activityId: "activity-restore-1" } : {}) });
    if (url.pathname.startsWith("/api/environments/0/activities/")) return ok({ activity: { id: "activity-demo-1", status: scenario === "creating" ? "running" : "success" } });
    if (url.pathname === "/api/backups/system-demo-1/restore") return json({ backupId: run.id, activityId: "activity-restore-1", status: "running" }, 202);
    if (url.pathname.startsWith("/api/backups/") && method === "DELETE") {
      const id = decodeURIComponent(url.pathname.slice("/api/backups/".length));
      state.runs = state.runs.filter((entry) => entry.id !== id);
      return json({ backupId: id, activityId: "activity-delete-1", status: "success" });
    }
    return json({ detail: "not-found" }, 404);
  });
  return { adapter: createHttpAdapter({ fetch: fetcher as typeof fetch }), state, fetcher };
}

describe.each(["demo", "http"] as const)("%s Hub backup contract", (kind) => {
  it.each(["list", "empty", "creating", "storage-unavailable", "tools-unavailable", "maintenance", "no-access"] as BackupScenario[])("maps %s without changing the archive contract", async (scenario) => {
    const adapter = kind === "demo" ? createDemoAdapter({ hubBackupScenario: scenario }) : backupBackend(scenario).adapter;
    const access = await adapter.getHubBackupAccess();
    expect(access.read).toBe(scenario !== "no-access");
    const maintenance = await adapter.getHubBackupMaintenance();
    expect(maintenance.active).toBe(scenario === "maintenance");
    if (!access.read) {
      await expect(adapter.listHubBackups()).rejects.toBeTruthy();
      await expect(adapter.deleteHubBackup("system-demo-1")).rejects.toBeTruthy();
      return;
    }
    const runs = await adapter.listHubBackups();
    expect(runs.map((run) => [run.id, run.trigger, run.status, run.size])).toEqual(scenario === "empty" ? [] : [["system-demo-1", "manual", scenario === "creating" ? "running" : "succeeded", 33_554_432]]);
    const storage = await adapter.getHubBackupStorage();
    expect(storage.reasonCode).toBe(scenario === "storage-unavailable" ? "backup.storage_unavailable" : scenario === "tools-unavailable" ? "backup.tools_unavailable" : undefined);
    expect((await adapter.getHubBackupPolicies())[0]?.retentionCount).toBe(7);
    if (scenario === "creating") expect(await adapter.getHubBackupActivityStatus("activity-demo-1")).toBe("running");
  });

  it("creates an archive, saves policy and starts a confirmed restore", async () => {
    const setup = backupBackend("list");
    const adapter = kind === "demo" ? createDemoAdapter() : setup.adapter;
    const created = await adapter.createHubBackup();
    expect(created.status).toBe("running");
    const policy = (await adapter.getHubBackupPolicies())[0]!;
    const saved = await adapter.saveHubBackupPolicies([{ ...policy, retentionCount: 3 }]);
    expect(saved[0]?.retentionCount).toBe(3);
    const restored = await adapter.restoreHubBackup("system-demo-1");
    expect([restored.backupId, restored.status]).toEqual(["system-demo-1", "running"]);
    if (kind === "http") {
      expect(setup.state.requests.some((item) => item.path === "/api/backups" && item.method === "POST")).toBe(true);
      expect(setup.state.requests.some((item) => item.path.endsWith("/restore") && item.body?.includes('"confirmBackupId":"system-demo-1"'))).toBe(true);
      expect(setup.state.requests.some((item) => item.path === "/api/backups/policies" && item.method === "PUT")).toBe(true);
    }
  });

  it("omits an unassigned policy ID on first save", async () => {
    if (kind !== "http") return;
    const setup = backupBackend("list");
    await setup.adapter.saveHubBackupPolicies([{ id: "", enabled: true, schedule: "0 20 4 * * 1", retentionCount: 7 }]);
    const request = setup.state.requests.find((entry) => entry.path === "/api/backups/policies" && entry.method === "PUT");
    expect(JSON.parse(request!.body!).policies[0]).toEqual({ enabled: true, schedule: "0 20 4 * * 1", retentionCount: 7 });
  });

  it("deletes an archive through the manage operation", async () => {
    const setup = backupBackend("list");
    const adapter = kind === "demo" ? createDemoAdapter() : setup.adapter;
    const operation = await adapter.deleteHubBackup("system-demo-1");
    expect([operation.backupId, operation.status]).toEqual(["system-demo-1", "success"]);
    expect((await adapter.listHubBackups()).some((run) => run.id === "system-demo-1")).toBe(false);
    if (kind === "http") expect(setup.state.requests.some((item) => item.path === "/api/backups/system-demo-1" && item.method === "DELETE")).toBe(true);
  });
});

it("uses an octet-stream upload with progress and an ID-based download", async () => {
  const request = { url: "", headers: {} as Record<string, string>, body: null as File | null };
  class FakeUpload {
    upload = { onprogress: null as ((event: ProgressEvent) => void) | null };
    responseText = JSON.stringify({ backupId: "system-upload-1", activityId: "activity-upload-1", status: "success" });
    status = 202;
    withCredentials = false;
    onerror: (() => void) | null = null;
    onload: (() => void) | null = null;
    open(_method: string, url: string) { request.url = url; }
    setRequestHeader(key: string, value: string) { request.headers[key] = value; }
    send(body: File) { request.body = body; this.upload.onprogress?.({ lengthComputable: true, loaded: 5, total: 10 } as ProgressEvent); this.onload?.(); }
  }
  const adapter = createHttpAdapter({ fetch: backupBackend("list").fetcher as typeof fetch, uploadFactory: () => new FakeUpload() as unknown as XMLHttpRequest });
  const progress: number[] = [];
  const file = new File(["PGDMP"], "archive.tar", { type: "application/octet-stream" });
  const result = await adapter.uploadHubBackup(file, (value) => progress.push(value));
  expect([result.backupId, request.url, request.headers["Content-Type"], request.body === file, progress]).toEqual(["system-upload-1", "/api/backups/upload", "application/octet-stream", true, [50, 100]]);
  expect(request.headers["X-CSRF-Token"]).toBe("example".padEnd(43, "x"));
  const clicked = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
  await adapter.downloadHubBackup("system-demo-1");
  expect(clicked).toHaveBeenCalledOnce();
  expect(new URL((clicked.mock.instances[0] as HTMLAnchorElement).href).pathname).toBe("/api/backups/system-demo-1/download");
});

it.each([[413, "backup.size_limit", "<html>Request rejected</html>"], [415, "backup.unsupported_media_type", ""]] as const)("maps upload HTTP %i without a JSON error body", async (status, code, payload) => {
  class FailedUpload {
    upload = { onprogress: null };
    status = status;
    responseText = payload;
    onerror: (() => void) | null = null;
    onload: (() => void) | null = null;
    open() {}
    setRequestHeader() {}
    send() { this.onload?.(); }
  }
  const adapter = createHttpAdapter({ fetch: backupBackend("list").fetcher as typeof fetch, uploadFactory: () => new FailedUpload() as unknown as XMLHttpRequest });
  await expect(adapter.uploadHubBackup(new File(["PGDMP"], "archive.tar"), () => undefined)).rejects.toMatchObject({ outcome: { kind: "error", code, status } });
});

it("recognizes a global administrator wildcard permission", async () => {
  const adapter = createHttpAdapter({ fetch: vi.fn(async () => ok({ isGlobalAdmin: true, permissionsByEnv: { global: ["*"] } })) as typeof fetch });
  expect(await adapter.getHubBackupAccess()).toEqual({ read: true, manage: true, restore: true });
});

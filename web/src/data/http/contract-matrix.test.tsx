// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original web contract matrix for issue 38.

import { screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "@/api/generated/schema";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { pruneRuns } from "@/domain/activity-history";
import { renderWithApp } from "@/test/render";
import { RunFocusView } from "@/features/activity/RunFocus";
import { RunOutput } from "@/features/activity/RunOutput";
import { deActivity } from "@/features/activity/messages/de";
import { enActivity } from "@/features/activity/messages/en";
import { TOAST_LIFETIME } from "@/features/activity/run-phase";
import { ActivitySection } from "@/features/settings/sections/SimpleSections";

import { renderRun } from "../message-code";
import type { CapabilityDescriptor } from "../capability";
import { createHttpAdapter } from "./http-adapter";
import { mapActivityDetail, mapEvent } from "./map-dto";

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
const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });
const ok = (data: unknown) => json({ success: true, data });
const page = (data: unknown[]) => json({ success: true, data, pagination: pagination(data.length) });

function backend() {
  const state = { detail: structuredClone(detail), events: [event], list: [activity], descriptors: [
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
    if (url.pathname === "/api/events") return page(state.events);
    return json({ code: "not-found" }, 404);
  });
  const http = createHttpAdapter({ fetch: fetcher as typeof fetch });
  return { http, state, fetcher, requests };
}

class FakeSource {
  onopen: ((event: Event) => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  close = vi.fn();
  open() { this.onopen?.({} as Event); }
  send(value: unknown, id = "") { this.onmessage?.({ data: JSON.stringify(value), lastEventId: id } as MessageEvent); }
}

afterEach(() => { vi.restoreAllMocks(); localStorage.clear(); });

describe("#38 adapter contract matrix", () => {
  it("1 maps recorded run and event fields alongside coded demo data", async () => {
    const { http } = backend();
    const demo = createDemoAdapter();
    const demoRun = (await demo.listActions()).find((run) => run.id === activity.id)!;
    const wireRun = (await http.listActions())[0]!;
    expect([wireRun.id, wireRun.kind, wireRun.status, wireRun.stackId, wireRun.services]).toEqual([demoRun.id, demoRun.kind, demoRun.status, demoRun.stackId, demoRun.services]);
    expect(wireRun.steps.map((step) => [step.key, step.state, step.label.code])).toEqual([["start", "done", "activity.success"]]);
    expect(wireRun.log.map((entry) => entry.code)).toEqual(["activity.success"]);
    expect(demoRun.steps.length > 0 && demoRun.log.every((entry) => !!entry.code)).toBe(true);
    const demoEvent = (await demo.listEvents()).find((item) => item.id === event.id)!;
    const wireEvent = (await http.listEvents())[0]!;
    expect([wireEvent.id, wireEvent.kind, wireEvent.level, wireEvent.stackId]).toEqual([demoEvent.id, demoEvent.kind, demoEvent.level, demoEvent.stackId]);
    expect(wireEvent.title.code).toBe("project.deploy");
  });

  it("2 keeps an unsupported operation disabled and refuses direct calls without a request", async () => {
    const { http, fetcher, state } = backend();
    state.descriptors.push({ operationId: "runStackCommand", resourceRef: { kind: "stack", id: "dns" }, available: false, disabledReasonCode: "capability.mapping-unavailable" });
    const descriptor = await http.getCapability("runStackCommand", { kind: "stack", id: "dns" });
    expect([descriptor.available, descriptor.disabledReasonCode]).toEqual([false, "capability.mapping-unavailable"]);
    const before = fetcher.mock.calls.length;
    await expect(http.runStackCommand("dns", "start")).rejects.toMatchObject({ outcome: { kind: "not-available", operationId: "runStackCommand", reasonCode: "capability.mapping-unavailable" } });
    expect(fetcher.mock.calls.length).toBe(before);
    expect((await createDemoAdapter().getCapability("runStackCommand", { kind: "stack", id: "cloud" })).available).toBe(true);
    expect((await createDemoAdapter().getCapability("runStackCommand", { kind: "stack", id: "missing" })).available).toBe(false);
  });

  it("3 treats missing and unreadable descriptors as unavailable", async () => {
    const { http, state, fetcher } = backend();
    state.descriptors = [];
    expect((await http.getCapability("listActions")).available).toBe(false);
    await expect(http.listActions()).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    expect(fetcher.mock.calls.every((call) => String(call[0]).includes("capabilities"))).toBe(true);
    fetcher.mockRejectedValueOnce(new Error("offline"));
    expect((await http.getCapability("listEvents")).available).toBe(false);
    const demo = createDemoAdapter();
    vi.spyOn(demo, "getCapability").mockResolvedValueOnce({ operationId: "listActions", resourceRef: { kind: "hub", id: "0" }, available: false, disabledReasonCode: "capability.unavailable" }).mockRejectedValueOnce(new Error("offline"));
    expect((await demo.getCapability("listActions")).available).toBe(false);
    await expect(demo.getCapability("listActions")).rejects.toThrow("offline");
  });

  it.each(["run.busy", "run.cancelling"] as const)("4 preserves %s with its active run", async (code) => {
    const { http, fetcher } = backend();
    fetcher.mockImplementation(async (input) => String(input).includes("/events") ? json({ code, activeRunId: "active-1" }, 409) : backendResponse(String(input)));
    await expect(http.listEvents()).rejects.toMatchObject({ outcome: { kind: "conflict", code, activeRunId: "active-1" } });
    expect(fetcher.mock.calls.some((call) => String(call[0]).includes("/activities"))).toBe(false);
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
    source.send({ channel: "events", eventLog: { type: "changed", timestamp: later }, timestamp: later });
    expect(changes).toHaveBeenCalledWith(["events"]);
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
    b.state.detail.activity.status = "running";
    const active = (await b.http.listActions())[0]!;
    b.state.list = [];
    expect(await b.http.listActions()).toEqual([]);
    expect(active.output[0]?.text).toBe("ready");
  });

  it.each([["queued", "Wartet", "Queued"], ["cancelled", "Abgebrochen", "Cancelled"]] as const)("13/14 renders %s distinctly", (status, de, en) => {
    const b = backend(); b.state.detail.activity.status = status;
    const coded = mapActivityDetail(b.state.detail);
    expect(coded.status).toBe(status);
    expect(renderRun(coded, "de").status).toBe(status);
    expect([deActivity.status[status], enActivity.status[status]]).toEqual([de, en]);
    if (status === "queued") expect(pruneRuns([renderRun(coded, "de")], 0)).toHaveLength(1);
    else expect([TOAST_LIFETIME.cancelled, pruneRuns([renderRun(coded, "de")], 0).length]).toEqual([8000, 0]);
    renderWithApp(<RunFocusView run={renderRun(coded, "de")} now={Date.parse(later)} />, b.http);
    expect(screen.getAllByText(de).length).toBeGreaterThan(0);
  });
});

function backendResponse(input: string): Response {
  const url = new URL(input, "https://example.test");
  if (url.pathname.endsWith("/capabilities")) return ok(["listActions", "listEvents"].map((operationId) => ({ operationId, available: true, resourceRef: { kind: "hub", id: "0" } })));
  return page([]);
}

// Source: docklet-next/issues/611
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M39 #611 grouped scan notification regressions against the real coordinator.

import { act, cleanup, fireEvent } from "@testing-library/react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { toast } from "sonner";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ActionDetailState } from "@/data/adapter";
import type { GateNotice } from "@/data/run-gate";
import type { ScanRunSummary } from "@/domain/security";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapActivityDetail } from "@/data/http/map-dto";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import type { CodedActionRun } from "@/data/message-code";
import { queryKeys } from "@/data/query-keys";
import { renderWithApp } from "@/test/render";
import { actionDialogStore, closeActionDialog, openActionDialog } from "./action-dialog-store";
import { ActionToasts } from "./ActionToasts";
import { AppToaster } from "./AppToaster";

// The gate is a proxy of the HTTP adapter; tests drive its notices directly.
const gate = vi.hoisted(() => ({ listener: undefined as ((notice: GateNotice) => void) | undefined }));
vi.mock("@/data/run-gate", async (original) => ({
  ...(await original<typeof import("@/data/run-gate")>()),
  runGateOf: () => new Proxy({ subscribe: (listener: (notice: GateNotice) => void) => { gate.listener = listener; return () => { gate.listener = undefined; }; } } as Record<string, unknown>, {
    get: (target, prop) => (typeof prop === "string" && prop in target ? target[prop] : () => undefined),
  }),
}));

const at = "2026-11-24T20:16:00Z";
type Status = "running" | "success" | "failed";
function scan(id: string, host: string, status: Status, change: Record<string, unknown> = {}): CodedActionRun {
  return mapActivityDetail({ activity: { id, environmentId: host, type: "vulnerability_scan", resourceType: "environment", resourceId: "0", status, startedAt: at, createdAt: at, ...(status === "running" ? {} : { endedAt: at }), ...change }, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
}
function deploy(id: string): CodedActionRun {
  return mapActivityDetail({ activity: { id, environmentId: "first", type: "deploy", resourceType: "project", resourceId: "app", resourceName: "app", status: "running", startedAt: at, createdAt: at }, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
}

let client!: QueryClient;
function Grab() { const current = useQueryClient(); useEffect(() => { client = current; }, [current]); return null; }
/** Detail owners receive `details[id]` (a durable summary or candidate counters) on top of the listed run. */
function harness(initial: CodedActionRun[] = [], details: Record<string, Partial<CodedActionRun>> = {}) {
  const adapter = createDemoAdapter();
  let version = 0;
  const identityListeners = new Set<() => void>();
  registerIdentityLifetime(adapter.subscribeChanges, { version: () => version, active: () => true, listen: (l) => { identityListeners.add(l); return () => { identityListeners.delete(l); }; } });
  // Held runs get no terminal detail until released (a slow durable-outcome read); `failing` runs get a detail read error.
  const held = new Set<string>(), failing = new Set<string>(), silent = new Set<string>(), stale = new Set<string>();
  const lastRunning = new Map<string, CodedActionRun>();
  let current = initial;
  const listeners = new Set<(runs: CodedActionRun[]) => void>();
  const observers: { id: string; listener: (state: ActionDetailState) => void; active: boolean }[] = [];
  adapter.listActions = async () => current;
  adapter.subscribeActions = (listener) => { listeners.add(listener); listener(current); return () => { listeners.delete(listener); }; };
  // Like the real observer, every observation of the run list re-delivers the run's detail while it is observed.
  const deliver = (o: (typeof observers)[number]) => {
    const run = current.find((r) => r.id === o.id);
    if (!run || !o.active || silent.has(o.id) || (held.has(o.id) && run.status !== "running")) return;
    if (failing.has(o.id) && run.status !== "running") { o.listener({ status: "error", error: new Error("detail unavailable") }); return; }
    // The real detail owner keeps its last good (running) copy on a failed refresh.
    if (stale.has(o.id) && run.status !== "running") { o.listener({ status: "error", error: new Error("detail unavailable"), run: lastRunning.get(o.id) }); return; }
    if (run.status === "running") lastRunning.set(o.id, run);
    o.listener({ status: "ready", run: { ...run, ...(run.status === "running" ? {} : details[o.id]) } });
  };
  adapter.observeActionDetail = (_host, id, listener) => {
    const o = { id, listener, active: true };
    observers.push(o);
    queueMicrotask(() => deliver(o));
    return () => { o.active = false; };
  };
  const publish = (runs: CodedActionRun[]) => act(() => { current = runs; listeners.forEach((l) => l(runs)); observers.forEach(deliver); });
  renderWithApp(<><Grab /><ActionToasts /><AppToaster /></>, adapter);
  return {
    publish, observers,
    hold: (id: string) => { held.add(id); }, fail: (id: string) => { failing.add(id); }, silence: (id: string) => { silent.add(id); }, failStale: (id: string) => { stale.add(id); },
    release: (id: string) => act(() => { held.delete(id); observers.filter((o) => o.id === id).forEach(deliver); }),
    rotate: () => act(() => { version++; identityListeners.forEach((l) => l()); }),
  };
}
const summary = (patch: Partial<ScanRunSummary> = {}): ScanRunSummary => ({ status: "completed", targets: 4, scanned: 4, failed: 0, skipped: 0, counts: { critical: 1, high: 2, medium: 0, low: 0, unknown: 0 }, startedAt: at, finishedAt: at, ...patch });
const withSummary = (value: ScanRunSummary) => ({ scan: { targets: value.targets, done: value.scanned + value.failed, failed: value.failed, summary: value } });
const toasts = () => [...document.querySelectorAll<HTMLElement>('[data-slot="action-toast"]')];
const group = () => document.querySelector<HTMLElement>("[data-scan-group]");
const rows = () => [...document.querySelectorAll<HTMLElement>("[data-scan-member]")].map((el) => el.dataset.scanMember);
const lifeTrack = () => document.querySelector('[data-slot="toast-life"]');
const tick = (ms = 0) => act(async () => { await vi.advanceTimersByTimeAsync(ms); });

beforeEach(() => { vi.useFakeTimers(); setAppLocale("en-US"); });
afterEach(() => { act(() => { toast.dismiss(); closeActionDialog(); }); cleanup(); actionDialogStore.reset(); vi.useRealTimers(); setAppLocale("de"); });

// Rows 8 and 13 (aggregate lifetime, host count) and N1's single notification.
it("shows one notification for scans on several hosts and closes it only after all succeeded", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  expect(toasts().length).toBe(1);
  expect(group()?.textContent).toContain("Vulnerability scans · 2 hosts");
  expect(rows()).toEqual(["a", "b"]);
  await f.publish([scan("a", "first", "success"), scan("b", "second", "running")]);
  await tick();
  expect(lifeTrack() === null).toBe(true);
  await f.publish([scan("a", "first", "success"), scan("b", "second", "success")]);
  await tick();
  expect(lifeTrack() !== null).toBe(true);
  fireEvent.animationEnd(lifeTrack()!);
  await tick(500);
  expect(toasts().length).toBe(0);
});

it("keeps the countdown off while a later member runs and keeps a failure until closed", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  await f.publish([scan("b", "second", "running"), scan("a", "first", "success")]);
  await tick();
  expect([rows(), lifeTrack() === null]).toEqual([["a", "b"], true]);
  await f.publish([scan("b", "second", "failed"), scan("a", "first", "success")]);
  await tick();
  expect([group()?.dataset.status, group()?.getAttribute("role"), lifeTrack() === null]).toEqual(["failed", "alert", true]);
  await tick(60_000);
  expect(toasts().length).toBe(1);
});

it("counts distinct hosts and keeps a manual and a candidate run on one host apart", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("candidate", "first", "running", { type: "vulnerability.candidate-scan" }), scan("manual", "first", "running")]);
  await tick();
  expect(group()?.textContent).toContain("Vulnerability scans · 1 host");
  expect(rows()).toEqual(["manual", "candidate"]);
});

// Row 6: closing while running, then a later failure reopens with that run only.
it("reopens with the failed run only after the group was closed", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("a", "first", "running"), scan("b", "second", "running")]);
  await tick();
  fireEvent.click(group()!.querySelector('button[aria-label="Close notice"]')!);
  await tick(500);
  expect(toasts().length).toBe(0);
  await f.publish([scan("a", "first", "success"), scan("b", "second", "failed")]);
  await tick();
  expect(toasts().length).toBe(1);
  expect([group() === null, toasts()[0]?.dataset.status, lifeTrack() === null]).toEqual([true, "failed", true]);
});

// Row 7: the dialog covers one member only.
it("removes only the member shown in the dialog and brings it back while it runs", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("a", "first", "running"), scan("b", "second", "running")]);
  await tick();
  act(() => openActionDialog("a"));
  await tick();
  expect([group() === null, toasts().length]).toEqual([true, 1]);
  act(() => closeActionDialog());
  await tick();
  expect(rows().sort()).toEqual(["a", "b"]);
});

// Row 11: history stays silent and a non-scan run keeps its own toast.
it("keeps history silent and other runs in their own toast", async () => {
  const f = harness([scan("old", "first", "running")]);
  await tick();
  expect(toasts().length).toBe(0);
  await f.publish([deploy("d"), scan("b", "second", "running"), scan("a", "first", "running"), scan("old", "first", "running")]);
  await tick();
  expect([toasts().length, rows()]).toEqual([2, ["a", "b"]]);
});

// Row 12: a member that leaves the run list leaves the group.
it("prunes members that left the run list", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("a", "first", "running"), scan("b", "second", "running")]);
  await tick();
  await f.publish([scan("b", "second", "running")]);
  await tick();
  expect([group() === null, toasts().length]).toEqual([true, 1]);
});

// Row 3: an identity replacement dismisses the group and old members never return.
it("dismisses the group on an identity replacement", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("a", "first", "running"), scan("b", "second", "running")]);
  await tick();
  act(() => { client.setQueryData(queryKeys.currentUser, { id: "replacement", username: "replacement" }); });
  await f.publish([scan("a", "first", "running"), scan("b", "second", "running")]);
  await tick(500);
  expect(toasts().length).toBe(0);
});

it("keeps a single successful scan's countdown and a single failed scan without one", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("a", "first", "running")]);
  await tick();
  await f.publish([scan("a", "first", "success")]);
  await tick();
  expect([group() === null, lifeTrack() !== null]).toEqual([true, true]);
  await f.publish([scan("a", "first", "success"), scan("b", "first", "running")]);
  await tick();
  await f.publish([scan("a", "first", "success"), scan("b", "first", "failed")]);
  await tick();
  expect(group()?.dataset.status).toBe("failed");
});

// Row 6 / S23-R01: a scan shown right after a close is not removed by the closed instance's deferred exit.
it("keeps a scan that starts while the closed group is still leaving", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  fireEvent.click(group()!.querySelector('button[aria-label="Close notice"]')!);
  await tick(16);
  await f.publish([scan("c", "first", "running"), scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick(1_000);
  expect(toasts().filter((t) => t.dataset.status === "running").length).toBe(1);
});

// Row 8 / S23-R02: partial results that only detail owners know keep the group, also for candidate runs.
it.each([
  ["host partial", { a: withSummary(summary({ status: "partial", failed: 1, scanned: 3 })) }],
  ["candidate deferred", { a: { candidateScan: { targets: 5, scanned: 3, cached: 0, deferred: 2, failed: 0 } } }],
] as const)("keeps a succeeded activity with a %s result until closed", async (_, details) => {
  const candidate = _ === "candidate deferred" ? { type: "vulnerability.candidate-scan" } : {};
  const f = harness([], details as Record<string, Partial<CodedActionRun>>);
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running", candidate)]);
  await tick();
  await f.publish([scan("b", "second", "success"), scan("a", "first", "success", candidate)]);
  await tick(50);
  expect([group()?.dataset.status, lifeTrack() === null]).toEqual(["failed", true]);
});

it("stops the countdown when a new scan joins and restarts it after that scan succeeded", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  await f.publish([scan("b", "second", "success"), scan("a", "first", "success")]);
  await tick();
  expect(lifeTrack() !== null).toBe(true);
  await f.publish([scan("c", "third", "running"), scan("b", "second", "success"), scan("a", "first", "success")]);
  await tick();
  expect([rows(), lifeTrack() === null]).toEqual([["a", "b", "c"], true]);
  await f.publish([scan("c", "third", "success"), scan("b", "second", "success"), scan("a", "first", "success")]);
  await tick();
  expect(lifeTrack() !== null).toBe(true);
});

// S23-R03: a failed host scan shows its error and its durable result.
it("shows a failed member's error and its result line", async () => {
  setAppLocale("en-US");
  const f = harness([], { b: withSummary(summary({ status: "partial", failed: 1, scanned: 3 })) });
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  await f.publish([scan("b", "second", "failed", { messageCode: "vulnerability.scan-failed" }), scan("a", "first", "success")]);
  await tick(50);
  const row = document.querySelector('[data-scan-member="b"]')!;
  expect([row.querySelector('[data-slot="scan-member-error"]') !== null, row.querySelector('[data-slot="scan-member-line"]')?.textContent]).toEqual([true, "3 images scanned · 1 critical · 2 high · 1 failed · partial result · 0 s"]);
});

// Row 13: a manual and a candidate run on one host keep their own results.
it("keeps each run's own result for a manual and a candidate run on one host", async () => {
  setAppLocale("en-US");
  const f = harness([], { manual: withSummary(summary()), candidate: { candidateScan: { targets: 2, scanned: 2, cached: 0, deferred: 0, failed: 0 } } });
  await f.publish([]);
  await f.publish([scan("candidate", "first", "running", { type: "vulnerability.candidate-scan" }), scan("manual", "first", "running")]);
  await tick();
  await f.publish([scan("candidate", "first", "success", { type: "vulnerability.candidate-scan" }), scan("manual", "first", "success")]);
  await tick(50);
  const line = (id: string) => document.querySelector(`[data-scan-member="${id}"] [data-slot="scan-member-line"]`)?.textContent;
  expect([line("manual"), line("candidate")]).toEqual(["4 images scanned · 1 critical · 2 high · 0 s", "2 scanned, 0 from cache of 2 targets"]);
});

// Row 3: a late detail delivery of the old identity cannot bring back its runs.
it("ignores old detail deliveries after an identity replacement", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  act(() => { client.setQueryData(queryKeys.currentUser, { id: "replacement", username: "replacement" }); });
  // The new identity sees the same runs as history: only the identity reset can remove the old group.
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick(500);
  expect(toasts().length).toBe(0);
  act(() => { for (const { id, listener } of f.observers) listener({ status: "ready", run: scan(id, id === "a" ? "first" : "second", "failed") }); });
  await tick(500);
  expect(toasts().length).toBe(0);
});

// Row 11: a gate reveal brings back a dismissed running scan inside the group, unless the dialog shows it.
it("reveals a dismissed running scan in the group only when it is not covered", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  fireEvent.click(group()!.querySelector('button[aria-label="Close notice"]')!);
  await tick(500);
  act(() => openActionDialog("a"));
  act(() => gate.listener?.({ reason: "inflight", key: "scan:first", runId: "a" }));
  await tick(500);
  expect(toasts().length).toBe(0);
  act(() => closeActionDialog());
  await tick(500);
  const shown = toasts().length;
  act(() => gate.listener?.({ reason: "inflight", key: "scan:second", runId: "b" }));
  await tick(500);
  expect([shown <= 1, toasts().length, rows().sort()]).toEqual([true, 1, ["a", "b"]]);
});

// CXR-01: the next identity's group survives the retiring identity's deferred exit.
it("keeps the new identity's scan while the old identity's group is still leaving", async () => {
  const f = harness();
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  await f.rotate();
  await tick(16);
  await f.publish([scan("c", "third", "running"), scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick(1_000);
  expect(toasts().filter((t) => t.dataset.status === "running").length).toBe(1);
});

// CXR-02: a settled list row whose durable outcome is still loading is pending, not success.
it.each(["group", "single"] as const)("holds the countdown until the %s scan's outcome is known, then keeps a partial result", async (shape) => {
  const f = harness([], { a: withSummary(summary({ status: "partial", failed: 1, scanned: 3 })) });
  await f.publish([]);
  const running = shape === "group" ? [scan("b", "second", "running"), scan("a", "first", "running")] : [scan("a", "first", "running")];
  await f.publish(running);
  await tick();
  f.hold("a");
  await f.publish(running.map((r) => scan(r.id, r.hostId!, "success")));
  await tick(50);
  expect(lifeTrack() === null).toBe(true);
  await f.release("a");
  await tick(50);
  expect([lifeTrack() === null, toasts().length]).toEqual([true, 1]);
});

it("starts the countdown once a held outcome turns out complete, and after a failed detail read", async () => {
  const f = harness([], { a: withSummary(summary()) });
  await f.publish([]);
  await f.publish([scan("b", "second", "running"), scan("a", "first", "running")]);
  await tick();
  f.hold("a"); f.fail("b");
  await f.publish([scan("b", "second", "success"), scan("a", "first", "success")]);
  await tick(50);
  expect(lifeTrack() === null).toBe(true);
  await f.release("a");
  await tick(50);
  expect(lifeTrack() !== null).toBe(true);
});

it("holds the countdown for a candidate run until its counters arrive and keeps a deferred result", async () => {
  const candidate = { type: "vulnerability.candidate-scan" };
  const f = harness([], { a: { candidateScan: { targets: 5, scanned: 3, cached: 0, deferred: 2, failed: 0 } } });
  await f.publish([]);
  await f.publish([scan("a", "first", "running", candidate)]);
  await tick();
  f.hold("a");
  await f.publish([scan("a", "first", "success", candidate)]);
  await tick(50);
  expect(lifeTrack() === null).toBe(true);
  await f.release("a");
  await tick(50);
  expect([lifeTrack() === null, toasts().length]).toEqual([true, 1]);
});

// CXR-02 (first load): a single scan whose first detail has not arrived shows the listed run without a countdown.
it.each([["host", {}], ["candidate", { type: "vulnerability.candidate-scan" }]] as const)("keeps a settled single %s scan without countdown while its first detail loads", async (_, change) => {
  const f = harness();
  f.silence("a");
  await f.publish([]);
  await f.publish([scan("a", "first", "running", change)]);
  await tick();
  await f.publish([scan("a", "first", "success", change)]);
  await tick(50);
  expect([toasts().length, toasts()[0]?.dataset.status, lifeTrack() === null]).toEqual([1, "succeeded", true]);
});

// CXR-04: a failed detail refresh that keeps the running copy still lets a settled scan close normally.
it.each(["group", "single"] as const)("starts the %s countdown when a failed detail refresh kept the running copy", async (shape) => {
  const f = harness();
  await f.publish([]);
  const running = shape === "group" ? [scan("b", "second", "running"), scan("a", "first", "running")] : [scan("a", "first", "running")];
  await f.publish(running);
  await tick();
  f.failStale("a");
  await f.publish(running.map((r) => scan(r.id, r.hostId!, "success")));
  await tick(50);
  expect([toasts().length, toasts()[0]?.dataset.status, lifeTrack() !== null]).toEqual([1, "succeeded", true]);
});

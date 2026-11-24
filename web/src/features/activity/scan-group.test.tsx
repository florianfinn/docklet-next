// Source: docklet-next/issues/611
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M39 #611 grouped scan notification regressions against the real coordinator.

import { act, cleanup, fireEvent } from "@testing-library/react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapActivityDetail } from "@/data/http/map-dto";
import type { CodedActionRun } from "@/data/message-code";
import { queryKeys } from "@/data/query-keys";
import { renderWithApp } from "@/test/render";
import { actionDialogStore, closeActionDialog, openActionDialog } from "./action-dialog-store";
import { ActionToasts } from "./ActionToasts";
import { AppToaster } from "./AppToaster";

const at = "2026-11-24T20:16:00Z";
type Status = "running" | "success" | "failed";
function scan(id: string, host: string, status: Status, change: Record<string, unknown> = {}): CodedActionRun {
  return mapActivityDetail({ activity: { id, environmentId: host, type: "vulnerability_scan", resourceType: "environment", resourceId: "0", status, startedAt: at, createdAt: at, ...(status === "running" ? {} : { endedAt: at }), ...change }, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
}
function deploy(id: string): CodedActionRun {
  return mapActivityDetail({ activity: { id, environmentId: "first", type: "deploy", resourceType: "project", resourceId: "app", resourceName: "app", status: "running", startedAt: at, createdAt: at }, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
}

let client!: QueryClient;
function Grab() { client = useQueryClient(); return null; }
function harness(initial: CodedActionRun[] = []) {
  const adapter = createDemoAdapter();
  let current = initial;
  const listeners = new Set<(runs: CodedActionRun[]) => void>();
  adapter.listActions = async () => current;
  adapter.subscribeActions = (listener) => { listeners.add(listener); listener(current); return () => { listeners.delete(listener); }; };
  adapter.observeActionDetail = undefined;
  const publish = (runs: CodedActionRun[]) => act(() => { current = runs; listeners.forEach((l) => l(runs)); });
  renderWithApp(<><Grab /><ActionToasts /><AppToaster /></>, adapter);
  return { publish };
}
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

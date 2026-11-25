// Source: docklet-next/issues/626
// Revision: 505d1715fdf1dc1668655e9affef78b67620f7d9
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bulk check presentation and producer-shaped regression coverage.

import { act, cleanup, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { activityFixture, at, deferred, type Activity } from "@/data/http/activity-reader.test-support";
import { renderWithApp } from "@/test/render";
import { Toaster } from "@/shared/ui/sonner";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { bindPresentation, presentationStore } from "./run-presentation";
import { ActionToasts } from "./ActionToasts";

const bulk = { unit: "target" as const, planned: 4, plannedKnown: true, completed: 1, succeeded: 1, failed: 0, skipped: 0, updates: 0 };
const job = (): Activity => ({ id: "example-check", environmentId: "good", type: "job_run", resourceType: "job", resourceId: "update-check-host", startedAt: at, createdAt: at, status: "running", bulk: { ...bulk }, stepTrace: [{ id: "step-running", name: "running", code: "job.running", status: "running", startedAt: at }] });
afterEach(() => { toast.dismiss(); cleanup(); vi.useRealTimers(); uiStyleStore.reset(); presentationStore.reset(); setAppLocale("de"); });
it("B10 a producer-shaped notification updates an open normal toast 1→2 through the real HTTP adapter with bounded rereads", async () => {
  setAppLocale("en-US");
  const f = activityFixture(); f.state.lists.good = []; f.state.lists.bad = [];
  await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const view = renderWithApp(<><ActionToasts /><Toaster /></>, f.adapter);
  await waitFor(() => expect(f.sources.length).toBe(1));
  const row = job(); bindPresentation(row.id, "toast"); f.state.lists.good = [row]; f.state.details[row.id] = { activity: row, stepTrace: row.stepTrace!, messages: [], output: [], outputTruncated: false };
  const notify = () => f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type: "updated", activityId: row.id, environmentId: "good", activity: row } }) } as MessageEvent);
  await act(async () => { notify(); });
  const visible = () => document.querySelector('[data-slot="action-toast"]');
  await waitFor(() => expect(visible() !== null).toBe(true));
  expect(visible()?.textContent).toContain("1 of 4 checked");
  const lists = () => f.requests.filter((url) => /\/good\/activities\?/.test(url)).length;
  const before = lists();
  row.bulk = { ...bulk, completed: 2, succeeded: 2 };
  await act(async () => { notify(); });
  await waitFor(() => expect(visible()?.textContent).toContain("2 of 4 checked"));
  expect(lists() - before).toBeLessThanOrEqual(1);
  expect(f.requests.some((url) => url.includes(`/activities/${row.id}`))).toBe(true);
  row.status = "success"; row.endedAt = "2026-11-25T12:01:00Z"; row.bulk = { ...bulk, completed: 4, succeeded: 3, skipped: 1, updates: 2 };
  const terminalBefore = lists();
  await act(async () => { notify(); });
  await waitFor(() => expect(visible()?.textContent).toContain("3 of 4 checked · 2 with updates · 1 skipped"));
  expect(lists() - terminalBefore).toBeLessThanOrEqual(1);
  view.unmount();
});
it("B10 a held stale read cannot replace later completed counts", async () => {
  setAppLocale("en-US"); const f = activityFixture(); const row = job(); f.state.lists.good = [row]; f.state.lists.bad = [];
  f.state.details[row.id] = { activity: row, stepTrace: row.stepTrace!, messages: [], output: [], outputTruncated: false };
  await f.adapter.getCurrentUser(); await f.adapter.listActions();
  const received: number[] = [], off = f.adapter.subscribeActions((runs) => { const run = runs.find((r) => r.id === row.id); if (run?.bulk) received.push(run.bulk.completed); });
  const held = deferred<Response>(), reached = deferred<void>(); let first = true;
  f.state.hold = async (path, response) => { if (first && path.endsWith(`/activities/${row.id}`)) { first = false; reached.resolve(); return held.promise; } return response; };
  const notify = () => f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: at, activity: { type: "updated", environmentId: "good", activity: row } }) } as MessageEvent);
  try {
    notify(); await reached.promise;
    row.bulk = { ...bulk, completed: 2, succeeded: 2 }; notify();
    held.resolve(Response.json({ success: true, data: { ...f.state.details[row.id], activity: { ...row, bulk } } }));
    await vi.waitFor(() => expect(received.at(-1)).toBe(2), { timeout: 5000 });
    expect(received.slice(received.indexOf(2) + 1)).not.toContain(1);
  } finally { off(); }
});

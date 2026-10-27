// Source: docklet-next/issues/102
// Revision: 1819a8a7ed4f4b8116c2195c9ba5e3b16a03120a
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original source-local initial history consumer regressions, informed by R4 P5.

import { act, screen } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, expect, it, vi } from "vitest";
import { deferred } from "@/data/http/activity-reader.test-support";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { actionDialogStore } from "./action-dialog-store";
import { activityStore } from "./activity-store";
import { presentationStore } from "./run-presentation";
import { sourceFixture, toastCount, waitFor, type Fault } from "./source-notification.test-support";

afterEach(() => act(() => { toast.dismiss(); actionDialogStore.reset(); presentationStore.reset(); uiStyleStore.reset(); activityStore.reset(); vi.restoreAllMocks(); }));

const faults: Fault[] = ["page502", "network", "page500", "page403", "summary-missing", "summary-mapping", "partial", "marked-first", "marked-later"];
it.each(faults.flatMap((fault) => [false, true].map((reverse) => ({ fault, reverse }))))("$fault initial bad plus partial Edge history, reverse=$reverse: recovery stays silent and genuine actions notify", async ({ fault, reverse }) => {
  const f = sourceFixture(reverse);
  f.put("good", "good-done", "success"); f.history("bad"); f.history("edge");
  f.faults.set("bad", fault); f.faults.set("edge", "partial"); await f.discover();
  const view = f.mount();
  try {
    await f.load(); await f.load();
    expect(f.observed.mock.lastCall?.[0].some((r: { id: string }) => r.id === "good-done")).toBe(true);
    expect(f.health.mock.lastCall?.[0].failures.map((failure: { hostId: string }) => failure.hostId).sort()).toEqual(["bad", "edge"]);
    expect(f.shows).not.toHaveBeenCalled(); expect(actionDialogStore.get()).toEqual({ waiting: [] });
    f.put("good", "healthy-while-missing"); await f.load();
    expect(f.ids()).toEqual(["healthy-while-missing"]);
    await waitFor(() => expect(toastCount()).toBe(1));
    // A new healthy failure in the SAME publication as B's first complete history is genuine.
    f.state.details["good-done"]!.activity.status = "failed"; f.faults.delete("bad"); await f.load();
    expect(f.ids()).toEqual(["healthy-while-missing", "good-done"]);
    expect(f.health.mock.lastCall?.[0].failures.map((failure: { hostId: string }) => failure.hostId)).toEqual(["edge"]);
    expect(f.observed.mock.lastCall?.[0].filter((r: { hostId: string }) => r.hostId === "bad")).toHaveLength(3);
    f.faults.delete("edge"); await f.load();
    expect(f.shows).toHaveBeenCalledTimes(2); expect(f.health.mock.lastCall?.[0].failures).toEqual([]);
    f.put("bad", "bad-genuine"); await f.load();
    f.state.details["edge-history-running"]!.activity.status = "failed"; await f.load();
    expect(f.ids()).toEqual(["healthy-while-missing", "good-done", "bad-genuine", "edge-history-running"]);
    expect(actionDialogStore.get()).toEqual({ waiting: [] });
    expect(f.requests.filter((p) => p.includes("/activities")).length).toBeLessThan(130);
  } finally { view.stop(); f.client.clear(); }
});

it.each([false, true])("partial summaries and warnings remain visible before silent recovery, reverse=%s", async (reverse) => {
  const f = sourceFixture(reverse); f.put("good", "good-done", "success");
  f.put("bad", "retained-summary", "failed"); f.history("bad"); f.faults.set("bad", "partial"); await f.discover();
  const view = f.mount(true);
  try {
    await f.load(); f.openCenter(); await screen.findByRole("dialog");
    await waitFor(() => expect(document.getElementById("run-retained-summary") !== null).toBe(true));
    expect(screen.getByRole("alert").textContent).toContain("The source could not be reached or read.");
    expect(document.getElementById("run-good-done") !== null).toBe(true);
    expect(f.shows).not.toHaveBeenCalled();
    // Already visible partial rows retain their genuine transitions despite malformed history.
    f.state.details["retained-summary"]!.activity.status = "running"; await f.load();
    f.state.details["retained-summary"]!.activity.status = "failed"; await f.load();
    expect(f.ids()).toEqual(["retained-summary"]);
    f.faults.delete("bad"); await f.load();
    await waitFor(() => expect(screen.queryByText("Some activities could not be read") === null).toBe(true));
    expect(document.getElementById("run-bad-history-failed") !== null).toBe(true);
    expect(f.ids()).toEqual(["retained-summary"]); expect(actionDialogStore.get()).toEqual({ waiting: [] });
  } finally { view.stop(); f.client.clear(); }
});

it.each([false, true])("deferred source completion order reverse=%s preserves healthy dialog offers during first recovery", async (reverse) => {
  const f = sourceFixture(reverse); f.history("bad"); f.history("edge");
  f.faults.set("bad", "page502"); f.faults.set("edge", "partial"); await f.discover();
  const view = f.mount(false, "dialog");
  const reached = deferred<void>(); const release = deferred<void>(); const finished = deferred<void>();
  const original = f.state.hold!;
  try {
    await f.load(); f.faults.delete("bad"); f.put("good", "healthy-major");
    f.state.hold = async (path, response, url) => {
      const result = await original(path, response, url);
      if (path.endsWith(`/${reverse ? "good" : "bad"}/activities`)) { reached.resolve(); await release.promise; }
      if (path.endsWith(`/${reverse ? "bad" : "good"}/activities`)) finished.resolve();
      return result;
    };
    const pending = f.adapter.listActions(); await Promise.all([reached.promise, finished.promise]);
    expect(actionDialogStore.get()).toEqual({ waiting: [] });
    await act(async () => { release.resolve(); await pending; });
    expect(actionDialogStore.get()).toEqual({ runId: "healthy-major", waiting: [] });
    expect(f.shows).not.toHaveBeenCalled();
    f.put("good", "healthy-next-major"); f.faults.delete("edge"); await f.load();
    expect(actionDialogStore.get()).toEqual({ runId: "healthy-major", waiting: ["healthy-next-major"] });
    expect(f.ids()).toEqual(["healthy-next-major"]);
    await waitFor(() => expect(toastCount()).toBe(1));
  } finally { release.resolve(); view.stop(); f.client.clear(); }
});

it.each(["page502", "partial"] as const)("two-source stream first read %s and recovery seed history without notices", async (fault) => {
  const f = sourceFixture(); f.state.hosts = ["good", "bad"]; f.put("good", "good-done", "success"); f.history("bad");
  f.faults.set("bad", fault); await f.discover(); const view = f.mount();
  try {
    await act(async () => { f.sources.at(-1)!.open(); });
    await waitFor(() => expect(f.observed.mock.calls.length).toBeGreaterThan(0));
    expect(f.shows).not.toHaveBeenCalled(); f.faults.delete("bad");
    await act(async () => { f.sources.at(-1)!.send(); });
    await waitFor(() => expect(f.observed.mock.lastCall?.[0].some((r: { id: string }) => r.id === "bad-history-queued")).toBe(true));
    expect(f.shows).not.toHaveBeenCalled(); expect(actionDialogStore.get()).toEqual({ waiting: [] });
    f.put("bad", "stream-genuine"); await act(async () => { f.sources.at(-1)!.send(); });
    await waitFor(() => expect(f.ids()).toEqual(["stream-genuine"]));
    await waitFor(() => expect(toastCount()).toBe(1));
  } finally { view.stop(); f.client.clear(); }
});

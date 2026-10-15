import { screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ActionRunner } from "@/data/demo/action-runner";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { HISTORY_LIMITS, pruneEvents, pruneRuns, validateHistoryLimit } from "@/domain/activity-history";
import type { ActionRun, OpsEvent } from "@/domain/model";
import { renderRoutes } from "@/test/render";

import SettingsPage from "./SettingsPage";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 50 })));
afterEach(() => {
  vi.useRealTimers();
  simulationStore.reset();
  localStorage.clear();
});

const run = (id: string, status: ActionRun["status"]): ActionRun => ({ id, kind: "restart", title: id, target: id, services: [], status, progress: 0, startedAt: "2026-10-12T08:00:00Z", steps: [], log: [], output: [] });

describe("activity history limits", () => {
  it("keep every running action and only the newest finished ones, in order", () => {
    const runs = [run("r1", "running"), run("f1", "succeeded"), run("r2", "running"), run("f2", "failed"), run("f3", "succeeded"), run("r3", "running")];
    expect(pruneRuns(runs, 2).map((r) => r.id)).toEqual(["r1", "f1", "r2", "f2", "r3"]);
    expect(pruneRuns(runs, 0).map((r) => r.id)).toEqual(["r1", "r2", "r3"]);
    const events = Array.from({ length: 6 }, (_, i) => ({ id: `e${i}` }) as OpsEvent);
    expect(pruneEvents(events, 4).map((e) => e.id)).toEqual(["e0", "e1", "e2", "e3"]);
    expect([HISTORY_LIMITS.events.default, HISTORY_LIMITS.actions.default]).toEqual([500, 100]);
    expect([validateHistoryLimit("events", 500), validateHistoryLimit("events", 49), validateHistoryLimit("actions", 1001), validateHistoryLimit("actions", 12.5), validateHistoryLimit("actions", Number.NaN)]).toEqual([null, "range", "range", "integer", "integer"]);
  });

  it("never evict a running action when more starts than the limit arrive; finished ones leave oldest first", async () => {
    vi.useFakeTimers();
    const runner = new ActionRunner([], undefined, () => 2);
    const plan = (title: string, ms: number) => ({ kind: "restart" as const, title, target: title, steps: [{ key: "start" as const, label: "Start", ms }] });
    const slow = runner.start(plan("slow", 60_000));
    for (let i = 0; i < 4; i++) runner.start(plan(`quick-${i}`, 10));
    await vi.advanceTimersByTimeAsync(1_000);
    const list = runner.list();
    expect([list.some((r) => r.id === slow && r.status === "running"), list.filter((r) => r.status !== "running").map((r) => r.title)]).toEqual([true, ["quick-3", "quick-2"]]);
  });

  it("keep finished runs by finish time, not by list position, and leave the order alone", () => {
    const fin = (id: string, startedAt: string, finishedAt?: string): ActionRun => ({ ...run(id, "succeeded"), startedAt, finishedAt });
    // Listed by start (newest first); "old" started first but finished last.
    const runs = [fin("n3", "2026-10-12T08:03:00Z", "2026-10-12T08:03:01Z"), fin("n2", "2026-10-12T08:02:00Z", "2026-10-12T08:02:01Z"), fin("old", "2026-10-12T08:00:00Z", "2026-10-12T08:10:00Z"), fin("seed", "2026-10-12T08:01:00Z")];
    expect(pruneRuns(runs, 2).map((r) => r.id)).toEqual(["n3", "old"]);
    expect(pruneRuns(runs, 3).map((r) => r.id)).toEqual(["n3", "n2", "old"]);
  });

  it("report success of a long run once and keep it in the history when newer short runs fill the limit", async () => {
    vi.useFakeTimers();
    const runner = new ActionRunner([], undefined, () => 10);
    const onSuccess = vi.fn();
    const onFailure = vi.fn();
    const plan = (title: string, ms: number) => ({ kind: "restart" as const, title, target: title, steps: [{ key: "start" as const, label: "Start", ms }] });
    const longId = runner.start({ ...plan("long", 1000), onSuccess, onFailure });
    for (let i = 0; i < 10; i++) runner.start(plan(`quick-${i}`, 1));
    await vi.advanceTimersByTimeAsync(2_000);
    const finished = runner.list().filter((r) => r.status !== "running");
    expect([onSuccess.mock.calls.length, onFailure.mock.calls.length, runner.list().find((r) => r.id === longId)?.status, finished.length, finished.some((r) => r.title === "quick-0")]).toEqual([1, 0, "succeeded", 10, false]);
    expect((onSuccess.mock.calls[0]?.[0] as ActionRun).id).toBe(longId);
  });

  it("report failure of a long run once with its error output", async () => {
    vi.useFakeTimers();
    const runner = new ActionRunner([], undefined, () => 10);
    const onSuccess = vi.fn();
    const onFailure = vi.fn();
    const plan = (title: string, ms: number) => ({ kind: "restart" as const, title, target: title, steps: [{ key: "start" as const, label: "Start", ms }] });
    const longId = runner.start({ ...plan("long", 1000), failAt: "start", failMessage: "boom", onSuccess, onFailure });
    for (let i = 0; i < 10; i++) runner.start(plan(`quick-${i}`, 1));
    await vi.advanceTimersByTimeAsync(2_000);
    const long = runner.list().find((r) => r.id === longId);
    expect([onSuccess.mock.calls.length, onFailure.mock.calls.length, long?.status]).toEqual([0, 1, "failed"]);
    const errorLine = (r?: ActionRun) => r?.output.some((l) => l.text === "boom" && l.level === "error" && l.stream === "stderr");
    expect([errorLine(long), errorLine(onFailure.mock.calls[0]?.[0] as ActionRun)]).toEqual([true, true]);
    expect((onFailure.mock.calls[0]?.[0] as ActionRun).id).toBe(longId);
  });

  it("clear busyActionId of a stack whose long run finishes behind ten newer finished runs", async () => {
    const adapter = createDemoAdapter();
    const stacks = await adapter.listStacks();
    const target = stacks[0]!;
    const other = stacks.find((s) => s.id !== target.id && s.services.length > 0)!;
    await adapter.saveSettings("activity", { ...(await adapter.getSettings()).activity, maxFinishedActions: 10 });
    await adapter.runStackCommand(target.id, "redeploy");
    for (let i = 0; i < 10; i++) await adapter.restartService(other.id, other.services[0]!.name);
    expect((await adapter.getStack(target.id))?.busyActionId !== undefined).toBe(true);
    await waitFor(async () => expect((await adapter.getStack(target.id))?.busyActionId).toBeUndefined(), { timeout: 4000 });
    expect((await adapter.listActions()).filter((r) => r.status !== "running" && r.stackId === target.id).length).toBe(1);
  });

  it("shows fixed history limits without editable controls", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/settings/:section", element: <SettingsPage /> }], ["/settings/activity"], adapter);
    await screen.findByText("Der Verlauf behält 500 Ereignisse und 100 abgeschlossene Abläufe je Umgebung. Aktive Abläufe bleiben erhalten.");
    expect(screen.queryByLabelText("Höchstens Ereignisse") === null).toBe(true);
    expect(screen.queryByLabelText("Höchstens abgeschlossene Aktionen") === null).toBe(true);
    await expect(adapter.saveSettings("activity", { ...(await adapter.getSettings()).activity, maxFinishedActions: 5 })).rejects.toThrow("invalid-history");
  });
});

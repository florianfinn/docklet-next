import { afterEach, describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";

import type { ActionKind, ActionRun, ActionStatus, UpdateInfo } from "@/domain/model";
import { checkTimes, lastUpdateAt, stackUpdateTimes } from "@/shared/components/updates/update-times";
import { DEFAULT_UI_STYLE, effectivePresentation, mergeUiStyle } from "@/shared/preferences/ui-style-store";

import { stepTone } from "./run-phase";
import { bindPresentation, initialPresentation, isCovered, presentationStore, RunToastCoordinator, startPresented, useInlineRunSurface, type PresentationState } from "./run-presentation";

afterEach(() => presentationStore.reset());

const run = (id: string, kind: ActionKind, status: ActionStatus = "running", stackId = "home"): ActionRun => ({
  id,
  kind,
  title: id,
  target: "Home · Atlas",
  stackId,
  services: [],
  steps: [],
  status,
  progress: 0,
  startedAt: "2026-10-11T10:00:00.000Z",
  log: [],
  output: [],
});

const state = (patch: Partial<PresentationState> = {}): PresentationState => ({ claims: [], byRun: {}, surfaces: {}, ...patch });

describe("presentation policy", () => {
  it("opens the dialog only for major actions with automatic opening on", () => {
    expect([initialPresentation(run("a", "redeploy"), state(), true).presentation, initialPresentation(run("b", "redeploy"), state(), false).presentation, initialPresentation(run("c", "check-updates"), state(), true).presentation]).toEqual(["dialog", "toast", "toast"]);
  });

  it("follows an explicit claim of the start site for its stack and kinds only", () => {
    const claim = { id: 1, stackId: "home", kinds: new Set<ActionKind>(["redeploy", "restore"]), presentation: "inline" as const };
    const s = state({ claims: [claim] });
    expect([initialPresentation(run("a", "redeploy"), s, true), initialPresentation(run("b", "stop"), s, true).presentation, initialPresentation(run("c", "redeploy", "running", "cloud"), s, true).presentation]).toEqual([{ presentation: "inline", claim }, "toast", "dialog"]);
  });

  it("prefers a bound run id over claims", () => {
    expect(initialPresentation(run("a", "redeploy"), state({ byRun: { a: "toast" }, claims: [{ id: 1, stackId: "home", kinds: new Set<ActionKind>(["redeploy"]), presentation: "inline" }] }), true).presentation).toBe("toast");
  });

  it("counts a run as covered while the dialog shows it or a mounted inline surface of its stack does", () => {
    const inline = state({ byRun: { a: "inline", b: "inline" }, surfaces: { home: 1 } });
    expect([isCovered(run("a", "redeploy"), "a", state()), isCovered(run("a", "redeploy"), undefined, inline), isCovered(run("a", "redeploy"), undefined, state({ byRun: { a: "inline" } })), isCovered(run("b", "stop"), undefined, inline)]).toEqual([true, true, false, false]);
  });

  it("registers the claim before the start resolves, binds the id afterwards and drops the claim", async () => {
    let during: PresentationState | undefined;
    const id = await startPresented({ stackId: "home" }, "inline", async () => {
      during = presentationStore.get();
      return "run-1";
    });
    expect([id, during?.claims.length, presentationStore.get().claims.length, presentationStore.get().byRun["run-1"]]).toEqual(["run-1", 1, 0, "inline"]);
  });
});

/** Fake surroundings: a dialog store and a toast list as the coordinator sees them. */
function harness(autoDialog = true) {
  let dialogRunId: string | undefined;
  const waiting: string[] = [];
  const shown = new Set<string>();
  const log: string[] = [];
  const coordinator: RunToastCoordinator = new RunToastCoordinator(
    {
      show: (id) => {
        shown.add(id);
        log.push(`show:${id}`);
      },
      hide: (id) => {
        shown.delete(id);
        log.push(`hide:${id}`);
      },
      offerDialog: (id) => {
        log.push(`offer:${id}`);
        if (dialogRunId) waiting.push(id);
        else dialogRunId = id;
        // The real dialog store notifies synchronously.
        coordinator.sync();
      },
    },
    { autoDialog: () => autoDialog, dialogRunId: () => dialogRunId, state: () => presentationStore.get() },
  );
  return {
    coordinator,
    shown,
    log,
    waiting,
    close: () => {
      dialogRunId = undefined;
      coordinator.sync();
    },
    show: (id: string) => {
      dialogRunId = id;
      coordinator.sync();
    },
  };
}

describe("toasts beside the dialog and inline surfaces", () => {
  it("keeps quiet for runs already present and for the run the dialog shows", () => {
    const h = harness();
    h.coordinator.sync([run("old", "redeploy", "running")]);
    h.coordinator.sync([run("a", "redeploy"), run("old", "redeploy")]);
    expect([h.log, [...h.shown]]).toEqual([["offer:a"], []]);
  });

  it("shows the toast on closing only while the run still runs", () => {
    const h = harness();
    h.coordinator.sync([]);
    h.coordinator.sync([run("a", "redeploy")]);
    h.close();
    expect([...h.shown]).toEqual(["a"]);

    const settled = harness();
    settled.coordinator.sync([]);
    settled.coordinator.sync([run("a", "redeploy")]);
    settled.coordinator.sync([run("a", "redeploy", "succeeded")]);
    settled.close();
    expect([...settled.shown]).toEqual([]);
  });

  it("gives a run that starts while the dialog shows another its own toast, and swaps toasts when the dialog switches", () => {
    const h = harness();
    h.coordinator.sync([]);
    h.coordinator.sync([run("a", "redeploy")]);
    h.coordinator.sync([run("b", "redeploy"), run("a", "redeploy")]);
    expect([h.waiting, [...h.shown]]).toEqual([["b"], ["b"]]);
    h.show("b");
    expect([...h.shown]).toEqual(["a"]);
  });

  it("shows a failure that happens uncovered, but not one the dialog already shows", () => {
    const h = harness(false);
    h.coordinator.sync([]);
    h.coordinator.sync([run("a", "redeploy")]);
    h.shown.delete("a"); // closed by hand
    h.coordinator.sync([run("a", "redeploy", "failed")]);
    expect([...h.shown]).toEqual(["a"]);

    const covered = harness();
    covered.coordinator.sync([]);
    covered.coordinator.sync([run("a", "redeploy")]);
    covered.coordinator.sync([run("a", "redeploy", "failed")]);
    expect(covered.log).toEqual(["offer:a"]);
  });

  it("leaves an inline run to its surface and shows the toast while the surface is gone", () => {
    const h = harness();
    presentationStore.set((s) => ({ ...s, surfaces: { home: 1 }, claims: [{ id: 9, stackId: "home", kinds: new Set<ActionKind>(["restore"]), presentation: "inline" }] }));
    h.coordinator.sync([]);
    h.coordinator.sync([run("a", "restore")]);
    expect([h.log, presentationStore.get().byRun.a, presentationStore.get().claims.length]).toEqual([[], "inline", 0]);

    presentationStore.set((s) => ({ ...s, surfaces: { home: 0 } }));
    h.coordinator.sync();
    presentationStore.set((s) => ({ ...s, surfaces: { home: 1 } }));
    h.coordinator.sync();
    h.coordinator.sync([run("a", "restore", "succeeded")]);
    presentationStore.set((s) => ({ ...s, surfaces: { home: 0 } }));
    h.coordinator.sync();
    expect(h.log).toEqual(["show:a", "hide:a"]);
  });

  it("binds a claim to exactly one run", () => {
    const h = harness();
    presentationStore.set((s) => ({ ...s, surfaces: { home: 1 }, claims: [{ id: 3, stackId: "home", kinds: new Set<ActionKind>(["redeploy"]), presentation: "inline" }] }));
    h.coordinator.sync([]);
    h.coordinator.sync([run("b", "redeploy"), run("a", "redeploy")]);
    expect([presentationStore.get().byRun, h.log]).toEqual([{ a: "inline" }, ["offer:b"]]);
  });

  it("does not let a binding of another surface affect plain runs", () => {
    bindPresentation("x", "toast");
    expect(isCovered(run("x", "redeploy"), undefined, presentationStore.get())).toBe(false);
  });
});

describe("bounded bookkeeping", () => {
  const tracked = (c: RunToastCoordinator) => (c as unknown as { seen: Map<string, unknown> | null }).seen?.size;

  it("forgets seen runs and their bindings once the runs left the list", () => {
    const h = harness();
    h.coordinator.sync([]);
    const runs = Array.from({ length: 100 }, (_, i) => run(`r${i}`, "restart", "succeeded"));
    h.coordinator.sync(runs);
    for (const r of runs) bindPresentation(r.id, "toast");
    expect([tracked(h.coordinator), Object.keys(presentationStore.get().byRun).length]).toEqual([100, 100]);
    h.coordinator.sync([]);
    expect([tracked(h.coordinator), Object.keys(presentationStore.get().byRun).length]).toEqual([0, 0]);
  });

  it("keeps a binding for a run that was not reported yet", () => {
    const h = harness();
    h.coordinator.sync([]);
    bindPresentation("later", "inline");
    h.coordinator.sync([run("other", "restart", "succeeded")]);
    h.coordinator.sync([]);
    expect(presentationStore.get().byRun).toEqual({ later: "inline" });
  });

  it("does not show a still listed failed run again after other runs were pruned", () => {
    const h = harness();
    h.coordinator.sync([]);
    h.coordinator.sync([run("f", "restart", "failed"), run("a", "restart", "succeeded"), run("b", "restart", "succeeded")]);
    h.shown.clear();
    h.log.length = 0;
    h.coordinator.sync([run("f", "restart", "failed")]);
    h.coordinator.sync([run("f", "restart", "failed")]);
    expect([h.log, tracked(h.coordinator)]).toEqual([[], 1]);
  });

  it("does not bind a run late that left the list before its start resolved", async () => {
    const h = harness();
    h.coordinator.sync([]);
    for (let i = 0; i < 100; i++) {
      await startPresented({ stackId: "home" }, "inline", async () => {
        h.coordinator.sync([run(`p${i}`, "redeploy", "succeeded")]);
        h.coordinator.sync([]);
        return `p${i}`;
      });
    }
    expect([Object.keys(presentationStore.get().byRun).length, presentationStore.get().claims.length]).toEqual([0, 0]);
  });

  it("drops the surface key when the last inline surface unmounts", () => {
    const a = renderHook(() => useInlineRunSurface("home"));
    const b = renderHook(() => useInlineRunSurface("home"));
    expect(presentationStore.get().surfaces).toEqual({ home: 2 });
    a.unmount();
    expect(presentationStore.get().surfaces).toEqual({ home: 1 });
    b.unmount();
    expect(presentationStore.get().surfaces).toEqual({});
  });
});

describe("run colours", () => {
  it("keeps finished steps in the brand tone while running and turns them semantic once settled", () => {
    expect([stepTone("done", "running"), stepTone("running", "running"), stepTone("done", "succeeded"), stepTone("done", "failed"), stepTone("error", "failed"), stepTone("skipped", "failed")]).toEqual(["run", "run", "ok", "ok", "err", "gray"]);
  });
});

describe("presentation preferences", () => {
  it("defaults to the focus dialog with automatic opening and sanitises the run motion", () => {
    // A stored "off" from the former browser switch becomes the matching override.
    const merged = mergeUiStyle({ runMotion: "spin", actionDialogAuto: false }, DEFAULT_UI_STYLE);
    expect([DEFAULT_UI_STYLE.actionDialogDesign, DEFAULT_UI_STYLE.actionPresentationOverride, merged.runMotion, merged.actionPresentationOverride, mergeUiStyle({ runMotion: "stripes", actionDialogAuto: true }, DEFAULT_UI_STYLE).actionPresentationOverride]).toEqual(["focus", "hub", "sweep", "toast", "hub"]);
    expect([effectivePresentation("hub", "toast"), effectivePresentation("hub", undefined), effectivePresentation("dialog", "toast")]).toEqual(["toast", "dialog", "dialog"]);
  });
});

describe("update times", () => {
  const img = { repository: "example/app", tag: "1" };
  const updates: UpdateInfo[] = [
    { state: "current", current: img, checkedAt: "2026-10-10T08:00:00.000Z", registry: "ghcr.io" },
    { state: "error", current: img, message: "x", checkedAt: "2026-10-11T08:00:00.000Z" },
    { state: "unchecked", current: img },
    { state: "local", current: img },
  ];

  it("takes the newest recorded check and counts images without one", () => {
    expect([checkTimes(updates), checkTimes([{ state: "unchecked", current: img }])]).toEqual([{ checkedAt: "2026-10-11T08:00:00.000Z", unchecked: 2 }, { checkedAt: undefined, unchecked: 1 }]);
  });

  it("uses only successful update runs of the stack, history and live runs together", () => {
    const r = (kind: ActionKind, status: ActionStatus, finishedAt: string, stackId = "home") => ({ kind, status, finishedAt, stackId });
    const runs = [r("update", "succeeded", "2026-10-09T10:00:00.000Z"), r("update", "failed", "2026-10-11T10:00:00.000Z"), r("redeploy", "succeeded", "2026-10-11T11:00:00.000Z"), r("update", "succeeded", "2026-10-11T12:00:00.000Z", "cloud")];
    expect([lastUpdateAt(runs, "home"), lastUpdateAt([], "home")]).toEqual(["2026-10-09T10:00:00.000Z", undefined]);
    const times = stackUpdateTimes({ id: "home", services: [], deployments: [] }, [r("update", "succeeded", "2026-10-11T13:00:00.000Z")]);
    expect(times).toEqual({ checkedAt: undefined, unchecked: 0, updatedAt: "2026-10-11T13:00:00.000Z" });
  });
});

describe("source history supplied with an observation", () => {
  it("seeds incomplete history statuses silently through the complete read, then reports a later failure", () => {
    const h = harness(false); h.coordinator.sync([]);
    h.coordinator.sync([run("history", "redeploy")], new Set(["history"]));
    h.coordinator.sync([run("history", "redeploy", "failed")], new Set(["history"]));
    expect(h.log).toEqual([]);
    h.coordinator.sync([run("history", "redeploy", "running")]);
    h.coordinator.sync([run("history", "redeploy", "failed")]);
    expect(h.log).toEqual(["show:history"]);
  });

  it("preserves a genuine simultaneous major dialog across reentrant passes and later coverage changes", () => {
    const h = harness(); h.coordinator.sync([]);
    h.coordinator.sync([run("history", "redeploy"), run("genuine", "redeploy")], new Set(["history"]));
    expect([h.log, h.waiting]).toEqual([["offer:genuine"], []]);
    // A presentation-only sync after seeding must use recorded coverage, rather than seed again.
    h.show("history"); h.close();
    expect(h.log).toEqual(["offer:genuine", "show:genuine", "hide:history", "show:history"]);
  });

  it("forgets seeded history and its bindings after authoritative removal", () => {
    const h = harness(false); h.coordinator.sync([]);
    h.coordinator.sync([run("history", "redeploy", "failed")], new Set(["history"]));
    bindPresentation("history", "toast"); h.coordinator.sync([]);
    expect(presentationStore.get().byRun).toEqual({});
    h.coordinator.sync([run("history", "redeploy", "failed")]);
    expect(h.log).toEqual(["show:history"]);
  });
});

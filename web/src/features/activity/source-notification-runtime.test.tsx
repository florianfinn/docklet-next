// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { act } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, expect, it, vi } from "vitest";
import { uiStyleStore } from "@/shared/preferences/ui-style-store";
import { actionDialogStore } from "./action-dialog-store";
import { activityStore } from "./activity-store";
import { presentationStore, startPresented } from "./run-presentation";
import { deferred } from "@/data/http/activity-reader.test-support";
import type { CodedActionRun } from "@/data/message-code";
import { mapRuntime } from "@/data/http/runtime-mapping";
import { snapshot } from "@/data/http/runtime-commands.test-support";
import { sourceFixture } from "./source-notification.test-support";

afterEach(() => act(() => { toast.dismiss(); actionDialogStore.reset(); presentationStore.reset(); uiStyleStore.reset(); activityStore.reset(); vi.restoreAllMocks(); }));
it("PR107-R5-F2 complete source then marked-partial visible running-to-failed transition notifies", async () => {
  const f = sourceFixture(); f.put("bad", "visible-run"); await f.discover(); const view = f.mount();
  try {
    await f.load(); expect(f.ids()).toEqual([]);
    f.faults.set("bad", "partial"); f.state.details["visible-run"]!.activity.status = "failed";
    await f.load(); expect(f.ids()).toEqual(["visible-run"]);
    await f.load(); f.faults.delete("bad"); await f.load(); expect(f.ids()).toEqual(["visible-run"]);
  } finally { view.stop(); f.client.clear(); }
});
it.each([false, true])("PR107-R5-F1 current claim notifies during persistent incompleteness or first recovery=%s while unseen history remains silent", async (complete) => {
  const f = sourceFixture(); f.history("bad"); f.faults.set("bad", "partial"); await f.discover(); const view = f.mount(false, "dialog");
  const release = deferred<string>();
  try {
    await f.load();
    f.put("bad", "current-command"); f.state.details["current-command"]!.activity.resourceType = "project"; f.state.details["current-command"]!.activity.resourceId = "target";
    // A marked full-window response includes the claimed row while history remains incomplete.
    f.state.lists.bad = [f.state.details["current-command"]!.activity, ...f.state.lists.bad!.filter((r) => r.id !== "current-command")];
    if (complete) f.faults.delete("bad"); else f.faults.set("bad", "marked-first");
    let pending!: Promise<string>;
    act(() => { pending = startPresented({ stackId: "bad~target" }, "dialog", () => release.promise); });
    await f.load(); expect(actionDialogStore.get().runId).toBe("current-command"); expect(f.ids()).toEqual([]);
    await act(async () => { release.resolve("current-command"); await pending; });
    f.faults.delete("bad"); await f.load(); expect(actionDialogStore.get().waiting).toEqual([]); expect(f.ids()).toEqual([]);
  } finally { release.resolve("current-command"); view.stop(); f.client.clear(); }
});
it("PR107-R5-F1 persistent malformed old summary never suppresses an observed healthy row failure", async () => {
  const f = sourceFixture(); f.put("bad", "visible"); f.put("bad", "malformed-old");
  f.state.details["malformed-old"]!.activity.type = undefined as unknown as string;
  await f.discover(); const view = f.mount();
  try {
    await f.load(); f.state.details.visible!.activity.status = "failed"; await f.load();
    expect(f.ids()).toEqual(["visible"]); expect(f.health.mock.lastCall?.[0].failures).toContainEqual({ hostId: "bad", reason: "invalid-data" });
    f.put("good", "healthy-new", "failed"); await f.load(); expect(f.ids()).toEqual(["visible", "healthy-new"]);
    f.state.details["malformed-old"]!.activity.type = "project_redeploy"; await f.load(); expect(f.ids()).toEqual(["visible", "healthy-new"]);
  } finally { view.stop(); f.client.clear(); }
});

it("targeted admission before the first aggregate read preserves the source history baseline", async () => {
  const f = sourceFixture(); f.history("good"); f.history("bad"); f.faults.set("bad", "partial"); await f.discover();
  const listeners = new Set<(runs: CodedActionRun[]) => void>();
  const subscribe = f.adapter.subscribeActions;
  f.adapter.subscribeActions = (listener) => { listeners.add(listener); const off = subscribe(listener); return () => { listeners.delete(listener); off(); }; };
  const view = f.mount();
  try {
    const run = mapRuntime(snapshot("edge"), "edge"); run.runtime = { currentIntent: true, sourceObserved: false };
    act(() => { for (const listener of listeners) listener([run]); });
    expect(f.ids()).toEqual([run.id]);
    await f.load(); expect(f.ids()).toEqual([run.id]);
    f.faults.delete("bad"); await f.load(); expect(f.ids()).toEqual([run.id]);
    f.put("good", "new-after-baseline", "failed"); await f.load(); expect(f.ids()).toEqual([run.id, "new-after-baseline"]);
  } finally { view.stop(); f.client.clear(); }
});

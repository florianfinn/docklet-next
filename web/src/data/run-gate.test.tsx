import { act, screen } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ActionToasts } from "@/features/activity/ActionToasts";
import { AppToaster } from "@/features/activity/AppToaster";
import { useAdapter } from "@/data/use-adapter";
import { renderWithApp } from "@/test/render";

import { createDemoAdapter } from "./demo/demo-adapter";
import { simulationStore } from "./demo/simulation";
import { CHECK_FRESH_MS, isRunBusy, runGateOf, withRunGate, type GateNotice } from "./run-gate";

beforeEach(() => {
  vi.useFakeTimers();
  // Maximum demo speed: a stack redeploy settles in well under a second of fake time.
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 20 }));
});
afterEach(() => {
  act(() => void toast.dismiss());
  vi.useRealTimers();
  simulationStore.reset();
  localStorage.clear();
});

const settle = () => act(async () => void (await vi.advanceTimersByTimeAsync(2_000)));

function gated() {
  const inner = createDemoAdapter();
  const spy = { check: vi.spyOn(inner, "checkUpdates"), stack: vi.spyOn(inner, "runStackCommand") };
  const adapter = withRunGate(inner);
  const notices: GateNotice[] = [];
  runGateOf(adapter)?.subscribe((n) => notices.push(n));
  const started = () => inner.listActions().then((runs) => runs.filter((r) => r.status === "running").length);
  return { inner, adapter, spy, notices, started };
}

describe("run gate", () => {
  it("answers concurrent identical starts with one run and one promise", async () => {
    const { adapter, spy, started } = gated();
    const ids = await Promise.all([adapter.checkUpdates({ stackId: "media" }), adapter.checkUpdates({ stackId: "media" }), adapter.checkUpdates({ stackId: "media" })]);
    expect([new Set(ids).size, spy.check.mock.calls.length, await started()]).toEqual([1, 1, 1]);
  });

  it("returns the running run to later clicks from other entry points and blocks a conflicting mutation", async () => {
    const { adapter, spy, notices } = gated();
    const first = await adapter.runStackCommand("media", "redeploy");
    // Header, overview and list menu all ask for the same redeploy while it runs.
    const again = await Promise.all([adapter.runStackCommand("media", "redeploy"), adapter.runStackCommand("media", "redeploy")]);
    expect([again.every((id) => id === first), spy.stack.mock.calls.length]).toEqual([true, 1]);
    // A different mutation of the same stack is refused, not queued behind it.
    const stop = await adapter.runStackCommand("media", "stop").catch((e: unknown) => e);
    expect([isRunBusy(stop), isRunBusy(stop) && stop.runId === first, spy.stack.mock.calls.length]).toEqual([true, true, 1]);
    expect(notices.map((n) => n.reason)).toEqual(["inflight", "inflight", "busy"]);
    // Other targets never wait.
    const other = await adapter.runStackCommand("cloud", "redeploy");
    expect([other !== first, spy.stack.mock.calls.length]).toEqual([true, 2]);
    // After the run, the same mutation may start again at once (no cooldown for mutations).
    await settle();
    const next = await adapter.runStackCommand("media", "redeploy");
    expect([next !== first, spy.stack.mock.calls.length]).toEqual([true, 3]);
  });

  it("answers a just finished check with its result, offers an explicit recheck and expires after the window", async () => {
    const { adapter, spy, notices } = gated();
    const first = await adapter.checkUpdates({ stackId: "home" });
    await settle();
    const fresh = await adapter.checkUpdates({ stackId: "home" });
    const notice = notices.at(-1);
    expect([fresh, spy.check.mock.calls.length, notice?.reason, typeof notice?.retry]).toEqual([first, 1, "fresh", "function"]);
    // The notice's recheck bypasses freshness exactly once.
    const forced = await notice!.retry!();
    expect([forced !== first, spy.check.mock.calls.length]).toEqual([true, 2]);
    await settle();
    await act(async () => void (await vi.advanceTimersByTimeAsync(CHECK_FRESH_MS)));
    const later = await adapter.checkUpdates({ stackId: "home" });
    expect([later !== forced, spy.check.mock.calls.length]).toEqual([true, 3]);
  });

  it("lets a failed check be retried right away and an all-host check cover a stack check", async () => {
    const { adapter, spy } = gated();
    // Cedar is offline in the demo: its check fails at the first step.
    const failed = await adapter.checkUpdates({ stackId: "wiki" });
    await settle();
    const retry = await adapter.checkUpdates({ stackId: "wiki" });
    expect([retry !== failed, spy.check.mock.calls.length]).toEqual([true, 2]);
    await settle();
    const all = await adapter.checkUpdates({});
    const covered = await adapter.checkUpdates({ stackId: "media" });
    expect([covered, spy.check.mock.calls.length]).toEqual([all, 3]);
  });

  it("refuses a container mutation while its stack runs, and the reverse, but not a sibling container", async () => {
    const { inner, adapter } = gated();
    const containers = await inner.listContainers();
    const [a, b] = containers.filter((c) => c.stackId === "media" && c.state === "running");
    const gate = runGateOf(adapter)!;
    const off = gate.setParentResolver((scope) => {
      const c = containers.find((x) => `container:${x.id}` === scope);
      return c?.stackId ? `stack:${c.stackId}` : undefined;
    });
    await adapter.runStackCommand("media", "redeploy");
    const blocked = await adapter.runContainerCommand(a!.id, "restart").catch((e: unknown) => e);
    expect(isRunBusy(blocked)).toBe(true);
    await settle();
    await adapter.runContainerCommand(a!.id, "restart");
    const sibling = await adapter.runContainerCommand(b!.id, "restart").catch((e: unknown) => e);
    const stack = await adapter.runStackCommand("media", "stop").catch((e: unknown) => e);
    expect([isRunBusy(sibling), isRunBusy(stack)]).toEqual([false, true]);
    off();
  });

  it("forgets settled starts, stops following the run list when idle and shares one gate per adapter", async () => {
    const { inner, adapter } = gated();
    const original = inner.subscribeActions.bind(inner);
    let following = 0;
    const follow = vi.spyOn(inner, "subscribeActions").mockImplementation((listener) => {
      following++;
      const off = original(listener);
      return () => {
        following--;
        off();
      };
    });
    for (const id of ["media", "cloud", "home"]) await adapter.runStackCommand(id, "restart");
    const gate = runGateOf(adapter)!;
    expect([gate.size, follow.mock.calls.length, following]).toEqual([3, 1, 1]);
    // Finished mutations leave with the run list update that ends them; idle, the gate stops following.
    await settle();
    expect([gate.size, following]).toEqual([0, 0]);
    // A finished check stays only while fresh.
    await adapter.checkUpdates({ stackId: "home" });
    await settle();
    expect([gate.size, following]).toEqual([1, 1]);
    await act(async () => void (await vi.advanceTimersByTimeAsync(CHECK_FRESH_MS)));
    await adapter.checkUpdates({ stackId: "media" });
    expect(gate.size).toBe(1);
    expect(withRunGate(inner)).toBe(adapter);
  });
});

function TwoEntryPoints() {
  const adapter = useAdapter();
  return (
    <>
      <button type="button" onClick={() => void adapter.checkUpdates({ stackId: "media" })}>
        Kopf
      </button>
      <button type="button" onClick={() => void adapter.checkUpdates({ stackId: "media" })}>
        Übersicht
      </button>
      <ActionToasts />
      <AppToaster />
    </>
  );
}

describe("run gate in the app", () => {
  it("shows one progress toast for rapid clicks from two entry points and one freshness notice afterwards", async () => {
    renderWithApp(<TwoEntryPoints />);
    await act(async () => void (await vi.advanceTimersByTimeAsync(10)));
    for (let i = 0; i < 4; i++) {
      act(() => screen.getByRole("button", { name: i % 2 ? "Übersicht" : "Kopf" }).click());
      await act(async () => void (await vi.advanceTimersByTimeAsync(20)));
    }
    expect(document.querySelectorAll('[data-slot="action-toast"]').length).toBe(1);
    await settle();
    for (const name of ["Kopf", "Übersicht", "Kopf"]) {
      act(() => screen.getByRole("button", { name }).click());
      await act(async () => void (await vi.advanceTimersByTimeAsync(20)));
    }
    expect([document.querySelectorAll('[data-slot="action-toast"]').length, screen.getAllByText(/gerade geprüft/).length]).toEqual([1, 1]);
    expect(screen.getByRole("button", { name: "Erneut prüfen" }) !== null).toBe(true);
  });
});

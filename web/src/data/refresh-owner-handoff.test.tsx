// Source: docklet-next/issues/391
// Revision: f2a8411892f4f9c2c33bcb98c56e28b800aba43f
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original regressions for the candidate-fix review R1G-1..R1G-3 at the adapter and provider boundary.

import { act, cleanup, render } from "@testing-library/react";
import { QueryClientProvider, QueryObserver, isCancelledError, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterProvider } from "./AdapterProvider";
import { useImages } from "./queries";
import { useBackupOverviews } from "./backup-queries";
import { recoveryFixture } from "./http/recovery-diagnosis.test-support";
import { backupFixture, overview } from "./http/backups.test-support";
import { createAppQueryClient } from "./app-query-client";
import { refresh } from "./refresh-owner";
import type { DockletAdapter } from "./adapter";
import { registerIdentityLifetime } from "./identity-lifetime";

afterEach(() => { cleanup(); vi.useRealTimers(); });
const tick = async (ms = 0) => { await act(() => vi.advanceTimersByTimeAsync(ms)); };

/** A mounted query: the first read answers "warm"; a refetch is held until `finish`/`fail`; later reads answer "fresh". */
function warmHeld(client: QueryClient, key: string) {
  let calls = 0, finish!: (value: string) => void, fail!: (error: Error) => void;
  function Reader() {
    useQuery({ queryKey: [key], queryFn: () => {
      calls++;
      if (calls === 1) return Promise.resolve("warm");
      if (calls === 2) return new Promise<string>((resolve, reject) => { finish = resolve; fail = reject; });
      return Promise.resolve("fresh");
    } });
    return null;
  }
  render(<QueryClientProvider client={client}><Reader /></QueryClientProvider>);
  return {
    calls: () => calls, finish: (value: string) => finish(value), fail: (error: Error) => fail(error),
    async refetching() {
      await vi.waitFor(() => expect(client.getQueryData([key])).toBe("warm"));
      void client.refetchQueries({ queryKey: [key] });
      await vi.waitFor(() => expect(calls).toBe(2));
    },
  };
}

it.each(["images", "backup-overviews"] as const)("a change notification during a first load of %s joins it: one read, rows published", async (kind) => {
  vi.useFakeTimers();
  const f = recoveryFixture();
  await f.adapter.getCurrentUser();
  const suffix = kind === "images" ? "images" : "volumes/backups/overview";
  for (const host of ["first", "second"]) f.state.hold.add(`/api/environments/${host}/${suffix}`);
  const call = vi.spyOn(f.adapter, kind === "images" ? "listImages" : "listBackupOverviews");
  let client!: QueryClient;
  function Images() { useImages(); return null; }
  function Backups() { useBackupOverviews(); return null; }
  function Client() { client = useQueryClient(); return null; }
  render(<AdapterProvider adapter={f.adapter}><IntlProvider locale="en-US" messages={catalogs["en-US"]}><Client />{kind === "images" ? <Images /> : <Backups />}</IntlProvider></AdapterProvider>);
  await tick(100);
  expect(f.pending.filter((p) => p.path.endsWith(suffix))).toHaveLength(2);
  if (kind === "images") { f.state.edge = "offline"; f.send(); } else f.source.onopen?.({} as Event);
  await tick();
  f.release(); await tick(100);
  expect([call.mock.calls.length, (client.getQueryData<unknown[]>([kind])?.length ?? 0) > 0, f.pending.some((p) => p.aborted)]).toEqual([1, true, false]);
});

it("R1G-2 a backup completion ends a completed shared overview before the next background read", async () => {
  vi.useFakeTimers();
  const f = recoveryFixture(); await f.adapter.getCurrentUser();
  const off = f.adapter.subscribeChanges(() => {});
  await f.adapter.getBackupOverview("first", "background");
  const reads = () => f.requests.filter((p) => p.path === "/api/environments/first/volumes/backups/overview").length;
  const policy = overview.volumes![0]!.policy!, oldNextRun = policy.nextRunAt, nextRun = "2026-12-01T01:00:00Z";
  try {
    policy.nextRunAt = nextRun;
    f.source.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: "2026-11-11T12:00:01Z", activity: { type: "activity", environmentId: "first", activity: { id: "example-backup", type: "resource_action", resourceType: "volume", resourceId: "data", environmentId: "first", createdAt: "2026-11-11T12:00:00Z", endedAt: "2026-11-11T12:00:01Z", metadata: { action: "backup_volume" }, status: "success" } } }), lastEventId: "" } as MessageEvent);
    const after = await f.adapter.getBackupOverview("first", "background");
    expect([reads(), after.volumes[0]?.backup?.nextRunAt]).toEqual([2, nextRun]);
  } finally { policy.nextRunAt = oldNextRun; off(); }
});

it("R1G-3 a change after the last check but before Query publishes is refreshed once more", async () => {
  const client = createAppQueryClient(), q = warmHeld(client, "handoff");
  await q.refetching();
  let fetchingAtChange: string | undefined;
  q.finish("old");
  queueMicrotask(() => { fetchingAtChange = client.getQueryState(["handoff"])?.fetchStatus; refresh(client, [["handoff"]]); });
  await vi.waitFor(() => expect(client.getQueryData(["handoff"])).toBe("fresh"));
  expect([fetchingAtChange, q.calls(), client.getQueryState(["handoff"])?.fetchStatus]).toEqual(["fetching", 3, "idle"]);
});

it("F-7 an earlier identity's in-flight shared overview rejects; the new identity reads again", async () => {
  const f = backupFixture(); let calls = 0, entered!: () => void, release!: () => void;
  const reached = new Promise<void>((resolve) => { entered = resolve; }), held = new Promise<void>((resolve) => { release = resolve; });
  f.intercept(async (url) => { if (url.pathname.endsWith("/overview") && ++calls === 1) { entered(); await held; } return undefined; });
  const old = f.api.getBackupOverview("first", "background").catch((error: unknown) => error);
  await reached; f.rotate();
  const reading = f.api.getBackupOverview("first", "background").then((row) => row.hostId, (error: unknown) => error);
  release();
  const current = await reading;
  expect([isCancelledError(await old), current, calls]).toEqual([true, "first", 2]);
});

/** A warm query whose refetch is held until its signal aborts. */
function heldWarm(client: QueryClient, key: string) {
  let calls = 0;
  const observer = new QueryObserver(client, { queryKey: [key], queryFn: ({ signal }) => {
    calls++; if (calls === 1) return Promise.resolve("warm");
    return new Promise<string>((resolve) => { signal.addEventListener("abort", () => resolve("cancelled"), { once: true }); });
  } });
  return { observer, calls: () => calls, off: observer.subscribe(() => {}) };
}

it("R2G-1 cancelling a superseded warm refetch starts no further read", async () => {
  const client = createAppQueryClient(), q = heldWarm(client, "cancel-warm");
  await vi.waitFor(() => expect(client.getQueryData(["cancel-warm"])).toBe("warm"));
  void q.observer.refetch(); refresh(client, [["cancel-warm"]]);
  await client.cancelQueries({ queryKey: ["cancel-warm"] }, { revert: false });
  await new Promise((resolve) => setTimeout(resolve, 10));
  expect(q.calls()).toBe(2);
  q.off(); client.clear();
});

it("R2G-1 an identity-style reset of a superseded warm refetch starts no further read", async () => {
  const client = createAppQueryClient(), q = heldWarm(client, "reset-warm");
  await vi.waitFor(() => expect(client.getQueryData(["reset-warm"])).toBe("warm"));
  void q.observer.refetch(); refresh(client, [["reset-warm"]]);
  const cancel = client.cancelQueries({ queryKey: ["reset-warm"] }, { revert: false });
  client.getQueryCache().find({ queryKey: ["reset-warm"] })!.reset();
  await cancel; await new Promise((resolve) => setTimeout(resolve, 10));
  expect(q.calls()).toBe(2);
  q.off(); client.clear();
});

it("R2G-1 the IdentityBridge's inactive reset never starts a new read", async () => {
  let active = true, notify!: () => void, client!: QueryClient, calls = 0;
  const states: boolean[] = [];
  const adapter = { kind: "demo", subscribeChanges: () => () => {}, getCurrentUser: async () => null } as unknown as DockletAdapter;
  registerIdentityLifetime(adapter.subscribeChanges, { active: () => active, version: () => (active ? 0 : 1), listen: (fn) => { notify = fn; return () => {}; } });
  function Reader() {
    client = useQueryClient();
    useQuery({ queryKey: ["images"], queryFn: ({ signal }) => {
      calls++; states.push(active);
      if (calls === 1) return Promise.resolve("warm");
      return new Promise<string>((resolve) => { signal.addEventListener("abort", () => resolve("cancelled"), { once: true }); });
    } });
    return null;
  }
  const view = render(<AdapterProvider adapter={adapter}><Reader /></AdapterProvider>);
  await vi.waitFor(() => expect(client.getQueryData(["images"])).toBe("warm"));
  await act(async () => { refresh(client, [["images"]]); refresh(client, [["images"]]); });
  await act(async () => { active = false; notify(); });
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 10)); });
  view.unmount();
  expect(states).toEqual([true, true]);
});

it("R2G-2 removing a superseded query releases its tail subscription", async () => {
  const client = createAppQueryClient(), cache = client.getQueryCache();
  const subscribe = cache.subscribe.bind(cache);
  const stops: Array<ReturnType<typeof vi.fn>> = [];
  vi.spyOn(cache, "subscribe").mockImplementation((listener) => { const stop = vi.fn(subscribe(listener)); stops.push(stop); return stop; });
  client.setQueryData(["removed"], "warm");
  void client.fetchQuery({ queryKey: ["removed"], staleTime: 0, queryFn: () => new Promise<string>(() => {}) }).catch(() => {});
  refresh(client, [["removed"]]);
  client.removeQueries({ queryKey: ["removed"] });
  await Promise.resolve();
  expect([stops.length, stops[0]?.mock.calls.length]).toEqual([1, 1]);
});

it("a partial cache write during a superseded fetch is not taken for its publication", async () => {
  const client = createAppQueryClient(), q = warmHeld(client, "manual");
  await q.refetching();
  q.finish("old");
  // In the handoff gap: a late change, then a manual partial write before the fetch publishes.
  queueMicrotask(() => { refresh(client, [["manual"]]); client.setQueryData(["manual"], "partial"); });
  await vi.waitFor(() => expect(client.getQueryData(["manual"])).toBe("fresh"));
  expect(q.calls()).toBe(3);
});

it("a change in the handoff gap before an error publication is refreshed once more", async () => {
  const client = createAppQueryClient(), q = warmHeld(client, "gap-error");
  await q.refetching();
  q.fail(new Error("old failure"));
  queueMicrotask(() => { refresh(client, [["gap-error"]]); });
  await vi.waitFor(() => expect(client.getQueryData(["gap-error"])).toBe("fresh"));
  expect(q.calls()).toBe(3);
});

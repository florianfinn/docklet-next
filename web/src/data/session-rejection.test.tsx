// Source: docklet-next/docs/design/runtime-control.md
// Revision: 274674bb319eb7b1b854688ebc945e831c4dd53f
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original actual adapter/provider rejection-ownership regressions.

import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, render, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { AdapterProvider } from "./AdapterProvider";
import type { DockletAdapter } from "./adapter";
import type { MetricsSnapshot } from "@/domain/model";
import { identityLifetime } from "./identity-lifetime";
import { queryKeys } from "./query-keys";
import { useCurrentUser } from "./queries";
import { deferred } from "./http/activity-reader.test-support";
import { wireLog } from "./http/logs.test-support";
import { response } from "./http/runtime-commands.test-support";
import { resourceFixture } from "./http/resources.test-support";
import { sessionFixture } from "./http/session-lifecycle.test-support";

afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

async function mountedIdentity(adapter: DockletAdapter) {
  let client!: QueryClient;
  function Observer() { client = useQueryClient(); useCurrentUser(); return null; }
  const view = render(<AdapterProvider adapter={adapter}><Observer /></AdapterProvider>);
  await waitFor(() => expect(client.getQueryData(queryKeys.currentUser)).toMatchObject({ id: "example-user" }));
  return { ...view, client };
}

it("a central non-owned settings 401 clears provider identity and every live owner without a stream error", async () => {
  let reject = false;
  const f = sessionFixture((url) => reject && url.pathname.endsWith("/environments/0/settings") ? response({ code: "http.error" }, 401) : undefined);
  const view = await mountedIdentity(f.adapter);
  vi.useFakeTimers();
  const frames = new Map<number, FrameRequestCallback>(); let frameId = 0;
  vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => { frames.set(++frameId, callback); return frameId; });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
  const snapshots: MetricsSnapshot[] = [];
  const lines = vi.fn();
  const offMetrics = f.adapter.subscribeMetrics((value) => snapshots.push(value));
  const offLogs = f.adapter.subscribeLogs("0~example", lines);
  await act(async () => { await vi.advanceTimersByTimeAsync(0); });
  const stats = f.sockets.find(({ url }) => url.includes("/stats"))!.socket;
  const log = f.sockets.find(({ url }) => url.includes("/logs"))!.socket;
  f.sample(stats); log.send(wireLog(1, "private tail"));
  for (const callback of frames.values()) callback(0); frames.clear();
  expect(snapshots.at(-1)?.hosts["0"]?.cpuHistory).toEqual([25]);
  expect(lines.mock.lastCall?.[0][0]?.message).toBe("private tail");
  const oldSource = f.sources[0]!;
  const oldError = oldSource.onerror!, oldMessage = stats.onmessage!;
  // Leave a log reconnect and a metrics poll pending when the unrelated request rejects.
  log.disconnect();
  await act(async () => { await vi.advanceTimersByTimeAsync(0); });
  expect(vi.getTimerCount()).toBeGreaterThan(0);
  const reset = vi.fn(); const offReset = identityLifetime(f.adapter).listen(reset);
  view.client.setQueryData(queryKeys.events, ["private event"]);
  reject = true;
  await act(async () => { await f.adapter.getSetupStatus().catch(() => undefined); });
  expect(identityLifetime(f.adapter).active()).toBe(false);
  expect(view.client.getQueryData(queryKeys.currentUser)).toBeNull();
  expect(view.client.getQueryData(queryKeys.events)).toBeUndefined();
  expect(reset).toHaveBeenCalledTimes(1);
  expect(oldSource.close).toHaveBeenCalled(); expect(stats.close).toHaveBeenCalled(); expect(log.close).toHaveBeenCalled();
  expect(snapshots.at(-1)?.hosts).toEqual({}); expect(snapshots.at(-1)?.containers).toEqual({});
  expect(lines.mock.lastCall?.[0]).toEqual([]); expect(frames.size).toBe(0);
  const requests = f.calls.length, sockets = f.sockets.length, sources = f.sources.length;
  oldError(new Event("error")); oldMessage(new MessageEvent("message", { data: JSON.stringify({ cpuUsage: 99 }) }));
  await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
  expect(f.calls).toHaveLength(requests); expect(f.sockets).toHaveLength(sockets); expect(f.sources).toHaveLength(sources);
  expect(snapshots.at(-1)?.hosts).toEqual({});
  offReset(); offLogs(); offMetrics(); view.unmount(); view.client.clear();
});

it.each(["me", "activities", "runtime-runs", "capabilities", "environments"] as const)("owned %s rejection follows its specific authority exactly once", async (family) => {
  let rejected = false;
  const f = sessionFixture((url) => rejected && (family === "me" ? url.pathname.endsWith("/auth/me") : family === "runtime-runs" ? url.pathname.includes("/runtime-runs/") : url.pathname.endsWith(`/${family}`)) ? response({ code: "http.error" }, 401) : undefined);
  const view = await mountedIdentity(f.adapter);
  const reset = vi.fn(); const off = identityLifetime(f.adapter).listen(reset);
  rejected = true;
  if (family === "me") await act(async () => { await view.client.refetchQueries({ queryKey: queryKeys.currentUser }); });
  else if (family === "runtime-runs") await act(async () => { await f.adapter.getRuntimeRun!("0", "example-run").catch(() => undefined); });
  else {
    if (family === "capabilities") {
      const descriptor = await f.adapter.getCapability("listHosts");
      expect(descriptor).toMatchObject({ available: false, disabledReasonCode: "capability.unavailable" });
      expect(identityLifetime(f.adapter).active()).toBe(true); expect(reset).not.toHaveBeenCalled();
    }
    if (family === "environments") {
      await f.adapter.listHosts().catch(() => undefined);
      expect(identityLifetime(f.adapter).active()).toBe(true); expect(reset).not.toHaveBeenCalled();
    }
    await act(async () => { await f.adapter.listActions().catch(() => undefined); });
  }
  expect(identityLifetime(f.adapter).active()).toBe(false);
  expect(view.client.getQueryData(queryKeys.currentUser)).toBeNull();
  expect(reset).toHaveBeenCalledTimes(1);
  off(); view.unmount(); view.client.clear();
});

it("owned rejected credentials stay inline without clearing a valid provider identity", async () => {
  const f = sessionFixture(); const view = await mountedIdentity(f.adapter);
  const reset = vi.fn(); const off = identityLifetime(f.adapter).listen(reset);
  f.state.credentialFailure = true;
  await expect(f.adapter.login("operator", "wrong-password")).rejects.toMatchObject({ outcome: { status: 401 } });
  await expect(f.adapter.changePassword("wrong-password", "example-password")).rejects.toMatchObject({ outcome: { status: 401 } });
  expect(reset).not.toHaveBeenCalled(); expect(identityLifetime(f.adapter).active()).toBe(true);
  expect(view.client.getQueryData(queryKeys.currentUser)).toMatchObject({ id: f.user.id });
  expect(f.sources[0]!.close).not.toHaveBeenCalled();
  off(); view.unmount(); view.client.clear();
});

it("a delayed non-owned 401 from the lost generation cannot clear a newly established provider identity", async () => {
  const held = deferred<Response>(); const started = deferred<void>(); let reject = false;
  const f = sessionFixture((url) => {
    if (reject && url.pathname.endsWith("/environments/0/settings")) { started.resolve(); return held.promise; }
  });
  const view = await mountedIdentity(f.adapter);
  reject = true;
  const pending = f.adapter.getSetupStatus().catch((error: unknown) => error);
  await started.promise;
  await act(async () => {
    await f.adapter.logout();
    const user = await f.adapter.login("operator", "example-password");
    view.client.setQueryData(queryKeys.currentUser, user);
  });
  const version = identityLifetime(f.adapter).version();
  await act(async () => { held.resolve(response({}, 401)); await pending; });
  expect(identityLifetime(f.adapter).active()).toBe(true); expect(identityLifetime(f.adapter).version()).toBe(version);
  expect(view.client.getQueryData(queryKeys.currentUser)).toMatchObject({ id: f.user.id });
  view.unmount(); view.client.clear();
});

it("forwarded 502 agent.invalid-token keeps the hub identity and healthy inventory observations", async () => {
  const resources = resourceFixture();
  let rejected = false;
  const f = sessionFixture((url) => {
    if (url.pathname.includes("/auth/")) return undefined;
    if (rejected && url.pathname.startsWith("/api/environments/second/")) return response({ code: "agent.invalid-token" }, 502);
    return resources.fetcher(url);
  });
  const view = await mountedIdentity(f.adapter);
  const initial = await f.adapter.listStacks();
  expect(initial.map(({ id }) => id)).toEqual(["first~project", "second~project"]);
  const healthy = initial.find(({ hostId }) => hostId === "first")!;
  const reset = vi.fn(); const off = identityLifetime(f.adapter).listen(reset);
  rejected = true;
  // A real environment notification invalidates the shared inventory reads.
  await act(async () => { f.sources[0]!.environments(); });
  const next = await f.adapter.listStacks();
  expect(next).toEqual([healthy]);
  const hosts = await f.adapter.listHosts();
  expect(hosts.find(({ id }) => id === "first")?.inventory).toEqual(healthy.inventory);
  expect(hosts.find(({ id }) => id === "second")?.inventory).toMatchObject({ stale: true, reason: "agent.invalid-token" });
  expect(identityLifetime(f.adapter).active()).toBe(true); expect(reset).not.toHaveBeenCalled();
  expect(view.client.getQueryData(queryKeys.currentUser)).toMatchObject({ id: f.user.id });
  expect(f.sources[0]!.close).not.toHaveBeenCalled();
  off(); view.unmount(); view.client.clear();
});

it("settled query cancellations from loss and establishment cannot null the replacement identity", async () => {
  const f = sessionFixture(); const view = await mountedIdentity(f.adapter);
  const cancellations: ReturnType<typeof deferred<void>>[] = [];
  const cancel = view.client.cancelQueries.bind(view.client);
  const intercept = vi.spyOn(view.client, "cancelQueries").mockImplementation((...args) => {
    const pending = cancel(...args), barrier = deferred<void>();
    cancellations.push(barrier);
    return pending.then(() => barrier.promise);
  });
  await act(async () => { await f.adapter.logout(); });
  expect(view.client.getQueryData(queryKeys.currentUser)).toBeNull();
  await act(async () => {
    const user = await f.adapter.login("operator", "example-password");
    view.client.setQueryData(queryKeys.currentUser, user);
  });
  expect(cancellations).toHaveLength(2);
  for (const barrier of cancellations) {
    await act(async () => { barrier.resolve(); });
    expect(view.client.getQueryData(queryKeys.currentUser)).toMatchObject({ id: f.user.id });
    expect(identityLifetime(f.adapter).active()).toBe(true);
  }
  intercept.mockRestore(); view.unmount(); view.client.clear();
});

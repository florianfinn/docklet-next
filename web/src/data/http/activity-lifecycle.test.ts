// Source: docklet-next/issues/102
// Revision: 28f968e594b730c4960f57296abf7c44158c28c5
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original valid-session stream and query cancellation regressions.

import { CancelledError, QueryCache, QueryClient } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { createActivityReader } from "./activity-reader";
import { activityFixture, deferred, detail, type Detail } from "./activity-reader.test-support";

it.each(["initial discovery", "different identity", "login", "password change"])("%s reconnects persistent activity and change listeners with fresh callbacks", async (transition) => {
  const f = activityFixture(); const runs = vi.fn(); const changes = vi.fn();
  if (transition !== "initial discovery") await f.adapter.getCurrentUser();
  const stops = [f.adapter.subscribeActions(runs), f.adapter.subscribeChanges(changes)];
  try {
    const old = f.sources[0]!; old.open();
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(2));
    if (transition === "initial discovery") await f.adapter.getCurrentUser();
    if (transition === "different identity") { f.state.user.id = "another-operator"; await f.adapter.getCurrentUser(); }
    if (transition === "login") await f.adapter.login("operator", "Example-password1!");
    if (transition === "password change") await f.adapter.changePassword("Example-password1!", "New-example-password2!");
    expect(old.close).toHaveBeenCalledTimes(1);
    expect(f.sources).toHaveLength(2);
    const before = f.requests.length; const calls = changes.mock.calls.length;
    old.open(); old.send("run-good", "good"); old.environments(); old.failClosed();
    expect(f.requests).toHaveLength(before); expect(changes).toHaveBeenCalledTimes(calls);
    f.state.details["run-good"]!.activity.status = "queued";
    f.sources[1]!.open();
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0].find((r: { id: string }) => r.id === "run-good")?.status).toBe("queued"));
    expect(changes.mock.calls.length).toBeGreaterThan(calls);
    const current = f.sources[1]!;
    await f.adapter.getCurrentUser();
    expect(current.close).not.toHaveBeenCalled(); expect(f.sources).toHaveLength(2);
    const targetedBefore = f.requests.length;
    f.state.details["run-good"]!.activity.status = "running"; current.send("run-good", "good");
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0].find((r: { id: string }) => r.id === "run-good")?.status).toBe("running"));
    expect(f.requests.length - targetedBefore).toBe(1);
    expect(changes.mock.lastCall?.[0]).toEqual(["events"]);
  } finally { stops.forEach((stop) => stop()); }
});
it.each(["logout", "rejected identity", "activity 401"])("%s suspends persistent listeners without unauthorized reconnect and login resumes", async (boundary) => {
  const f = activityFixture(); await f.adapter.getCurrentUser();
  const runs = vi.fn(); const changes = vi.fn();
  const stops = [f.adapter.subscribeActions(runs), f.adapter.subscribeChanges(changes)];
  try {
    const old = f.sources[0]!; old.open();
    await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(2));
    if (boundary === "logout") await f.adapter.logout();
    if (boundary === "rejected identity") { f.state.authStatus = 401; await expect(f.adapter.getCurrentUser()).rejects.toMatchObject({ outcome: { status: 401 } }); }
    if (boundary === "activity 401") { f.state.failure = "page401"; await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } }); }
    expect(old.close).toHaveBeenCalledTimes(1);
    expect(runs.mock.lastCall?.[0]).toEqual([]);
    const count = f.requests.length; const publications = runs.mock.calls.length;
    vi.useFakeTimers();
    old.open(); old.send(); old.environments(); old.failClosed();
    const off = f.adapter.subscribeChanges(vi.fn()); stops.push(off);
    await vi.advanceTimersByTimeAsync(1000);
    expect(f.sources).toHaveLength(1); expect(f.requests).toHaveLength(count);
    expect(runs).toHaveBeenCalledTimes(publications);
    vi.useRealTimers();
    f.state.authStatus = 200; f.state.failure = "";
    await f.adapter.login("operator", "Example-password1!");
    expect(f.sources).toHaveLength(2);
    f.sources[1]!.open(); await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(2));
  } finally { vi.useRealTimers(); stops.forEach((stop) => stop()); }
});
it("invalid password credentials leave the valid session stream and observations intact", async () => {
  const f = activityFixture(); await f.adapter.getCurrentUser();
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  try {
    f.sources[0]!.open(); await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(2));
    f.state.authStatus = 401;
    await expect(f.adapter.changePassword("wrong-example-password", "New-example-password2!")).rejects.toMatchObject({ outcome: { status: 401 } });
    expect(f.sources).toHaveLength(1); expect(f.sources[0]!.close).not.toHaveBeenCalled();
    expect(runs.mock.lastCall?.[0]).toHaveLength(2);
  } finally { stop(); }
});
it.each(["logout", "unsubscribe"])("%s rejects an invalidated actions query without a successful empty cache entry", async (boundary) => {
  const f = activityFixture(); const runs = vi.fn(); const health = vi.fn();
  const stop = f.adapter.subscribeActions(runs); const off = f.adapter.subscribeActionReadHealth!(health);
  const success = vi.fn(); const errors = vi.fn();
  const client = new QueryClient({ queryCache: new QueryCache({ onSuccess: success }), defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
  window.addEventListener("docklet:adapter-error", errors);
  try {
    await f.adapter.listActions();
    const held = deferred<Response>(); const reached = deferred<Response>();
    f.state.hold = async (path, response) => {
      if (path.endsWith("/bad/activities/run-bad")) { reached.resolve(response); return held.promise; }
      return response;
    };
    const queryKey = ["actions"];
    const pending = client.fetchQuery({ queryKey, queryFn: () => f.adapter.listActions() });
    const rejection = expect(pending).rejects.toBeInstanceOf(CancelledError);
    const captured = await reached.promise;
    if (boundary === "logout") await f.adapter.logout(); else stop();
    const calls = runs.mock.calls.length; const healthCalls = health.mock.calls.length;
    held.resolve(captured); await rejection;
    expect(success).not.toHaveBeenCalled();
    expect(client.getQueryData(queryKey)).toBeUndefined();
    expect(client.getQueryState(queryKey)?.status).toBe("error");
    expect(client.getQueryState(queryKey)?.fetchStatus).toBe("idle");
    expect(runs).toHaveBeenCalledTimes(calls); expect(health).toHaveBeenCalledTimes(healthCalls);
    expect(errors).not.toHaveBeenCalled();
  } finally { stop(); off(); client.clear(); window.removeEventListener("docklet:adapter-error", errors); }
});
it.each(["reset", "invalidate"] as const)("targeted completion rejects cancellation after %s without publishing or fallback reads", async (boundary) => {
  const held = deferred<Detail>(); const publish = vi.fn(); const hosts = vi.fn(async () => ["good"]);
  const readDetail = vi.fn(async () => detail("good"));
  const reader = createActivityReader({ hosts, page: async () => ({ items: [detail("good").activity], incomplete: false }), detail: readDetail, publish, unauthorized: vi.fn() });
  await reader.load();
  readDetail.mockReturnValueOnce(held.promise);
  const pending = reader.refreshOne("run-good", "good");
  const rejection = expect(pending).rejects.toBeInstanceOf(CancelledError);
  reader[boundary](); const count = publish.mock.calls.length;
  const terminal = detail("good"); terminal.activity.status = "success";
  held.resolve(terminal); await rejection;
  expect(publish).toHaveBeenCalledTimes(count); expect(hosts).toHaveBeenCalledTimes(1);
});
it.each(["full", "targeted"])("%s subscription work silently discards cancellation on logout", async (kind) => {
  const f = activityFixture(); const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  const errors = vi.fn(); window.addEventListener("docklet:adapter-error", errors);
  try {
    f.sources[0]!.open(); await vi.waitFor(() => expect(runs.mock.lastCall?.[0]).toHaveLength(2));
    const held = deferred<Response>(); const reached = deferred<Response>();
    f.state.hold = async (path, response) => {
      if (path.endsWith("/good/activities/run-good")) { reached.resolve(response); return held.promise; }
      return response;
    };
    if (kind === "full") f.sources[0]!.open(); else f.sources[0]!.send("run-good", "good");
    const response = await reached.promise;
    await f.adapter.logout();
    const publications = runs.mock.calls.length;
    vi.useFakeTimers();
    held.resolve(response);
    // Drain the bounded promise pipeline and reconnect timers under the test clock.
    await vi.runAllTimersAsync();
    expect(errors).not.toHaveBeenCalled();
    expect(runs).toHaveBeenCalledTimes(publications);
    expect(f.sources).toHaveLength(1);
  } finally { vi.useRealTimers(); stop(); window.removeEventListener("docklet:adapter-error", errors); }
});

it.each(["logout", "password change", "activity 401"])("a held identity discovery before %s cannot reconnect or replace the new generation", async (boundary) => {
  const f = activityFixture();
  await f.adapter.getAuthMode(); // Complete CSRF bootstrap before holding an independent identity read.
  const runs = vi.fn(); const stop = f.adapter.subscribeActions(runs);
  const held = deferred<Response>(); const reached = deferred<Response>();
  try {
    f.state.hold = async (path, response) => {
      if (path.endsWith("/auth/me")) { reached.resolve(response); return held.promise; }
      return response;
    };
    const pending = f.adapter.getCurrentUser();
    const rejection = expect(pending).rejects.toBeInstanceOf(CancelledError);
    const response = await reached.promise;
    f.state.hold = undefined;
    if (boundary === "logout") await f.adapter.logout();
    else if (boundary === "activity 401") {
      f.state.failure = "page401";
      await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } });
    } else await f.adapter.changePassword("Example-password1!", "New-example-password2!");
    const sources = f.sources.length; const publications = runs.mock.calls.length;
    held.resolve(response); await rejection;
    expect(f.sources).toHaveLength(sources); expect(runs).toHaveBeenCalledTimes(publications);
    if (boundary === "logout") expect(f.sources).toHaveLength(1);
  } finally { stop(); }
});
it("a confirmed same identity resumes a stream suspended by an activity identity rejection", async () => {
  const f = activityFixture(); await f.adapter.getCurrentUser();
  const stop = f.adapter.subscribeActions(vi.fn());
  try {
    await f.adapter.listActions();
    f.state.failure = "page401";
    await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } });
    expect(f.sources[0]!.close).toHaveBeenCalledTimes(1);
    f.state.failure = "";
    await f.adapter.getCurrentUser();
    expect(f.sources).toHaveLength(2);
  } finally { stop(); }
});

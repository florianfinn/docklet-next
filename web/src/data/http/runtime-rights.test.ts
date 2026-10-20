// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { expect, it } from "vitest";
import { RIGHTS_MAX_AGE_MS } from "./runtime-commands";
import { deferred } from "./activity-reader.test-support";
import { envelope, response, RUN_ID, runtimeDetail, runtimeFixture, snapshot } from "./runtime-commands.test-support";

const user = (permissions = ["activities:read", "activities:cancel"]) => envelope({ isGlobalAdmin: true, permissionsByEnv: { global: permissions } });
const authReads = (f: ReturnType<typeof runtimeFixture>) => f.calls.filter(({ url }) => url.pathname.endsWith("/auth/me"));
const mutations = (f: ReturnType<typeof runtimeFixture>) => f.calls.filter(({ init }) => init.method === "POST");

it.each(["cancel", "release"] as const)("503 rights read is evicted and next %s reaches the real route after recovery", async (action) => {
  const f = runtimeFixture(); let reads = 0;
  f.state.run = snapshot("edge"); f.state.recovery.unknownReason = "transport";
  f.state.auth = (url) => url.pathname.endsWith("/me") ? ++reads === 1 ? response({}, 503) : user() : envelope({});
  const first = await f.adapter.getRuntimeRun!("edge", RUN_ID);
  expect(first).toMatchObject({ runtime: { permissionsUnavailable: true, unknownReason: "transport" } });
  expect(first).not.toHaveProperty("runtime.canCancel", true);
  f.state.read = (_url, init) => init.method === "POST" ? envelope({ ...f.state.run, ...(action === "release" ? { status: "failed" } : { cancelRequested: true }) }) : envelope(runtimeDetail(f.state.run));
  const result = action === "cancel" ? f.adapter.cancelRuntimeRun!("edge", RUN_ID) : f.adapter.releaseRuntimeRun!("edge", RUN_ID, { reason: "Executor stopped and inspected", acknowledgeRisk: true });
  await expect(result).resolves.toMatchObject({ id: RUN_ID }); expect(reads).toBe(2);
  expect(mutations(f).map(({ url }) => url.pathname)).toEqual([`/api/environments/edge/runtime-runs/${RUN_ID}/${action}`]);
});
it.each(["cancel", "release"] as const)("unavailable rights are explicit on %s, never fabricated as a server403", async (action) => {
  const f = runtimeFixture(); f.state.auth = () => response({}, 503);
  const result = action === "cancel" ? f.adapter.cancelRuntimeRun!("edge", RUN_ID) : f.adapter.releaseRuntimeRun!("edge", RUN_ID, { reason: "Example inspection", acknowledgeRisk: true });
  await expect(result).rejects.toMatchObject({ outcome: { code: "runtime.permissions-unavailable", status: undefined } });
  expect(mutations(f)).toHaveLength(0);
});
it("successful grants expire within the bound and refresh a mid-session grant", async () => {
  const f = runtimeFixture(); f.state.permissions = [];
  expect((await f.helper.getRuntimeRun("0", RUN_ID)).runtime?.canCancel).toBe(false);
  f.state.permissions = ["activities:read", "activities:cancel"];
  f.clock.elapsed = RIGHTS_MAX_AGE_MS - 1;
  expect((await f.helper.getRuntimeRun("0", RUN_ID)).runtime?.canCancel).toBe(false); expect(authReads(f)).toHaveLength(1);
  f.clock.elapsed++;
  expect((await f.helper.getRuntimeRun("0", RUN_ID)).runtime?.canCancel).toBe(true); expect(authReads(f)).toHaveLength(2);
  await f.helper.cancelRuntimeRun("0", RUN_ID); expect(mutations(f)).toHaveLength(1);
});
it("concurrent detail and mutation share one successful rights read", async () => {
  const f = runtimeFixture(); await f.helper.getRuntimeRun("0", RUN_ID); f.clock.elapsed = RIGHTS_MAX_AGE_MS;
  const reached = deferred<void>(), release = deferred<void>();
  f.state.auth = async () => { reached.resolve(); await release.promise; return user(); };
  const a = f.helper.getRuntimeRun("0", RUN_ID); await reached.promise;
  const b = f.helper.cancelRuntimeRun("0", RUN_ID);
  release.resolve(); await Promise.all([a, b]);
  expect(authReads(f)).toHaveLength(2); expect(mutations(f)).toHaveLength(1);
});
it.each([503, 401])("late rights failure%s after reset cannot erase or suspend fresh successful rights", async (status) => {
  const f = runtimeFixture(); await f.helper.getRuntimeRun("0", RUN_ID); f.clock.elapsed = RIGHTS_MAX_AGE_MS;
  const reached = deferred<void>(), release = deferred<void>(); let old = true;
  f.state.auth = async () => { if (old) { old = false; reached.resolve(); await release.promise; return response({}, status); } return user(); };
  const prior = f.helper.getRuntimeRun("0", RUN_ID).catch((error: unknown) => error); await reached.promise;
  f.helper.reset(); await f.helper.getRuntimeRun("0", RUN_ID); release.resolve();
  expect(await prior).toMatchObject({ name: "Error" });
  await f.helper.cancelRuntimeRun("0", RUN_ID);
  expect(authReads(f)).toHaveLength(3); expect(mutations(f)).toHaveLength(1); expect(f.helper.retained()).toHaveLength(1); expect(f.unauthorized).not.toHaveBeenCalled();
});
it("real server403 remains authoritative after a cached grant", async () => {
  const f = runtimeFixture(); await f.adapter.getRuntimeRun!("0", RUN_ID);
  f.state.read = () => response({ detail: "Example denied" }, 403);
  await expect(f.adapter.cancelRuntimeRun!("0", RUN_ID)).rejects.toMatchObject({ outcome: { status: 403, code: "http.error" } });
  expect(mutations(f)).toHaveLength(1); expect(authReads(f)).toHaveLength(1);
});
it("rights401 suspends the actual adapter identity and clears published runs", async () => {
  const f = runtimeFixture(); const seen: unknown[][] = [];
  const off = f.adapter.subscribeActions((rows) => seen.push(rows));
  try {
    f.state.auth = () => response({}, 401);
    await expect(f.adapter.getRuntimeRun!("0", RUN_ID)).rejects.toBeTruthy();
    expect(seen.at(-1)).toEqual([]); expect(f.source.close).toHaveBeenCalled(); expect(mutations(f)).toHaveLength(0);
  } finally { off(); }
});

it.each([null, [], "unavailable", { global: [42] }].map((permissionsByEnv) => ({ permissionsByEnv })))("malformed permission data$permissionsByEnv is unavailable and is not cached", async ({ permissionsByEnv }) => {
  const f = runtimeFixture(); f.state.auth = () => envelope({ isGlobalAdmin: true, permissionsByEnv });
  await expect(f.helper.cancelRuntimeRun("0", RUN_ID)).rejects.toMatchObject({ outcome: { code: "runtime.permissions-unavailable", status: undefined } });
  expect(mutations(f)).toHaveLength(0); f.state.auth = undefined;
  await expect(f.helper.cancelRuntimeRun("0", RUN_ID)).resolves.toMatchObject({ id: RUN_ID }); expect(authReads(f)).toHaveLength(2); expect(mutations(f)).toHaveLength(1);
});

it.each([false, true])("an empty environment permission slice preserves global grant=%s without changing policy", async (granted) => {
  const f = runtimeFixture(); f.state.run = snapshot("edge");
  f.state.auth = () => envelope({ isGlobalAdmin: true, permissionsByEnv: { edge: null, global: granted ? ["activities:read", "activities:cancel"] : [] } });
  const run = await f.helper.getRuntimeRun("edge", RUN_ID);
  expect(run.runtime).toMatchObject({ canCancel: granted, canRelease: granted });
  expect(run.runtime?.permissionsUnavailable).toBeUndefined();
  if (granted) await expect(f.helper.cancelRuntimeRun("edge", RUN_ID)).resolves.toMatchObject({ id: RUN_ID });
  else await expect(f.helper.cancelRuntimeRun("edge", RUN_ID)).rejects.toMatchObject({ outcome: { status: 403 } });
  expect(authReads(f)).toHaveLength(1); expect(mutations(f)).toHaveLength(granted ? 1 : 0);
});

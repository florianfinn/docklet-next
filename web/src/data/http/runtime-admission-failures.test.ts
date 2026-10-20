// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { afterEach, expect, it, vi } from "vitest";
import { deferred } from "./activity-reader.test-support";
import { COMMAND_RETRY_MS, createRuntimeCommands } from "./runtime-commands";
import { createCsrfClient } from "./csrf-client";
import { envelope, response, RUN_ID, runtimeFixture, snapshot } from "./runtime-commands.test-support";

const bare = (status = 200) => new Response("{}", { status, headers: { "Content-Type": "application/json" } });
afterEach(() => vi.useRealTimers());

it.each(["503", "missing", "transport"])("zeroPOST token bootstrap %s is pre-admission and allows a later deliberate intent", async (fault) => {
  const f = runtimeFixture();
  f.state.auth = () => { if (fault === "transport") throw new TypeError("Example bootstrap loss"); return bare(fault === "503" ? 503 : 200); };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { kind: "error", code: fault === "transport" ? "runtime.pre-admission" : fault === "503" ? "http.error" : "http.csrf-token-missing" } });
  expect(f.commands()).toHaveLength(0); expect(f.published).toEqual([]);
  f.state.auth = undefined;
  expect(await f.helper.runContainerCommand("0~target", "start")).toBe(RUN_ID); expect(f.commands()).toHaveLength(1);
});
it("raw TypeError from an unwrapped CSRF token preflight still proves zeroPOST", async () => {
  let recover = false; const posts: string[] = [];
  const client = createCsrfClient("/api", async (_url, init) => {
    if (init?.method === "POST") { posts.push("POST"); return envelope({ activityId: RUN_ID, code: "run.started", activity: snapshot() }, 202); }
    if (!recover) throw new TypeError("Example bootstrap loss");
    return envelope({});
  });
  const helper = createRuntimeCommands({ baseUrl: "/api", client, capability: async (operationId, resourceRef) => ({ operationId, resourceRef, available: true }), publish() {} });
  await expect(helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.pre-admission" } });
  expect(posts).toEqual([]); recover = true;
  expect(await helper.runContainerCommand("0~target", "start")).toBe(RUN_ID); expect(posts).toEqual(["POST"]);
});
it.each(["503", "missing", "transport"])("definitive CSRF403 followed by %s token failure does not hold a never-admitted target", async (fault) => {
  const f = runtimeFixture(); let bootstraps = 0;
  f.state.auth = () => { if (++bootstraps === 1) return envelope({}); if (fault === "transport") throw new TypeError("Example refresh loss"); return bare(fault === "503" ? 503 : 200); };
  f.state.command = () => response({ code: "csrf.invalid" }, 403);
  const result = await f.helper.runContainerCommand("0~target", "start").catch((error: unknown) => error);
  expect(result).toMatchObject({ outcome: { kind: "error", code: fault === "transport" ? "runtime.pre-admission" : fault === "503" ? "http.error" : "http.csrf-token-missing" } });
  expect(f.commands()).toHaveLength(1); expect(f.published).toEqual([]);
  f.state.auth = undefined; f.state.command = undefined;
  expect(await f.helper.runContainerCommand("0~target", "start")).toBe(RUN_ID); expect(f.commands()).toHaveLength(2);
});
it.each(["bootstrap", "denial", "malformed", "loss"])("a possibly accepted POST followed by %s retains ambiguity and the exact key", async (fault) => {
  const f = runtimeFixture(); let bootstraps = 0;
  f.state.auth = () => ++bootstraps === 1 ? envelope({}) : bare(503);
  f.state.command = () => {
    if (f.commands().length === 1 || fault === "loss") throw new TypeError("Example accepted response loss");
    return fault === "bootstrap" ? response({ code: "csrf.invalid" }, 403) : fault === "denial" ? response({}, 403) : bare(202);
  };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  await expect(f.helper.runContainerCommand("0~target", "restart")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(f.commands()).toHaveLength(2); expect(new Set(f.commands().map(({ init }) => new Headers(init.headers).get("Idempotency-Key"))).size).toBe(1);
  expect(f.published).toEqual([]);
});
it("bootstrap exceeding the monotonic budget sends noPOST and permits later deliberate intent", async () => {
  const f = runtimeFixture(); f.state.auth = () => { f.clock.elapsed += COMMAND_RETRY_MS; return envelope({}); };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.pre-admission" } });
  expect(f.commands()).toHaveLength(0); f.state.auth = undefined;
  expect(await f.helper.runContainerCommand("0~target", "start")).toBe(RUN_ID); expect(f.commands()).toHaveLength(1);
});
it.each(["stack", "container"] as const)("%s admission with a mismatched snapshot host cannot publish links or allow replacement", async (kind) => {
  const f = runtimeFixture(); f.state.run = snapshot("other-host");
  const command = kind === "stack" ? f.helper.runStackCommand : f.helper.runContainerCommand;
  await expect(command("edge~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  await expect(command("edge~target", "restart")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(f.published).toEqual([]); expect(f.helper.retained()).toEqual([]); expect(f.commands()).toHaveLength(1);
  expect(f.calls.some(({ url }) => url.pathname.includes("/runtime-runs/"))).toBe(false);
});
it("detail with a mismatched snapshot host rejects without publication or wrong cancel routing", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("other-host");
  await expect(f.helper.getRuntimeRun("edge", RUN_ID)).rejects.toMatchObject({ outcome: { code: "runtime.invalid-response" } });
  expect(f.published).toEqual([]); expect(f.helper.retained()).toEqual([]); expect(f.calls.filter(({ init }) => init.method === "POST")).toEqual([]);
  f.state.run = snapshot("edge"); await f.helper.getRuntimeRun("edge", RUN_ID); await f.helper.cancelRuntimeRun("edge", RUN_ID);
  expect(f.calls.filter(({ init }) => init.method === "POST").map(({ url }) => url.pathname)).toEqual([`/api/environments/edge/runtime-runs/${RUN_ID}/cancel`]);
});

it.each(["before", "after"])("exhausted POST loss %s acceptance stays held with one UUID and no replacement", async (when) => {
  const f = runtimeFixture(); const effects = new Set<string>();
  f.state.command = (_url, init) => { if (when === "after") effects.add(new Headers(init.headers).get("Idempotency-Key")!); throw new TypeError("Example lost command response"); };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  await expect(f.helper.runContainerCommand("0~target", "restart")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(f.commands()).toHaveLength(2); expect(new Set(f.commands().map(({ init }) => new Headers(init.headers).get("Idempotency-Key"))).size).toBe(1);
  expect(effects.size).toBe(when === "before" ? 0 : 1); expect(f.published).toEqual([]);
});
it("identity reset during token preflight prevents the old command from sending", async () => {
  const f = runtimeFixture(); const reached = deferred<void>(), release = deferred<void>();
  f.state.auth = async () => { reached.resolve(); await release.promise; return envelope({}); };
  const old = f.helper.runContainerCommand("0~target", "start").catch((error: unknown) => error);
  await reached.promise; f.helper.reset(); release.resolve(); await old;
  expect(f.commands()).toHaveLength(0); expect(f.published).toEqual([]);
  f.state.auth = undefined; expect(await f.helper.runContainerCommand("0~target", "start")).toBe(RUN_ID);
});
it("late malformed response after reset cannot add an ambiguous hold to the fresh generation", async () => {
  const f = runtimeFixture(); const reached = deferred<void>(), release = deferred<void>();
  f.state.command = () => { const result = envelope({}); result.json = async () => { reached.resolve(); await release.promise; throw new SyntaxError("Example old malformed response"); }; return result; };
  const old = f.helper.runContainerCommand("0~target", "start").catch((error: unknown) => error);
  await reached.promise; f.helper.reset(); release.resolve(); await old;
  f.state.command = undefined; expect(await f.helper.runContainerCommand("0~target", "start")).toBe(RUN_ID); expect(f.commands()).toHaveLength(2);
});

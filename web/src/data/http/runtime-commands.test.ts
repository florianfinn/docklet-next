// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { afterEach, expect, it, vi } from "vitest";
import { COMMAND_RETRY_MS } from "./runtime-commands";
import { deferred } from "./activity-reader.test-support";
import { envelope, response, RUN_ID, runtimeFixture, snapshot } from "./runtime-commands.test-support";

afterEach(() => vi.useRealTimers());
const operations = [
  ["stack", "start", "up"], ["stack", "stop", "down"], ["stack", "restart", "restart"], ["stack", "redeploy", "redeploy"],
  ["container", "start", "start"], ["container", "stop", "stop"], ["container", "restart", "restart"],
] as const;
it.each(["0", "direct", "edge"].flatMap((host) => operations.map(([kind, command, suffix]) => ({ host, kind, command, suffix }))))("$host $kind $command uses existing async route and stable canonical identity", async ({ host, kind, command, suffix }) => {
  const f = runtimeFixture(); f.state.run = snapshot(host, { resourceType: kind === "stack" ? "project" : "container", type: kind === "stack" ? "project_deploy" : "container_start" });
  const id = kind === "stack" ? await f.adapter.runStackCommand(`${host}~target /?#%`, command) : await f.adapter.runContainerCommand(`${host}~target /?#%`, command as "start");
  expect(id).toBe(RUN_ID); expect(f.commands()).toHaveLength(1);
  const request = f.commands()[0]!;
  expect(request.url.pathname).toBe(`/api/environments/${host}/${kind === "stack" ? "projects" : "containers"}/target%20%2F%3F%23%25/${suffix}`);
  const headers = new Headers(request.init.headers);
  expect(headers.get("Prefer")).toBe("respond-async"); expect(headers.get("Idempotency-Key")).toMatch(/^[0-9a-f-]{36}$/);
  expect(headers.get("X-CSRF-Token")).toBeTruthy(); expect(request.init.credentials).toBe("same-origin"); expect(request.init.body).toBeUndefined();
  const run = await f.adapter.getRuntimeRun!(host, id);
  expect(run).toMatchObject({ id, hostId: host, status: "running", outcomeState: host === "0" ? "known" : "unknown", cancelRequested: false });
  expect(kind === "stack" ? run.stackId : run.containerId).toBe(`${host}~canonical-id`);
  expect(f.commands()).toHaveLength(1);
});

it.each(["running", "success", "failed", "cancelled"] as const)("200 join/retry returns the same %s identity without claiming admission is success", async (status) => {
  const f = runtimeFixture(); f.state.run = snapshot("0", { status });
  f.state.command = () => envelope({ activityId: RUN_ID, code: "run.joined", activity: f.state.run });
  expect(await f.adapter.runContainerCommand("0~target", "start")).toBe(RUN_ID);
  expect(f.commands()).toHaveLength(1);
  expect((await f.adapter.getRuntimeRun!("0", RUN_ID)).status).toBe(status === "success" ? "succeeded" : status);
});

it.each([400, 401, 403, 404, 410, 503])("HTTP %s refuses with no fabricated run or retry", async (status) => {
  const f = runtimeFixture(); f.state.command = () => response({ success: false, error: { code: "runtime.target-unavailable" } }, status);
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { kind: "error", status } });
  expect(f.published).toEqual([]); expect(f.commands()).toHaveLength(1);
});

it.each(["run.busy", "run.cancelling", "run.request-mismatch"].flatMap((code) => [false, true].map((withID) => ({ code, withID }))))("409 $code authorized ID=$withID never creates work or discloses mismatch ID", async ({ code, withID }) => {
  const f = runtimeFixture(); f.state.command = () => response({ success: false, data: { code, ...(withID ? { activeRunId: RUN_ID } : {}) } }, 409);
  const failure = await f.helper.runContainerCommand("0~target", "start").catch((error: unknown) => error);
  expect(failure).toMatchObject({ outcome: { code, kind: code === "run.request-mismatch" || withID ? "conflict" : "error" } });
  if (code === "run.request-mismatch" || !withID) expect(failure).not.toHaveProperty("outcome.activeRunId");
  expect(f.commands()).toHaveLength(1);
});

it.each(["json", "false", "empty", "different", "202joined", "200started", "status", "knowledge", "cancel"])("invalid successful %s response remains ambiguous and cannot create a replacement intent", async (fault) => {
  const f = runtimeFixture();
  f.state.command = () => fault === "json" ? new Response("{", { status: 202 }) : response({ success: fault !== "false", data: {
    code: fault === "202joined" ? "run.joined" : "run.started", activityId: fault === "empty" ? "" : RUN_ID,
    activity: { ...f.state.run, ...(fault === "different" ? { id: "another" } : {}), ...(fault === "status" ? { status: "unknown" } : {}), ...(fault === "knowledge" ? { outcomeState: undefined } : {}), ...(fault === "cancel" ? { cancelRequested: undefined } : {}) },
  } }, fault === "200started" ? 200 : 202);
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  await expect(f.helper.runContainerCommand("0~target", "restart")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(f.published).toEqual([]); expect(f.commands()).toHaveLength(1);
});

it.each(["before", "after"])("lost response %s server acceptance retries exactly the original key and performs one synthetic admission", async (when) => {
  const f = runtimeFixture(); const keys = new Set<string>(); let effects = 0;
  f.state.command = (_url, init) => {
    const key = new Headers(init.headers).get("Idempotency-Key")!;
    if (f.commands().length !== 1 || when === "after") { if (!keys.has(key)) { keys.add(key); effects++; } }
    if (f.commands().length === 1) throw new TypeError("Example response loss");
    return envelope({ activityId: RUN_ID, code: "run.joined", activity: f.state.run });
  };
  expect(await f.helper.runContainerCommand("0~target", "start")).toBe(RUN_ID);
  expect(f.commands()).toHaveLength(2); expect(effects).toBe(1);
  expect(new Set(f.commands().map(({ init }) => new Headers(init.headers).get("Idempotency-Key"))).size).toBe(1);
});

it("CSRF refresh preserves exact key, then a final response discards it for the next deliberate intent", async () => {
  const f = runtimeFixture(); f.state.command = () => f.commands().length === 1 ? response({ code: "csrf.invalid" }, 403) : envelope({ activityId: RUN_ID, code: "run.joined", activity: f.state.run });
  await f.helper.runContainerCommand("0~target", "start");
  await f.helper.runContainerCommand("0~target", "start");
  const keys = f.commands().map(({ init }) => new Headers(init.headers).get("Idempotency-Key"));
  expect(keys).toHaveLength(3); expect(keys[0]).toBe(keys[1]); expect(keys[2]).not.toBe(keys[1]);
});

it.each([COMMAND_RETRY_MS, COMMAND_RETRY_MS + 1])("elapsed %d prevents another POST after an ambiguous response; no timestamp header is sent", async (elapsed) => {
  const f = runtimeFixture(); f.state.command = () => { f.clock.elapsed = elapsed; throw new TypeError("Example loss"); };
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  expect(f.commands()).toHaveLength(1);
  await expect(f.helper.runContainerCommand("0~target", "start")).rejects.toBeTruthy(); expect(f.commands()).toHaveLength(1);
});

it("two transport failures exhaust the bounded retry with no timer replay or fabricated ID", async () => {
  vi.useFakeTimers(); const f = runtimeFixture(); f.state.command = () => { throw new TypeError("Example loss"); };
  await expect(f.helper.runStackCommand("0~target", "start")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  await vi.advanceTimersByTimeAsync(COMMAND_RETRY_MS * 2);
  expect(f.commands()).toHaveLength(2); expect(f.published).toEqual([]); expect(vi.getTimerCount()).toBe(0);
});

it("duplicate pending intent shares admission, and old-generation completion cannot publish", async () => {
  const f = runtimeFixture(); const reached = deferred<void>(), release = deferred<void>();
  f.state.command = async () => { reached.resolve(); await release.promise; return envelope({ activityId: RUN_ID, code: "run.started", activity: f.state.run }, 202); };
  const a = f.helper.runContainerCommand("0~target", "start"), b = f.helper.runContainerCommand("0~target", "start");
  const result = Promise.allSettled([a, b]); await reached.promise; f.helper.reset(); release.resolve();
  expect((await result).map((r) => r.status)).toEqual(["rejected", "rejected"]); expect(f.commands()).toHaveLength(1);
  expect(f.commands()[0]!.init.signal?.aborted).toBe(true); expect(f.published).toEqual([]);
});

it("encodes the host segment as well as the raw target without interpreting either as a route", async () => {
  const f = runtimeFixture(); const host = "direct /?#%"; f.state.run = snapshot(host);
  expect(await f.adapter.runContainerCommand(`${host}~raw /?#%`, "start")).toBe(RUN_ID);
  expect(f.commands()[0]!.url.pathname).toBe("/api/environments/direct%20%2F%3F%23%25/containers/raw%20%2F%3F%23%25/start");
  expect(f.commands()).toHaveLength(1);
});

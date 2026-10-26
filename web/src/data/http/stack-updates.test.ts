// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer-shaped HTTP fixtures and behavioural guards for issue #231.

import { describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { withRunGate, isRunBusy } from "../run-gate";
const contract = JSON.parse(readFileSync("../api/openapi.json", "utf8")) as { components: { schemas: { UpdaterAutoUpdatePolicy: { properties: { applyCode: { enum: string[] } } } } } };
import { createStackUpdates } from "./stack-updates";
import { baseline, points, policy, schedules, hints, checkResult } from "./update-decoders";
import { reason, updateReasons } from "./update-validation";
import { updateFixture, base, point, storedPolicy, schedule, systemHints, revision } from "./stack-updates.test-support";
import { deferred } from "./activity-reader.test-support";
import { response } from "./runtime-commands.test-support";
import { mapActivitySummary } from "./map-dto";
import { catalogs } from "@/app/i18n/catalog";

describe("update decoder boundaries", () => {
  it("covers the declared contract reason enum", () => expect([...updateReasons].sort()).toEqual([...contract.components.schemas.UpdaterAutoUpdatePolicy.properties.applyCode.enum].sort()));
  it.each(contract.components.schemas.UpdaterAutoUpdatePolicy.properties.applyCode.enum)("declares and localizes %s without raw fallback", (code) => {
    expect(reason(code)).toBe(code);
    const key = code.replaceAll(".", "_");
    for (const locale of ["de", "en-US"] as const) expect(typeof (catalogs[locale].images.errors as Record<string, string>)[key]).toBe("string");
    expect(reason("undeclared.raw-diagnostic")).toBe("http.error");
  });
  it.each([
    () => baseline({ ...base, available: "false" }), () => baseline({ ...base, savedRevision: "old" }),
    () => points([{ ...point, targetId: "replacement" }], "project"), () => points([{ ...point, volumeDataIncluded: true }], "project"),
    () => policy({ ...storedPolicy, mode: "scheduled" }, "direct", "project"), () => policy({ ...storedPolicy, environmentId: "other" }, "direct", "project"),
    () => policy({ ...storedPolicy, candidates: [{ ...storedPolicy.candidates[0], notificationState: "delivered" }] }, "direct", "project"),
    () => schedules({ ...schedule, timeZone: "Invalid/Zone" }, "direct"), () => schedules({ ...schedule, pollingEnabled: 1 }, "direct"),
    () => hints({ ...systemHints, images: [{ ...systemHints.images[0], candidateVersion: "2.1.0" }] }),
    () => hints({ ...systemHints, images: [{ ...systemHints.images[0], candidateVersion: "1.0.0" }] }),
  ])("rejects malformed/foreign data", (decode) => expect(decode).toThrow());
  it("keeps status channels, sending and null collections distinct", () => {
    expect(policy(storedPolicy, "direct", "project")).toMatchObject({ stale: true, applyCode: "update.config-drift", code: undefined, candidates: [{ notificationState: "sending" }] });
    expect(points(null, "project")).toEqual([]);
    expect(baseline({ ...base, available: false, code: "update.baseline-unavailable" })).toMatchObject({ available: false, savedRevision: revision });
    expect(hints(systemHints).images[1]).toMatchObject({ state: "pinned", currentVersion: "4.0.0" });
  });
});
it.each([false, true])("maps real hub-facing reads equally for Direct/Edge=%s", async (edge) => {
  const f = updateFixture(); f.state.isEdge = edge;
  expect(await f.adapter.getUpdateBaseline!("direct~project")).toEqual(base);
  expect(await f.adapter.listRestorePoints!("direct~project")).toEqual([point]);
  expect(await f.adapter.getAutoUpdatePolicy!("direct~project")).toEqual(storedPolicy);
  expect(await f.adapter.getUpdateSchedules!("direct")).toEqual(schedule);
  expect(await f.adapter.getSystemImageHints!("direct")).toEqual(systemHints);
  expect(f.writes()).toEqual([]);
});
it.each(["installStackUpdates", "getUpdateBaseline", "restoreRestorePoint", "listRestorePoints"])("missing prerequisite %s sends no lifecycle write", async (missing) => {
  const f = updateFixture(); f.state.denied.add(missing);
  const call = missing.includes("Restore") || missing === "restoreRestorePoint" ? () => f.adapter.restoreRestorePoint("direct~project", point.id, { expectedRevision: revision, replaceConfiguration: true }) : () => f.adapter.installStackUpdates("direct~project", { expectedRevision: revision });
  await expect(call()).rejects.toHaveProperty("outcome.kind", "not-available"); expect(f.writes()).toHaveLength(0);
});
it("mode-specific apply capability cannot borrow off/notify authority", async () => {
  const f = updateFixture(); f.state.denied.add("setAutoUpdateApply");
  await expect(f.adapter.saveAutoUpdatePolicy!("direct~project", "apply", 1)).rejects.toHaveProperty("outcome.kind", "not-available");
  expect((await f.adapter.saveAutoUpdatePolicy!("direct~project", "off", 1)).mode).toBe("off");
  expect(f.writes()).toHaveLength(1);
});
it("schedule aggregation preserves healthy hosts and missing next run", async () => {
  const f = updateFixture(); f.state.hostFailure = "other";
  const rows = await f.adapter.listSchedule(["direct", "other"]);
  expect(rows.filter((r) => r.hostId === "direct")).toHaveLength(2);
  expect(rows.find((r) => r.hostId === "direct")).toMatchObject({ enabled: false, nextRunAt: undefined });
  expect(rows.find((r) => r.hostId === "other")).toMatchObject({ unavailable: true });
  expect(f.calls.filter((c) => c.path.endsWith("/schedules"))).toHaveLength(2);
});
it("same-ref descriptor reads coalesce and independent dialogs share only that ref", async () => {
  const f = updateFixture();
  await Promise.all([f.adapter.getCapability("getUpdateBaseline", { kind: "stack", id: "direct~project" }), f.adapter.getCapability("installStackUpdates", { kind: "stack", id: "direct~project" })]);
  expect(f.calls.filter((c) => c.path.endsWith("/capabilities"))).toHaveLength(1);
});
it("late identity-owned read rejects after logout even when transport ignores abort", async () => {
  const f = updateFixture(), held = deferred<Response>(), started = deferred<void>();
  f.state.hold = (path) => { if (path.endsWith("/baseline")) { started.resolve(); return held.promise; } };
  const read = f.adapter.getUpdateBaseline!("direct~project"); const result = expect(read).rejects.toBeDefined();
  await started.promise; await f.adapter.logout(); held.resolve(response({ success: true, data: base })); await result;
});
it.each(["queued", "running", "failed"] as const)("cold %s summaries retain unknown and cancel intent", (status) => {
  const f = updateFixture();
  expect(mapActivitySummary({ ...f.run, status, outcomeState: "unknown", cancelRequested: true, recovery: { unknownReason: "transport" } })).toMatchObject({ status, kind: "update", stackId: "direct~project", outcomeState: "unknown", cancelRequested: true, runtime: { unknownReason: "transport" } });
});
it("unknown lifecycle code never includes the raw diagnostic", () => {
  const f = updateFixture(); const mapped = mapActivitySummary({ ...f.run, error: "private raw diagnostic", messageCode: "project.update" });
  expect(JSON.stringify(mapped)).not.toContain("private raw diagnostic"); expect(mapped.error?.code).toBe("common.commandErrors.failed");
});

it.each(["malformed", "lost", "proxy"])("uncertain %s admission holds intent without repeat", async (kind) => {
  const f = updateFixture();
  f.state.hold = (path, init) => {
    if (!path.endsWith("/update") || init.method !== "POST") return;
    return kind === "lost" ? Promise.reject(new TypeError("network")) : Promise.resolve(kind === "proxy" ? response({ code: "agent.unavailable" }, 502) : response({ success: true, data: { activityId: f.run.id, code: "unknown.acceptance", activity: f.run } }, 202));
  };
  await expect(f.adapter.installStackUpdates("direct~project", { expectedRevision: revision })).rejects.toHaveProperty("outcome.code", "runtime.admission-unknown");
  const count = f.writes().length;
  await expect(f.adapter.installStackUpdates("direct~project", { expectedRevision: revision })).rejects.toHaveProperty("outcome.code", "runtime.admission-unknown");
  expect(f.writes()).toHaveLength(count);
  expect(JSON.parse(String(f.writes()[0]!.init.body))).toEqual({ expectedRevision: revision });
});
it("checks whole-stack observations without creating a fabricated run", async () => {
  const f = updateFixture(); await expect(f.adapter.checkStackUpdates!("direct~project")).resolves.toBeUndefined();
  expect(f.writes()[0]!.path).toBe("/api/environments/direct/updater/projects/project/check");
});

it("manual check response validates counts and per-service observations", () => {
  expect(() => checkResult({ status: "has_update", hasUpdate: true, imageCount: "one", checkedImageCount: 1, imagesWithUpdates: 1, imagesNotPulled: 0, errorCount: 0 })).toThrow();
});
it("absent point services keep explicit absence instead of fabricated image IDs", () => {
  expect(points([{ ...point, sourceState: "partial-source", images: [{ service: "app", present: false, running: false, reference: "", imageId: "" }] }], "project")[0]!.images).toMatchObject([{ present: false, imageId: "" }]);
});
it("owner loss during capability read prevents a policy write before dispatch", async () => {
  const f = updateFixture(), held = deferred<Response>(), started = deferred<void>();
  await f.adapter.getAuthMode();
  const initial = f.fetcher.getMockImplementation()!;
  let hold = true;
  f.fetcher.mockImplementation(async (input, init) => {
    if (String(input).includes("/capabilities") && hold) { hold = false; const reply = await initial(input, init); started.resolve(); await held.promise; return reply; }
    return initial(input, init);
  });
  const save = f.adapter.saveAutoUpdatePolicy!("direct~project", "apply", 1); const result = expect(save).rejects.toBeDefined();
  await started.promise; await f.adapter.logout(); held.resolve(response({})); await result; expect(f.writes().filter((write) => write.path.includes("/policies/"))).toHaveLength(0);
});

it.each(["getAutoUpdatePolicy", "listSchedule"])("write prerequisites include %s read authority", async (missing) => {
  const f = updateFixture(); f.state.denied.add(missing);
  const action = missing === "getAutoUpdatePolicy" ? () => f.adapter.saveAutoUpdatePolicy!("direct~project", "off", 1) : () => f.adapter.saveUpdateSchedule!("direct", { expectedRevision: 1, pollingInterval: schedule.pollingInterval, autoUpdateInterval: schedule.autoUpdateInterval, pollingEnabled: true, autoUpdateEnabled: true });
  await expect(action()).rejects.toHaveProperty("outcome.kind", "not-available"); expect(f.writes()).toHaveLength(0);
});

it("owner loss during capability read fences the mapper before dispatch", async () => {
  let generation = 0; const held = deferred<void>(), started = deferred<void>();
  const fetcher = vi.fn(async () => response({ success: true, data: storedPolicy }));
  const api = createStackUpdates({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "test" }, generation: () => generation, changed: () => {}, capability: async (operationId, resourceRef) => { started.resolve(); await held.promise; return { operationId, resourceRef, available: true }; } });
  const save = api.saveAutoUpdatePolicy("direct~project", "apply", 1), assertion = expect(save).rejects.toBeDefined();
  await started.promise; ++generation; held.resolve(); await assertion;
  expect(fetcher).toHaveBeenCalledTimes(0);
});

it.each(["update", "restore"] as const)("a second %s with a new revision is refused rather than joined", async (kind) => {
  const f = updateFixture(), held = deferred<Response>(), started = deferred<void>(), adapter = withRunGate(f.adapter);
  f.state.hold = (path, init) => { if (path.endsWith(`/${kind}`) && init.method === "POST") { started.resolve(); return held.promise; } };
  const send = (expectedRevision: string) => kind === "update" ? adapter.installStackUpdates("direct~project", { expectedRevision }) : adapter.restoreRestorePoint("direct~project", point.id, { expectedRevision, replaceConfiguration: true });
  if (kind === "restore") f.run.type = "project_restore";
  const first = send(revision); await started.promise;
  const second = send("c".repeat(64)).catch((error: unknown) => error);
  held.resolve(response({ success: true, data: { code: "run.started", activityId: f.run.id, activity: f.run } }, 202)); await first;
  expect(isRunBusy(await second)).toBe(true); expect(f.writes()).toHaveLength(1);
});

it("an accepted snapshot for another stack is uncertain rather than success", async () => {
  const f = updateFixture();
  f.state.hold = (path, init) => path.endsWith("/update") && init.method === "POST" ? Promise.resolve(response({ success: true, data: { code: "run.started", activityId: f.run.id, activity: { ...f.run, resourceId: "another-project" } } }, 202)) : undefined;
  await expect(f.adapter.installStackUpdates("direct~project", { expectedRevision: revision })).rejects.toHaveProperty("outcome.code", "runtime.admission-unknown");
});

it("a malformed lifecycle timestamp is not an accepted admission", async () => {
  const f = updateFixture();
  f.state.hold = (path, init) => path.endsWith("/update") && init.method === "POST" ? Promise.resolve(response({ success: true, data: { code: "run.started", activityId: f.run.id, activity: { ...f.run, startedAt: "not-a-timestamp" } } }, 202)) : undefined;
  await expect(f.adapter.installStackUpdates("direct~project", { expectedRevision: revision })).rejects.toHaveProperty("outcome.code", "runtime.admission-unknown");
});

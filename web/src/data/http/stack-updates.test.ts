// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer-shaped HTTP fixtures and behavioural guards for issue #231. M24 S6: observed recovery and update UI follow-ups.

import { describe, expect, it, vi } from "vitest";
import { readFileSync } from "node:fs";
import { withRunGate, isRunBusy } from "../run-gate";
const contract = JSON.parse(readFileSync("../api/openapi.json", "utf8")) as { components: { schemas: { UpdaterAutoUpdatePolicy: { properties: { applyCode: { enum: string[] } } } } } };
import { createStackUpdates } from "./stack-updates";
import { baseline, points, policy, window, hints, checkResult, compareVersion } from "./update-decoders";
import { reason, updateReasons } from "./update-validation";
import { updateFixture, base, point, storedPolicy, systemHints, revision, targetIdentity } from "./stack-updates.test-support";
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
  it("retains host.restarting in decoded policy status channels", () => {
    const code = "host.restarting";
    expect(policy({ ...storedPolicy, code, applyCode: code, bookkeepingCode: code }, "direct", "project")).toMatchObject({ code, applyCode: code, bookkeepingCode: code });
    expect(reason("undeclared.raw-diagnostic")).toBe("http.error");
  });
  it.each([
    () => baseline({ ...base, available: "false" }), () => baseline({ ...base, savedRevision: "old" }),
    () => points([{ ...point, targetId: "replacement" }], "project"), () => points([{ ...point, volumeDataIncluded: true }], "project"),
    () => policy({ ...storedPolicy, mode: "invalid" }, "direct", "project"), () => policy({ ...storedPolicy, environmentId: "other" }, "direct", "project"),
    () => policy({ ...storedPolicy, candidates: [{ ...storedPolicy.candidates[0], notificationState: "delivered" }] }, "direct", "project"),
    () => window({ kind: "daily", time: "25:00" }), () => window({ kind: "weekly", day: 7, time: "04:00" }),
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
  expect(await f.adapter.listUpdatePolicies!("direct")).toEqual([storedPolicy]);
  expect(await f.adapter.getSystemImageHints!("direct")).toEqual(systemHints);
  expect(f.writes()).toEqual([]);
});
it.each(["installStackUpdates", "getUpdateBaseline", "restoreRestorePoint", "listRestorePoints"])("missing prerequisite %s sends no lifecycle write", async (missing) => {
  const f = updateFixture(); f.state.denied.add(missing);
  const call = missing.includes("Restore") || missing === "restoreRestorePoint" ? () => f.adapter.restoreRestorePoint("direct~project", point.id, { expectedRevision: revision, replaceConfiguration: true }) : () => f.adapter.installStackUpdates("direct~project", { expectedRevision: revision });
  await expect(call()).rejects.toHaveProperty("outcome.kind", "not-available"); expect(f.writes()).toHaveLength(0);
});
it("scheduled capability cannot borrow check authority", async () => {
  const f = updateFixture(); f.state.denied.add("setAutoUpdateApply");
  await expect(f.adapter.saveAutoUpdatePolicy!("direct~project", "scheduled", 1)).rejects.toHaveProperty("outcome.kind", "not-available");
  expect((await f.adapter.saveAutoUpdatePolicy!("direct~project", "check", 1)).mode).toBe("check");
  expect(f.writes()).toHaveLength(1);
});
it("policy aggregation preserves healthy hosts and missing next run", async () => {
  const f = updateFixture(); f.state.hostFailure = "other";
  f.state.policy.mode = "scheduled"; f.state.policy.window = { kind: "daily", time: "04:00" };
  const rows = await f.adapter.listSchedule(["direct", "other"]);
  expect(rows.filter((r) => r.hostId === "direct" && r.kind === "auto-update")).toHaveLength(1);
  expect(rows.find((r) => r.hostId === "direct")).toMatchObject({ enabled: true, cadence: "daily" });
  expect(rows.find((r) => r.hostId === "other")).toMatchObject({ unavailable: true });
  expect(f.calls.filter((c) => c.path.endsWith("/policies"))).toHaveLength(2);
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
  const save = f.adapter.saveAutoUpdatePolicy!("direct~project", "scheduled", 1); const result = expect(save).rejects.toBeDefined();
  await started.promise; await f.adapter.logout(); held.resolve(response({})); await result; expect(f.writes().filter((write) => write.path.includes("/policies/"))).toHaveLength(0);
});

it("policy writes require read authority", async () => {
  const f = updateFixture(); f.state.denied.add("getAutoUpdatePolicy");
  await expect(f.adapter.saveAutoUpdatePolicy!("direct~project", "check", 1)).rejects.toHaveProperty("outcome.kind", "not-available"); expect(f.writes()).toHaveLength(0);
});

it("owner loss during capability read fences the mapper before dispatch", async () => {
  let generation = 0; const held = deferred<void>(), started = deferred<void>();
  const fetcher = vi.fn(async () => response({ success: true, data: storedPolicy }));
  const api = createStackUpdates({ baseUrl: "/api", client: { fetch: fetcher, token: async () => "test" }, generation: () => generation, changed: () => {}, capability: async (operationId, resourceRef) => { started.resolve(); await held.promise; return { operationId, resourceRef, available: true }; } });
  const save = api.saveAutoUpdatePolicy("direct~project", "scheduled", 1), assertion = expect(save).rejects.toBeDefined();
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


it.each([{ targetId: "replacement" }, { targetKind: "container" }])("policy rejects a foreign target identity %j", (foreign) => {
  expect(() => policy({ ...storedPolicy, ...foreign }, "direct", "project")).toThrow();
});
it("hints accept build metadata while retaining numeric ordering and the hub cap", () => {
  const value = { ...systemHints, hubVersion: "v2.0.0+hub.7", images: [{ ...systemHints.images[0]!, currentVersion: "1.9.0+agent.3", candidateVersion: "2.0.0+agent.8" }] };
  expect(() => hints(value)).not.toThrow();
  expect(hints(value).images[0]!.candidateVersion).toBe("2.0.0+agent.8");
  expect(compareVersion("v2.0.0+hub.7", "2.0.0+agent.8")).toBe(0);
  expect(() => hints({ ...value, images: [{ ...value.images[0]!, candidateVersion: "2.0.1+agent.8" }] })).toThrow();
  for (const invalid of ["01.2.3+build", "1.2.3-rc.1+build", "9007199254740992.0.0+build"]) expect(compareVersion(invalid, "2.0.0")).toBeUndefined();
});

it.each([
  ["2.0.0", "2.0.0-rc.1", "2.0.0"],
  ["2.0.0-rc.2", "1.8.0", "1.9.0"],
])("hints decode product versions: hub %s, agent %s, candidate %s", (hubVersion, currentVersion, candidateVersion) => {
  const value = { hubVersion, images: [{ ...systemHints.images[0]!, reference: `registry.example.com/docklet/agent:${currentVersion}`, currentVersion, candidateVersion, state: "available" }] };
  expect(hints(value).images[0]!.candidateVersion).toBe(candidateVersion);
});
it("hints still refuse an rc candidate above an rc hub or a non-newer candidate", () => {
  const base = { ...systemHints.images[0]!, state: "available" };
  expect(() => hints({ hubVersion: "2.0.0-rc.2", images: [{ ...base, currentVersion: "1.8.0", candidateVersion: "2.0.0" }] })).toThrow();
  expect(() => hints({ hubVersion: "2.0.0", images: [{ ...base, currentVersion: "2.0.0", candidateVersion: "2.0.0-rc.1" }] })).toThrow();
});

// Producer shapes from updater GetImagePolicy: an unbound stack and an unsaved container send "", a saved container its name.
it("decodes every policy identity the hub produces and still rejects a malformed stack identity", async () => {
  const f = updateFixture();
  const row = (kind: "stack" | "container", target: string, identity: string, revision: number) => ({ ...storedPolicy, candidates: [], targetKind: kind, targetId: target, targetName: target, targetIdentity: identity, revision });
  f.state.policyList = [row("stack", "project", targetIdentity, 1), row("stack", "unbound", "", 0), row("container", "app", "app", 2), row("container", "fresh", "", 0)];
  const listed = await f.adapter.listUpdatePolicies!("direct");
  expect(listed.map((p) => [p.targetKind, p.targetId, p.targetIdentity])).toEqual([["stack", "project", targetIdentity], ["stack", "unbound", ""], ["container", "app", "app"], ["container", "fresh", ""]]);
  f.state.policyList = [row("stack", "project", "not-a-hash", 1)];
  await expect(f.adapter.listUpdatePolicies!("direct")).rejects.toBeTruthy();
});

it("F5 dashboard keeps policy windows and adds only the hub update-check job with its backend NextRun", async () => {
  const f = updateFixture();
  f.state.policy.mode = "scheduled"; f.state.policy.window = { kind: "weekly", day: 0, time: "04:00" };
  const rows = await f.adapter.listSchedule(["0", "direct"]);
  const checks = rows.filter((r) => r.kind === "update-check");
  expect(checks).toHaveLength(1);
  expect(checks[0]).toMatchObject({ id: "0~update-check", hostId: "0", nextRunAt: f.state.job.nextRun, enabled: true });
  // The backend cron expression is a wire value, never UI text.
  expect(checks[0]!.cadence).toBe("");
  expect(rows.filter((r) => r.kind === "auto-update")).toHaveLength(2);
  expect(f.calls.filter((c) => c.path.endsWith("/jobs")).every((c) => c.path === "/api/environments/0/jobs")).toBe(true);
});

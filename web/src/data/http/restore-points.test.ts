// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original point projection and deletion boundary proofs for issue #231.

import { expect, it } from "vitest";
import { points } from "./update-decoders";
import { updateFixture, point } from "./stack-updates.test-support";
import { response } from "./runtime-commands.test-support";
import { deferred } from "./activity-reader.test-support";

it.each([
  ["protectionReason", undefined], ["protectionReason", "failed-slot"], ["protectionReason", false],
  ["holdState", undefined], ["holdState", "future-hold"], ["holdState", null],
  ["failedAttempt", undefined], ["failedAttempt", "false"], ["failedAttempt", null],
  ["images", undefined], ["images", null], ["images", {}],
])("point boundary rejects %s=%s with declared fallback", (field, value) => {
  let error: unknown;
  try { points([point, { ...point, [field]: value }], "project"); } catch (e) { error = e; }
  expect(error ?? {}).toHaveProperty("outcome.code", "update.check-unavailable");
});
it.each([false, true])("cold Direct/Edge=%s reads keep all projections and typed agent errors", async (edge) => {
  const f = updateFixture(); f.state.isEdge = edge;
  f.state.points = [{ ...point, protectionReason: "healthy-baseline", holdState: "unknown", failedAttempt: true, available: false, code: "restore.unavailable", images: [] }];
  expect(await f.adapter.listRestorePoints!("direct~project")).toEqual(f.state.points);
  expect(await f.adapter.listHostRestorePoints!("direct")).toEqual(f.state.points);
  for (const [code, status] of [["restore.unavailable", 409], ["update.check-unavailable", 503], ["update.persistence-failed", 500], ["agent.unavailable", 502]] as const) {
    f.state.hold = (path) => path.endsWith("/restore-points") ? Promise.resolve(response({ code }, status)) : undefined;
    await expect(f.adapter.listRestorePoints!("direct~project")).rejects.toMatchObject({ outcome: { code, status } });
  }
});
it.each(["deleteRestorePoint", "listRestorePoints", "deleteHostRestorePoint", "listHostRestorePoints"])("point delete requires descriptor %s", async (operation) => {
  const f = updateFixture(); f.state.denied.add(operation);
  const call = operation.includes("Host") ? () => f.adapter.deleteHostRestorePoint!("direct", point.id) : () => f.adapter.deleteRestorePoint!("direct~project", point.id);
  await expect(call()).rejects.toHaveProperty("outcome.kind", "not-available");
  expect(f.writes()).toHaveLength(0);
});
it.each([false, true])("delete uses the exact %s host-admin route and validated acknowledgment", async (host) => {
  const f = updateFixture();
  await (host ? f.adapter.deleteHostRestorePoint!("direct", point.id) : f.adapter.deleteRestorePoint!("direct~project", point.id));
  expect(f.writes()).toHaveLength(1);
  expect(f.writes()[0]).toMatchObject({ path: `/api/environments/direct/updater/${host ? "" : "project/project/"}restore-points/${point.id}`, init: { method: "DELETE" } });
  expect(await f.adapter.listHostRestorePoints!("direct")).toEqual([]);
});
it.each(["lost", "malformed", "proxy"])("delete %s response remains uncertain", async (kind) => {
  const f = updateFixture();
  f.state.hold = (_path, init) => init.method !== "DELETE" ? undefined : kind === "lost" ? Promise.reject(new TypeError("network")) : Promise.resolve(kind === "proxy" ? response({ code: "agent.unavailable" }, 502) : response({ success: true, data: { message: false } }));
  await expect(f.adapter.deleteRestorePoint!("direct~project", point.id)).rejects.toHaveProperty("outcome.code", "runtime.admission-unknown");
  expect(f.writes()).toHaveLength(1);
});
it("delete cannot publish a response owned by the logged-out identity", async () => {
  const f = updateFixture(), started = deferred<void>(), held = deferred<Response>();
  await f.adapter.getCurrentUser();
  f.state.hold = (_path, init) => { if (init.method === "DELETE") { started.resolve(); return held.promise; } };
  const deletion = f.adapter.deleteRestorePoint!("direct~project", point.id), result = expect(deletion).rejects.toBeDefined();
  await started.promise; await f.adapter.logout(); held.resolve(response({ success: true, data: { message: "Deleted" } })); await result;
  expect(f.writes().filter((c) => c.init.method === "DELETE")).toHaveLength(1);
});

it("enabled delete descriptors intersect the real HTTP mapping", async () => {
  const f = updateFixture();
  expect((await f.adapter.getCapability("deleteRestorePoint", { kind: "stack", id: "direct~project" })).available).toBe(true);
  expect((await f.adapter.getCapability("deleteHostRestorePoint", { kind: "host", id: "direct" })).available).toBe(true);
});

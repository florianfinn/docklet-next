// Source: docklet-next/issues/586
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original setup test decoder and independent lifecycle fencing proofs.

import { expect, test } from "vitest";
import { isCancelledError } from "@tanstack/react-query";
import type { TailnetStatus, TestResult } from "@/domain/tailnet";
import { createTailnet } from "./tailnet";

const status: TailnetStatus = { enabled: true, credentialRevision: "example-boot:1", generation: 1, connection: { state: "connected" }, provisioning: { state: "ok" }, hostCount: 0, onlineHosts: 0, routeAvailable: true };
function result(): TestResult { return { credentialRevision: status.credentialRevision, stale: false, checks: ["credential", "agent-key", "cleanup", "hub-joined", "hub-tag", "hub-address"].map((id) => ({ id: id as TestResult["checks"][number]["id"], state: "pass" })) }; }
function fixture() {
  let owner = 0;
  const pending: ((response: Response) => void)[] = [];
  const controller = createTailnet("/api", { token: async () => "example", fetch: async (_url, init) => {
    if (init?.method === "GET") return Response.json({ success: true, data: status });
    return new Promise<Response>((resolve) => pending.push(resolve));
  } }, () => owner);
  controller.observe(status);
  return { controller, identity: () => { ++owner; controller.clear(); }, answer: (data: unknown = result(), httpStatus = 200) => pending.shift()!(Response.json(httpStatus === 200 ? { success: true, data } : data, { status: httpStatus })) };
}

test.each(["tailnet.mint-refused", "tailnet.test-cleanup-failed", "tailnet.test-running"] as const)("decodes transported check code %s", async (code) => {
  const f = fixture(), out = result(); out.checks[1] = { id: "agent-key", state: "fail", code, providerStatus: 403 };
  const pending = f.controller.api.test(); f.answer(out);
  expect((await pending).checks[1]).toEqual(out.checks[1]);
});
test("decodes the concurrent HTTP Problem", async () => {
  const f = fixture(), pending = f.controller.api.test();
  f.answer({ code: "tailnet.test-running", status: 409 }, 409);
  await expect(pending).rejects.toMatchObject({ outcome: { code: "tailnet.test-running", status: 409 } });
});

test.each([
  { checks: [] }, { checks: result().checks.map((c) => ({ ...c, id: "future-check" })) },
  { credentialRevision: 42 }, { stale: "false" },
  { checks: result().checks.map((c) => ({ ...c, state: "future-state" })) },
  { checks: result().checks.map((c) => ({ ...c, code: "tailnet.future-code" })) },
  { checks: result().checks.map((c) => ({ ...c, providerStatus: 0 })) },
  { checks: result().checks.map((c) => ({ ...c, id: "credential" })) },
])("rejects malformed result %#", async (change) => {
  const f = fixture(), pending = f.controller.api.test(); f.answer({ ...result(), ...change });
  await expect(pending).rejects.toThrow("tailnet.invalid-response");
});

test("ordinary status observations preserve a held test and its epoch", async () => {
  const f = fixture(), epoch = f.controller.api.testEpoch(), pending = f.controller.api.test();
  const sequence = f.controller.api.testSequence();
  f.controller.observe({ ...status, connection: { state: "joining" } });
  expect(f.controller.api.testEpoch()).toBe(epoch);
  expect(f.controller.api.testSequence()).toBe(sequence);
  f.answer(); await expect(pending).resolves.toEqual(result());
});

test.each(["save", "reset", "clear", "reconnect"] as const)("%s advances only the test epoch and fences a held result", async (action) => {
  const f = fixture(), epoch = f.controller.api.testEpoch(), pending = f.controller.api.test().catch((e: unknown) => e);
  let write: Promise<unknown> | undefined;
  if (action === "save") write = f.controller.api.save({ enabled: true, mode: "oauth" });
  if (action === "reset") write = f.controller.api.reset();
  if (action === "clear") f.controller.clear();
  if (action === "reconnect") write = f.controller.reconnect();
  expect(f.controller.api.testEpoch()).toBe(epoch + 1);
  f.answer(); expect(isCancelledError(await pending)).toBe(true);
  if (action === "save") f.answer({ enabled: true, hostname: "example", secretConfigured: true, customControl: false, policySnippet: "{}" });
  if (action === "reset") f.answer(status);
  await write;
});

test.each(["revision", "stale", "identity", "sequence"] as const)("fences late tests by %s", async (change) => {
  const f = fixture(), pending = f.controller.api.test().catch((e: unknown) => e);
  const data = result(); let second: Promise<unknown> | undefined;
  if (change === "revision") f.controller.observe({ ...status, credentialRevision: "another-boot:1" });
  if (change === "stale") data.stale = true;
  if (change === "identity") f.identity();
  if (change === "sequence") second = f.controller.api.test();
  f.answer(data); expect(isCancelledError(await pending)).toBe(true);
  if (second) { f.answer(); await second; }
});


test.each(["pass", "fail", "skipped", "unknown"] as const)("decodes check state %s", async (state) => {
  const f = fixture(), out = result(); out.checks[2]!.state = state;
  const pending = f.controller.api.test(); f.answer(out);
  expect((await pending).checks[2]!.state).toBe(state);
});

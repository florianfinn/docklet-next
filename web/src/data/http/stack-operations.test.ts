// Source: docklet-next/issues/316
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W5/W6 frozen restart query, pending identity and hub stop-mode transport tests.

import { expect, it } from "vitest";
import { waitFor } from "@testing-library/react";
import { deferred } from "./activity-reader.test-support";
import { envelope, response, RUN_ID, runtimeFixture, snapshot } from "./runtime-commands.test-support";
import { GATE_RULES } from "../run-gate";
import { preferencesStore } from "@/shared/preferences/preferences-store";

it.each(["0", "direct", "edge"])("W5 %s uses restart-project's real services query with one service and an idempotency header", async (host) => {
  const f = runtimeFixture(); f.state.run = snapshot(host, { resourceType: "project", resourceId: "stack", type: "project_restart" });
  expect(await f.adapter.restartService(`${host}~stack`, "api /?#")).toBe(RUN_ID);
  const call = f.commands()[0]!;
  expect(call.url.pathname).toBe(`/api/environments/${host}/projects/stack/restart`);
  expect(call.url.searchParams.getAll("services")).toEqual(["api /?#"]);
  expect(call.init.method).toBe("POST");
  expect(call.init.body).toBeUndefined();
  expect(new Headers(call.init.headers).get("Idempotency-Key")).toMatch(/^[0-9a-f-]{36}$/);
});

it("W5 includes the service in both pending and run-gate keys and retains a parallel stack refusal", async () => {
  const f = runtimeFixture(); f.state.run = snapshot("0", { resourceType: "project", resourceId: "stack", type: "project_restart" });
  const first = deferred<Response>();
  f.state.command = async (url) => url.searchParams.get("services") === "api" ? first.promise : response({ success: false, data: { code: "run.busy", activeRunId: RUN_ID } }, 409);
  const a = f.helper.restartService("0~stack", "api");
  await waitFor(() => expect(f.commands().length).toBe(1));
  const b = f.helper.restartService("0~stack", "worker");
  first.resolve(envelope({ activityId: RUN_ID, code: "run.started", activity: f.state.run }, 202));
  await expect(b).rejects.toMatchObject({ outcome: { kind: "conflict", code: "run.busy", activeRunId: RUN_ID } });
  expect(f.commands().length).toBe(2);
  await a;
  const api = GATE_RULES.restartService!(["0~stack", "api"]);
  const worker = GATE_RULES.restartService!(["0~stack", "worker"]);
  expect(api.scope).toBe(worker.scope); expect(api.key).not.toBe(worker.key);
  await expect(f.helper.runStackCommand("0~stack", "restart")).rejects.toMatchObject({ outcome: { code: "run.busy" } });
});

it.each(["down", "stop", undefined] as const)("W6 reads the hub setting under an Edge host filter and dispatches %s", async (mode) => {
  preferencesStore.set((state) => ({ ...state, hostSelection: ["edge"] }));
  try {
    const f = runtimeFixture(); f.state.stopMode = mode; f.state.run = snapshot("edge", { resourceType: "project", resourceId: "stack", type: "project_down" });
    await f.adapter.runStackCommand("edge~stack", "stop");
    const settings = f.calls.filter((call) => call.url.pathname.endsWith("/settings"));
    expect(settings.map((call) => call.url.pathname)).toEqual(["/api/environments/0/settings"]);
    expect(f.commands()[0]!.url.pathname).toBe(`/api/environments/edge/projects/stack/${mode === "stop" ? "stop" : "down"}`);
    expect(f.commands()[0]!.init.body).toBeUndefined();
    await f.adapter.runStackCommand("edge~stack", mode === "stop" ? "down" : "stop-containers");
    expect(f.commands()[1]!.url.pathname).toBe(`/api/environments/edge/projects/stack/${mode === "stop" ? "down" : "stop"}`);
  } finally { preferencesStore.reset(); }
});

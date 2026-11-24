// Source: docklet-next/issues/597
// Revision: 6e3c477e97192029f67bcf924da27b3b3849bbd8
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original static host-job HTTP, gate and producer mapping proofs.

import { expect, it, vi } from "vitest";
import { createDemoAdapter } from "../demo/demo-adapter";
import { createDisconnectedAdapter } from "../disconnected-adapter";
import { RunGate, GATE_RULES, withRunGate } from "../run-gate";
import { mapActivityDetail } from "./map-dto";
import { renderRun, type CodedActionRun } from "../message-code";
import { updateFixture } from "./stack-updates.test-support";
import { response } from "./runtime-commands.test-support";
import activity from "@/features/stacks/testdata/host-update-activity.json";

it.each(["0", "direct", "edge", undefined])("starts %s through the existing hub route and follows its activity ID", async (host) => {
  const f = updateFixture(), target = host ?? "0", job = host === undefined ? "update-check" : "update-check-host", runId = host === undefined ? f.run.id : "example-run";
  f.state.hold = (path, init) => path.endsWith("/run") && init.method === "POST" ? Promise.resolve(response({ success: true, runId: "example-run", status: "queued" }, 202))
    : path.includes("/runs/") ? Promise.resolve(response({ id: runId, jobId: job, environmentId: target, activityId: "example-activity" })) : undefined;
  expect(await f.adapter.checkHostUpdates!(host)).toBe("example-activity");
  const write = f.writes()[0]!;
  expect(write.path).toBe(host === undefined ? "/api/updater/check-all" : `/api/environments/${host}/jobs/update-check-host/run`);
  expect(f.calls.some((c) => c.path === `/api/environments/${target}/jobs/${job}/runs/${runId}`)).toBe(true);
});

it("checks the existing administrator JobsManage descriptor before admission", async () => {
  const f = updateFixture(); f.state.denied.add("checkAllUpdates");
  await expect(f.adapter.checkHostUpdates!("direct")).rejects.toMatchObject({ outcome: { kind: "not-available", operationId: "checkAllUpdates" } });
  expect(f.writes()).toHaveLength(0);
});

it("rejects a run read from another host instead of following its activity", async () => {
  const f = updateFixture();
  f.state.hold = (path) => path.endsWith("/run") ? Promise.resolve(response({ success: true, runId: "example-run" }))
    : path.includes("/runs/") ? Promise.resolve(response({ id: "example-run", jobId: "update-check-host", environmentId: "other", activityId: "foreign" })) : undefined;
  await expect(f.adapter.checkHostUpdates!("direct")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
});

it("the host gate coalesces pending admission and an observed queued/running run", async () => {
  const f = updateFixture(); let publish!: (rows: CodedActionRun[]) => void, release!: (value: string) => void;
  const start = vi.fn(() => new Promise<string>((resolve) => { release = resolve; }));
  f.adapter.checkHostUpdates = start;
  f.adapter.subscribeActions = (listener) => { publish = listener; return () => {}; };
  const gate = new RunGate(f.adapter);
  expect(GATE_RULES.checkHostUpdates!(["direct"]).key).toBe("check:host:direct");
  const first = gate.run("checkHostUpdates", ["direct"]), second = gate.run("checkHostUpdates", ["direct"]);
  expect(start).toHaveBeenCalledTimes(1);
  release("example-host-check");
  expect(await first).toBe(await second);
  const row = mapActivityDetail({ activity, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
  for (const status of ["queued", "running"] as const) {
    publish([{ ...row, status }]);
    expect(await gate.run("checkHostUpdates", ["direct"])).toBe("example-host-check");
    expect(start).toHaveBeenCalledTimes(1);
  }
  publish([{ ...row, status: "succeeded" }]);
  const next = gate.run("checkHostUpdates", ["direct"]);
  expect(start).toHaveBeenCalledTimes(2); release("next"); await next;
});

it.each(["en-US", "de"] as const)("maps the real host-job producer to the update kind, title and jobRef (%s)", (locale) => {
  const run = mapActivityDetail({ activity, stepTrace: [], messages: [], output: [], outputTruncated: false } as never);
  expect(run.jobRef).toEqual({ jobId: "update-check-host", runId: "example-run" });
  expect(run.hostId).toBe("example-host");
  expect(renderRun(run, locale).kind).toBe("check-updates");
  expect(renderRun(run, locale).title).toBe(locale === "de" ? "Update-Prüfung" : "Update check");
});

it("the demo port targets one host and the disconnected port refuses it", async () => {
 vi.useFakeTimers();
 try {
  const adapter = createDemoAdapter();
  const hosts = adapter.listHosts(); await vi.runAllTimersAsync(); const host = (await hosts)[0]!;
  const id = await adapter.checkHostUpdates!(host.id);
  const actions = adapter.listActions(); await vi.runAllTimersAsync();
  const run = (await actions).find((r) => r.id === id)!;
  expect(run.hostId).toBe(host.id); expect(run.kind).toBe("check-updates");
  await expect(createDisconnectedAdapter().checkHostUpdates!(host.id)).rejects.toMatchObject({ outcome: { kind: "not-available" } });
 } finally { vi.clearAllTimers(); vi.useRealTimers(); }
});

// A host check is safe to resend: one dropped POST must not lock the host check for the session.
it("sends a host check again after one dropped POST", async () => {
  const f = updateFixture(); let posts = 0, drop = true;
  f.state.hold = (path, init) => {
    if (path.endsWith("/jobs/update-check-host/run") && init.method === "POST") {
      posts++;
      if (drop) { drop = false; return Promise.reject(new TypeError("network")); }
      return Promise.resolve(response({ success: true, runId: "example-run", status: "queued" }, 202));
    }
    return path.includes("/runs/") ? Promise.resolve(response({ id: "example-run", jobId: "update-check-host", environmentId: "direct", activityId: "example-activity" })) : undefined;
  };
  const gated = withRunGate(f.adapter);
  await expect(gated.checkHostUpdates!("direct")).rejects.toMatchObject({ outcome: { code: "runtime.admission-unknown" } });
  const second = await gated.checkHostUpdates!("direct").then((value) => value, (error: unknown) => error);
  expect(posts).toBe(2);
  expect(second).toBe("example-activity");
});

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { withRunGate } from "../run-gate";

import { createDemoAdapter } from "./demo-adapter";
import { queueDemoFailure, type DemoFailure } from "./options";
import { simulationStore } from "./simulation";

beforeEach(() => {
  vi.useFakeTimers();
  simulationStore.set((state) => ({ ...state, latency: "instant", live: false, actionSpeed: 20 }));
});

afterEach(() => {
  vi.useRealTimers();
  simulationStore.reset();
  localStorage.clear();
});

const cases: { failure: DemoFailure; start: (adapter: ReturnType<typeof createDemoAdapter>) => Promise<string>; step: string }[] = [
  { failure: { operation: "runStackCommand", at: "pull" }, start: (adapter) => adapter.runStackCommand("media", "redeploy"), step: "pull" },
  { failure: { operation: "installStackUpdates", at: "pull" }, start: (adapter) => adapter.installStackUpdates("cloud"), step: "pull" },
  { failure: { operation: "deployStack", at: "pull" }, start: async (adapter) => (await adapter.deployStack({ stackId: null, hostId: "atlas", name: "Example", compose: "services:\n  app:\n    image: nginx:1.27\n", env: "", labels: [] })).actionId, step: "pull" },
  { failure: { operation: "recreateContainer", at: "start" }, start: (adapter) => adapter.recreateContainer("c-whoami-test", { image: "traefik/whoami:v1.11", ports: [], env: {}, restartPolicy: "no" }), step: "start" },
  { failure: { operation: "connectHost" }, start: (adapter) => adapter.connectHost("atlas"), step: "connect" },
  { failure: { operation: "connectHost" }, start: (adapter) => adapter.connectHost("cedar"), step: "wait" },
  { failure: { operation: "pullImage" }, start: async (adapter) => (await adapter.pullImage({ hostId: "atlas", reference: "example/missing:1.0" })).actionId!, step: "pull" },
];

describe("demo-only failure options", () => {
  it.each(cases)("fails $failure.operation at the requested step", async ({ failure, start, step }) => {
    const adapter = createDemoAdapter({ nextFailure: failure });
    const id = await start(adapter);
    await vi.advanceTimersByTimeAsync(20000);
    const run = (await adapter.listActions()).find((item) => item.id === id);
    expect([run?.status, run?.steps.find((item) => item.state === "error")?.key]).toEqual(["failed", step]);
  });

  it("consumes only the matching operation and leaves the next call normal", async () => {
    const adapter = createDemoAdapter();
    queueDemoFailure(adapter, { operation: "pullImage" });
    const other = await adapter.runStackCommand("media", "stop");
    const requested = await adapter.pullImage({ hostId: "atlas", reference: "example/missing:1.0" });
    await vi.advanceTimersByTimeAsync(20000);
    const runs = await adapter.listActions();
    expect([runs.find((run) => run.id === other)?.status, runs.find((run) => run.id === requested.actionId)?.status]).toEqual(["succeeded", "failed"]);
  });

  it("applies options through the same run-gated adapter that UI hooks receive", async () => {
    const adapter = withRunGate(createDemoAdapter());
    queueDemoFailure(adapter, { operation: "runStackCommand", at: "pull" });
    const id = await adapter.runStackCommand("media", "redeploy");
    await vi.advanceTimersByTimeAsync(20000);
    expect((await adapter.listActions()).find((run) => run.id === id)?.status).toBe("failed");
  });
});

// Source: docklet-next/issues/635
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M42 container health projection and probe reason rules.

import { describe, expect, it } from "vitest";
import type { components } from "@/api/generated/schema";
import { AdapterOutcomeError } from "../capability";
import { createHttpHistory } from "./history";

type WireDetails = components["schemas"]["ContainerDetails"];

// Shape of the hub's get-container body (types/container/inventory.go NewDetails): Docker durations in nanoseconds,
// log entries from a CMD-form probe captured in the sandbox. Environment and command values are synthetic.
const ID = "4f1c0d9e2b7a";
function wire(extra: Partial<WireDetails> = {}): WireDetails {
  return {
    id: ID, name: "probes-exec-1", image: "alpine:3.22.6", imageId: "sha256:0", imagePlatform: "linux/amd64", created: "2026-11-25T22:02:54Z",
    state: {
      status: "running", running: true, startedAt: "2026-11-25T22:02:55.112233445Z",
      health: { status: "unhealthy", failingStreak: 3, log: [{ start: "2026-11-25T22:03:07.330261119Z", end: "2026-11-25T22:03:07.377080893Z", exitCode: -1, output: 'OCI runtime exec failed: exec failed: unable to start container process: exec: "curl": executable file not found in $PATH' }] },
    },
    config: { env: ["LOG_LEVEL=debug"], cmd: ["sleep", "600"], healthcheck: { test: ["CMD", "curl", "-fsS", "http://127.0.0.1/"], interval: 2_000_000_000, startPeriod: 0 } },
    hostConfig: {} as WireDetails["hostConfig"], networkSettings: {} as WireDetails["networkSettings"], ports: [], mounts: [], autoUpdateEnabled: false, origin: "compose", originSource: "",
    ...extra,
  } as WireDetails;
}

function client(body: unknown, status = 200) {
  const calls: string[] = [];
  return { calls, client: { token: async () => "token", fetch: (async (input: RequestInfo | URL) => { calls.push(String(input)); return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }); }) as typeof fetch } };
}

describe("container health transport", () => {
  it("reads get-container on the container's host and keeps only the health projection", async () => {
    const { calls, client: c } = client({ success: true, data: wire() });
    const health = await createHttpHistory("/api", c).getContainerHealth(`host-1~${ID}`);
    expect(calls).toEqual([`/api/environments/host-1/containers/${ID}`]);
    expect(health).toEqual({
      containerId: `host-1~${ID}`, running: true, startedAt: "2026-11-25T22:02:55.112233445Z", status: "unhealthy", failingStreak: 3,
      probes: [{ start: "2026-11-25T22:03:07.330261119Z", end: "2026-11-25T22:03:07.377080893Z", exitCode: -1, output: 'OCI runtime exec failed: exec failed: unable to start container process: exec: "curl": executable file not found in $PATH' }],
      check: { test: ["CMD", "curl", "-fsS", "http://127.0.0.1/"], intervalMs: 2_000, timeoutMs: 0, retries: 0, startPeriodMs: 0, startIntervalMs: 0 },
    });
    expect(JSON.stringify(health).includes("LOG_LEVEL")).toBe(false);
  });

  it("maps a container without healthcheck and Docker's zero start time", async () => {
    const { client: c } = client({ success: true, data: wire({ state: { status: "exited", running: false, startedAt: "0001-01-01T00:00:00Z" }, config: { healthcheck: undefined } }) });
    const health = await createHttpHistory("/api", c).getContainerHealth(`host-1~${ID}`);
    expect([health.running, health.status, health.startedAt === undefined, health.check === undefined, health.probes.length]).toEqual([false, "none", true, true, 0]);
  });

  it("refuses an answer for another container id", async () => {
    const { client: c } = client({ success: true, data: wire({ id: "otherid" }) });
    await expect(createHttpHistory("/api", c).getContainerHealth(`host-1~${ID}`)).rejects.toBeInstanceOf(AdapterOutcomeError);
  });
});

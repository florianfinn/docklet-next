// Source: docklet-next/issues/428
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs that a maintenance refusal of one context leg fails topology and volume reads instead of dropping rows (R1E-1).

import { expect, it } from "vitest";
import { isMaintenanceError } from "../maintenance-retry";
import { maintenanceResponse } from "./dispatch-gate";
import { resourceFixture } from "./resources.test-support";
import { createResourceReader } from "./resources";
import { createVolumeReads } from "./volumes";

/** The resource fixture hub whose `leg` list answers with the gate's maintenance refusal once armed. */
async function fixture(leg: "containers" | "projects") {
  const f = resourceFixture(); f.state.hostIds = ["first"]; f.state.binds = [];
  const hosts = await f.adapter.listHosts();
  let armed = false;
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    if (armed && new URL(String(input), "https://hub.example.com").pathname.endsWith(`/environments/first/${leg}`)) return maintenanceResponse();
    return (f.fetcher as typeof fetch)(input, init);
  }) as typeof fetch;
  return { hosts, fetcher, arm: () => { armed = true; } };
}
const outcome = (read: Promise<unknown>) => read.then(() => "resolved", (error: unknown) => isMaintenanceError(error));

it.each(["containers", "projects"] as const)("R1E-1 topology: a maintenance refusal of only the %s context fails the read instead of an empty graph", async (leg) => {
  const f = await fixture(leg);
  const reader = createResourceReader("/api", f.fetcher, async () => f.hosts, async () => {}, () => {});
  await reader.getTopology();
  f.arm();
  expect(await outcome(reader.getTopology())).toBe(true);
});

it.each(["containers", "projects"] as const)("R1E-1 volumes: a maintenance refusal of only the %s context fails list and detail instead of dropping bind rows", async (leg) => {
  const f = await fixture(leg);
  const reader = createVolumeReads("/api", f.fetcher, async () => f.hosts, async () => {}, () => {}, () => 0);
  const [volume] = await reader.listVolumes();
  f.arm();
  expect([await outcome(reader.listVolumes()), await outcome(reader.getVolume(volume!.id))]).toEqual([true, true]);
});

// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original live runtime condition merge tests from the hub response shape, issue #371.

import { describe, expect, it } from "vitest";
import { STANDALONE } from "@/data/demo/fixtures/containers";
import type { ContainerInfo, Service, Stack } from "@/domain/model";
import { containerRuntime, serviceRuntime } from "@/domain/status";
import { readRuntimeConditions, withConditions } from "./runtime-conditions";

const item = (containerId: string, condition: string, extra: Record<string, unknown> = {}) => ({
  kind: "container", containerId, name: containerId, projectId: "", condition, reason: "exit", exitCode: 1, restarts10m: 0,
  since: "2026-11-03T11:00:00Z", sampledAt: "2026-11-03T12:00:00Z", ...extra,
});
const reply = (items: unknown[]) => (async () => new Response(JSON.stringify({ success: true, data: { hostId: "h", sampledAt: "2026-11-03T12:00:00Z", items } }))) as typeof fetch;

describe("runtime conditions", () => {
  it("reads container conditions of one host by Docker id and skips stack rows and minutes without data", async () => {
    const urls: string[] = [];
    const fetcher = (async (input: RequestInfo | URL) => { urls.push(String(input)); return reply([item("c1", "down"), item("c2", "nodata"), { ...item("p1", "down"), kind: "stack" }])(input); }) as typeof fetch;
    const conditions = await readRuntimeConditions("/api", fetcher, "host 1");
    expect(urls).toEqual(["/api/environments/host%201/runtime-conditions"]);
    expect([...conditions.keys()]).toEqual(["c1"]);
    expect(conditions.get("c1")).toEqual({ condition: "down", reason: "exit", exitCode: 1, sampledAt: "2026-11-03T12:00:00Z" });
  });

  it("yields no conditions when the read fails, so exited containers stay grey", async () => {
    const failing = (async () => new Response(JSON.stringify({ code: "history.unavailable", status: 503 }), { status: 503 })) as typeof fetch;
    expect((await readRuntimeConditions("/api", failing, "h")).size).toBe(0);
    expect((await readRuntimeConditions("/api", (async () => { throw new TypeError("offline"); }) as typeof fetch, "h")).size).toBe(0);
  });

  it("turns an exited container and its service red only for the sampled incarnation", async () => {
    const container: ContainerInfo = { ...STANDALONE[0]!, id: "h~c1", hostId: "h", state: "stopped", health: "none" };
    const other: ContainerInfo = { ...container, id: "h~c9" };
    const service = { name: "app", containerId: "c1", state: "stopped", health: "none" } as Service;
    const stack = { id: "h~p", services: [service, { ...service, containerId: "c9" }] } as unknown as Stack;
    const merged = withConditions({ stacks: [stack], containers: [container, other] }, await readRuntimeConditions("/api", reply([item("c1", "down")]), "h"));
    expect(merged.containers.map((c) => containerRuntime(c, undefined))).toEqual(["down", "stopped"]);
    expect(merged.stacks[0]!.services.map((s) => serviceRuntime(s, undefined))).toEqual(["down", "stopped"]);
  });
});

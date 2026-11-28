// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { expect, it } from "vitest";
import { createDemoAdapter } from "./demo-adapter";
import { renderStackDetail } from "../message-code";
it("exposes unhealthy, exit, OOM and restart-loop fixtures with matching container identities", async () => {
  const adapter = createDemoAdapter({ logHealthInsights: true });
  const stacks = await adapter.listStacks();
  const services = stacks.flatMap((s) => s.services);
  for (const reason of ["health", "exit", "oom", "restart-loop"]) {
    const service = services.find((s) => s.condition?.reason === reason)!;
    expect(service).toBeDefined();
    expect(service.condition?.containerId).toBe(service.containerId);
  }
  const unhealthy = services.find((s) => s.health === "unhealthy")!;
  const health = await adapter.getContainerHealth(unhealthy.containerId!);
  expect(health.probes.some((p) => p.exitCode !== 0 && p.output.includes("404"))).toBe(true);
  const exited = services.find((s) => s.condition?.reason === "exit")!;
  const sink: string[] = [];
  const result = await adapter.readLogs({ kind: "container", id: exited.containerId! }, { tail: 20, maxLines: 20, maxBytes: 65536 }, (lines) => sink.push(...lines.map((l) => l.message)), new AbortController().signal);
  expect(result.reason).toBe("cap-lines"); expect(sink).toHaveLength(20);
  const stack = renderStackDetail(await adapter.getStack(stacks[0]!.id), "en-US");
  expect(stack.services[0]!.containerId).toBeDefined();
});

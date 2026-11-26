// Source: docklet-next/issues/635
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M42 container health projection and probe reason rules.

import { describe, expect, it } from "vitest";
import { mapContainer, mapStack, type ContainerRow, type ProjectRow } from "./inventory-mapping";

// Runtime services follow types/project/project.go RuntimeService: containerName is the Docker name history uses.
const row = { id: "c1", names: ["/web-app-1"], image: "nginx:1", state: "running", status: "Up", origin: "compose", labels: { "com.docker.compose.project": "web", "com.docker.compose.service": "app" } } as unknown as ContainerRow;
const project = (services: unknown[]) => ({ id: "p1", name: "web", path: "/srv/web", status: "running", serviceCount: services.length, runningCount: 1, runtimeServices: services }) as unknown as ProjectRow;

function names(p: ProjectRow) {
  const container = mapContainer(row, "h", [p], { stale: false });
  return mapStack(p, "h", [container], { stale: false }).services.map((s) => [s.name, s.containerName ?? null]);
}

describe("service container name", () => {
  it("keeps the hub's container name instead of the service name", () => {
    expect(names(project([{ name: "app", image: "nginx:1", status: "running", containerId: "c1", containerName: "web-app-1" }]))).toEqual([["app", "web-app-1"]]);
  });

  it("falls back to the matching inventory container and leaves removed services without a name", () => {
    expect(names(project([{ name: "app", image: "nginx:1", status: "running", containerId: "c1" }, { name: "db", image: "postgres:18", status: "not-created" }])))
      .toEqual([["app", "web-app-1"], ["db", null]]);
  });
});

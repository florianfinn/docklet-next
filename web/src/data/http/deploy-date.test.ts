// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original deploy-date fallback for same-tag rebuild labels.

import { expect, it } from "vitest";
import type { InventoryObservation } from "@/domain/model";
import { mapContainer, mapStack, type ContainerRow, type ProjectRow } from "./inventory-mapping";
import { observation } from "./update-observation.test-support";
import identity from "./project-ownership.test-support";

const labels = { "com.docker.compose.project": "beta_stack", "com.docker.compose.service": "web" };
const inventory = { stale: false } as InventoryObservation;
const ref = "registry.example.com/app:latest";
const update = { ...observation("rebuild"), imageRef: ref, currentVersion: "latest", latestVersion: "latest" };
const container: ContainerRow = { autoUpdateEnabled: false, origin: "compose", originSource: "detected", command: "serve", created: 1794294000, ports: [], hostConfig: {}, mounts: [], networkSettings: { networks: {} }, id: "c1", names: ["/beta_stack-web-1"], state: "running", status: "Up", image: ref, imageId: "sha256:example", imagePlatform: "unknown", labels, updateInfo: update };

// Without a build date the running image is dated by its container's creation, in container rows and stack services.
it("carries the container's deploy date to the running image", () => {
  const row: ProjectRow = { ...identity, path: "/srv/projects/example-project", status: "running", serviceCount: 1, runningCount: 1, isArchived: false, createdAt: "", updatedAt: "", tags: [], runtimeServices: [{ name: "web", image: ref, status: "running", containerId: "c1" }], updateInfo: { status: "has_update", hasUpdate: true, imageCount: 1, checkedImageCount: 1, imagesWithUpdates: 1, imagesNotPulled: 0, errorCount: 0, serviceUpdates: { web: { imageRef: ref, updateInfo: update } } } };
  const mapped = mapContainer(container, "h1", [row], inventory);
  expect(mapped.update.current.deployed).toBe("2026-11-10T07:00:00.000Z");
  expect(mapped.update.state === "rebuild" && mapped.update.latest.deployed).toBe(undefined);
  expect(mapStack(row, "h1", [mapped], inventory).services[0]!.update.current.deployed).toBe("2026-11-10T07:00:00.000Z");
  expect(mapContainer({ ...container, created: 0 }, "h1", [], inventory).update.current.deployed).toBe(undefined);
});

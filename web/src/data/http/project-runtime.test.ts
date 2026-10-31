// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Backend-serialized runtime membership through actual web projections.

import { describe, expect, it } from "vitest";
import type { components } from "@/api/generated/schema";
import type { InventoryObservation } from "@/domain/model";
import { removeBlock } from "@/domain/networks";
import { mapContainer, mapStack, type ContainerRow, type ProjectRow } from "./inventory-mapping";
import { mapImage, mapTopology, mapVolumes } from "./resource-mapping";
import wire from "./project-runtime.test-support";

// Go's TestComposeNameRuntimeMembershipWire validates these actual serialized
// list/detail fields for manual labels and unrefreshed current-file/env edits.
const project: ProjectRow = { path: "/srv/projects/example-project", isArchived: false, createdAt: "", updatedAt: "", tags: [], ...wire, runtimeServices: wire.runtimeServices as NonNullable<ProjectRow["runtimeServices"]> };
const inventory = { stale: false } as InventoryObservation;
const labels = { "com.docker.compose.project": "manual_p", "com.docker.compose.service": "app", "com.docker.compose.project.working_dir": project.path };
const container: ContainerRow = { autoUpdateEnabled: false, origin: "compose", originSource: "detected", command: "serve", created: 0, ports: [{ privatePort: 8080, publicPort: 8080, type: "tcp" }], hostConfig: {},
  mounts: [{ type: "volume", name: "manual_p_data", destination: "/data", rw: true }], networkSettings: { networks: { manual_p_default: {} } },
  id: "c-fallback", names: ["/manual_p-app-1"], state: "running", status: "Up", image: "registry.example.com/app:1", imageId: "sha256:example", labels };
const volume: components["schemas"]["VolumeVolume"] = { id: "manual_p_data", name: "manual_p_data", driver: "local", containers: [], inUse: false, options: {}, scope: "local", mountpoint: "/srv/data", size: 0, createdAt: "", labels: { ...labels, "com.docker.compose.volume": "data" } };
const network: components["schemas"]["NetworkInspect"] = { scope: "local", created: "", configFrom: { Network: "" }, options: {}, containersList: [], attachable: false, ingress: false, configOnly: false,
  id: "n-fallback", name: "manual_p_default", driver: "bridge", internal: false, enableIPv4: true, enableIPv6: false, ipam: { driver: "default", options: {}, config: [] }, labels, containers: {} };

describe("explicit backend runtime membership", () => {
  it.each(["current", "old"])("retains the actual directory-fallback wire membership for a %s peer", (peer) => {
    const p = { ...project, ...(peer === "old" ? { composeProjectName: undefined } : {}) };
    const c = mapContainer(container, "h1", [p], inventory);
    const stack = mapStack(p, "h1", [c], inventory);
    expect([stack.id, stack.name, stack.reportedStatus, stack.serviceCount, stack.runningCount]).toEqual(["h1~example-project", "alpha", "running", 1, 1]);
    expect(stack.services, "reported running 1/1 must retain its explicit backend service").toHaveLength(1);
    expect(c.stackId).toBe("h1~example-project");
    expect(c.id).toBe("h1~c-fallback");
    expect(c.stackName).toBe("alpha");
    expect(c.origin).toBe("compose");
    expect(stack.services[0]).toMatchObject({ name: "app", state: "running", image: { repository: "registry.example.com/app", tag: "1" }, ports: [{ container: "8080", host: "8080", protocol: "tcp" }], networks: ["manual_p_default"] });
    const image = mapImage({ id: "sha256:example", size: 0, created: 0, repoTags: [container.image] } as components["schemas"]["ImageSummary"], "h1", [container], [p], inventory);
    expect(image.consumers[0]).toMatchObject({ containerId: c.id, stackId: stack.id, stackName: "alpha", serviceName: "app" });
    const v = mapVolumes([volume], "h1", [container], [p], inventory)[0]!;
    expect(v.consumers[0]).toMatchObject({ containerId: c.id, stackId: stack.id, stackName: "alpha", service: "app", target: "/data" });
    expect(v.stackId).toBeUndefined();
    expect(mapTopology([network], "h1", [container], [p], inventory).networks[0]?.stackId).toBeUndefined();
  });

  it.each(["absent", "unowned"])("retains explicit services when container inventory is %s", (kind) => {
    const unowned = mapContainer(container, "h1", [], inventory);
    expect(unowned.stackId).toBeUndefined();
    const stack = mapStack(project, "h1", kind === "absent" ? [] : [unowned], inventory);
    expect(stack.services).toHaveLength(1);
    expect(stack.services[0]).toMatchObject({ name: "app", state: "running", image: { repository: "registry.example.com/app", tag: "1" } });
    expect(stack.runningCount).toBe(1);
  });

  it.each(["current", "old"])("keeps a concrete visible label owner ahead of stale %s-peer runtime IDs", (peer) => {
    const wrong = { ...project, ...(peer === "old" ? { composeProjectName: undefined } : {}) };
    const owner = { ...project, id: "z-owner", name: "beta", composeProjectName: "manual_p", runtimeServices: [] };
    for (const projects of [[wrong, owner], [owner, wrong]]) {
      const c = mapContainer(container, "h1", projects, inventory);
      expect(c.stackId).toBe("h1~z-owner");
      expect(mapStack(wrong, "h1", [c], inventory).services).toEqual([]);
      expect(mapStack(owner, "h1", [c], inventory).services).toHaveLength(1);
    }
  });

  it("does not turn runtime membership or linked consumers into volume/network ownership", () => {
    const p = { ...project, composeProjectName: "beta_stack" };
    for (const containers of [[], [container]]) {
      for (const label of [undefined, "manual_p", "beta_stack"]) {
        const resourceLabels: Record<string, string> = label ? { "com.docker.compose.project": label } : {};
        const v = mapVolumes([{ ...volume, labels: resourceLabels }], "h1", containers, [p], inventory)[0]!;
        expect(v.stackId).toBe(label === "beta_stack" ? "h1~example-project" : undefined);
        const topology = mapTopology([{ ...network, labels: resourceLabels }], "h1", [], [p], inventory);
        expect(topology.networks[0]?.stackId).toBe(label === "beta_stack" ? "h1~example-project" : undefined);
        // Any Compose label protects deletion; only the matching owner supplies a stack link.
        expect(removeBlock(topology, topology.networks[0]!)).toBe(label ? "compose-managed" : undefined);
      }
    }
  });

  it("retains backend non-Compose origins despite explicit runtime IDs", () => {
    for (const [ownLabels, origin] of [[{}, "standalone"], [{ "com.docker.compose.service": "app" }, "unknown"], [{ "com.docker.compose.project": "manual_p" }, "unknown"]] as const) {
      const c = mapContainer({ ...container, labels: ownLabels, origin }, "h1", [project], inventory);
      expect(c.origin).toBe(origin);
      expect(c.stackId).toBeUndefined();
      expect(mapStack(project, "h1", [c], inventory).services).toHaveLength(1);
    }
  });

  it("retains the visible duplicate-label tie rule despite conflicting backend runtime IDs", () => {
    // PI-R4-02: full-table backend maps can select the equal-name observation
    // by ID; this visible projection still prefers the differing observation.
    const discovered = { ...project, id: "a-disc", name: "manual_p", composeProjectName: "manual_p" };
    const override = { ...project, id: "b-over", composeProjectName: "manual_p", runtimeServices: [] };
    for (const projects of [[discovered, override], [override, discovered]]) {
      const c = mapContainer(container, "h1", projects, inventory);
      expect(c.stackId).toBe("h1~b-over");
      expect(mapStack(discovered, "h1", [c], inventory).services).toEqual([]);
      expect(mapStack(override, "h1", [c], inventory).services).toHaveLength(1);
    }
  });
});

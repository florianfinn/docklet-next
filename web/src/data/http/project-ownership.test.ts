// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Original effective Compose-label consequence regression coverage.

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import type { components } from "@/api/generated/schema";
import type { InventoryObservation } from "@/domain/model";
import { removeBlock } from "@/domain/networks";
import { mapContainer, mapStack, type ContainerRow, type ProjectRow } from "./inventory-mapping";
import { mapImage, mapTopology, mapVolumes } from "./resource-mapping";
import identity from "./project-ownership.test-support";

// Go's TestComposeNameResponse* verifies these exact identity fields against
// JSON serialized from the actual migrated-SQLite list and detail services.
const project = (values: Partial<ProjectRow> = {}): ProjectRow => ({ ...identity, path: "/srv/projects/example-project", status: "stopped", serviceCount: 1, runningCount: 0, isArchived: false, createdAt: "", updatedAt: "", tags: [], ...values });
const inventory = { stale: false } as InventoryObservation;
const labels = { "com.docker.compose.project": "beta_stack", "com.docker.compose.service": "web" };
const container: ContainerRow = { autoUpdateEnabled: false, command: "serve", created: 0, ports: [], hostConfig: {}, mounts: [], networkSettings: { networks: {} }, id: "c1", names: ["/beta_stack-web-1"], state: "running", status: "Up", image: "registry.example.com/app:1", imageId: "sha256:example", labels };
const volume: components["schemas"]["VolumeVolume"] = { id: "beta_stack_data", containers: [], inUse: false, options: {}, scope: "local", name: "beta_stack_data", driver: "local", mountpoint: "/srv/data", size: 0, createdAt: "", labels: { ...labels, "com.docker.compose.volume": "data" } };
const network: components["schemas"]["NetworkInspect"] = { scope: "local", created: "", configFrom: { Network: "" }, options: {}, containersList: [], attachable: false, ingress: false, configOnly: false, id: "n1", name: "beta_stack_default", driver: "bridge", internal: false, enableIPv4: true, enableIPv6: false, ipam: { driver: "default", options: {}, config: [] }, labels, containers: {} };

function assertOwners(projects: ProjectRow[], expected = "example-project") {
 const c = mapContainer(container, "h1", projects, inventory);
 expect(c.stackId).toBe(`h1~${expected}`);
 expect(c.stackName).toBe(projects.find((p) => p.id === expected)?.name);
 for (const p of projects) {
  const stack = mapStack(p, "h1", [c], inventory);
  expect(stack.id).toBe(`h1~${p.id}`);
  expect(stack.name).toBe(p.name);
  expect(stack.services.length).toBe(p.id === expected ? 1 : 0);
 }
 const v = mapVolumes([volume], "h1", [], projects, inventory)[0]!;
 expect(v.stackId).toBe(`h1~${expected}`);
 expect(v.consumers).toEqual([]);
 const topology = mapTopology([network], "h1", [], projects, inventory);
 expect(topology.attachments).toEqual([]);
 expect(topology.networks[0]?.stackId).toBe(`h1~${expected}`);
 expect(removeBlock(topology, topology.networks[0]!)).toBe("compose-managed");
 const image = mapImage({ id: "sha256:example", size: 0, created: 0, repoTags: ["registry.example.com/app:1"] } as components["schemas"]["ImageSummary"], "h1", [container], projects, inventory);
 expect(image.consumers[0]?.stackId).toBe(`h1~${expected}`);
}

describe("effective Compose label across actual HTTP mappings", () => {
 it("keeps the generated observation optional and distinct from the required display name", () => {
  const spec = JSON.parse(readFileSync("../api/openapi.json", "utf8"));
  const schema = spec.components.schemas.ProjectDetails;
  expect(schema.properties.composeProjectName.type).toBe("string");
  expect(schema.required).not.toContain("composeProjectName");
  expect(schema.required).toContain("name");
 });
 it("maps volumes independently when there are zero containers", () => {
  const mapped = mapVolumes([volume], "h1", [], [project()], inventory)[0]!;
  expect(mapped.stackId).toBe("h1~example-project");
  expect(mapped.stackName).toBe("alpha");
  expect(mapped.consumers).toEqual([]);
 });
 it("preserves the actual domain removal refusal for an unattached Compose network", () => {
  const topology = mapTopology([network], "h1", [], [project()], inventory);
  expect(topology.attachments).toEqual([]);
  expect(removeBlock(topology, topology.networks[0]!)).toBe("compose-managed");
  expect(topology.networks[0]?.stackId).toBe("h1~example-project");
 });
 it("does not substitute a coexisting stale-default container group for effective members", () => {
  const rows = [container, { ...container, id: "stale", labels: { ...labels, "com.docker.compose.project": "alpha" } }];
  const mapped = rows.map((row) => mapContainer(row, "h1", [project()], inventory));
  expect(mapped[0]?.stackId).toBe("h1~example-project");
  expect(mapped[1]?.stackId).toBeUndefined();
  expect(mapStack(project(), "h1", mapped, inventory).services).toHaveLength(1);
 });
 it("preserves stable display and IDs, including volume and unattached network ownership without containers", () => assertOwners([project()]));
 it("prefers observed owners over legacy aliases in either response order", () => {
  const owner = project();
  for (const metadata of [undefined, "", "beta_stack"]) {
   const alias = project({ id: "a-alias", name: "Beta_Stack", composeProjectName: metadata });
   for (const rows of [[alias, owner], [owner, alias]]) assertOwners(rows);
  }
 });
 it("prefers an initial-discovery exact name over a normalized legacy alias", () => {
  const owner = project({ name: "beta_stack" });
  const alias = project({ id: "a-alias", name: "Beta_Stack", composeProjectName: "beta_stack" });
  for (const rows of [[alias, owner], [owner, alias]]) assertOwners(rows);
 });
 it("does not let a stale runtime ID or default displace a conflicting observed label owner", () => {
  const wrong = project({ id: "a-wrong", name: "beta_stack", composeProjectName: "other_stack", runtimeServices: [{ name: "web", image: container.image, status: "running", containerId: container.id }] });
  for (const rows of [[wrong, project()], [project(), wrong]]) {
   const c = mapContainer(container, "h1", rows, inventory);
   expect(c.stackId).toBe("h1~example-project");
   expect(mapStack(wrong, "h1", [c], inventory).services).toEqual([]);
  }
 });
 it("retains bounded old-peer runtime ID fallback without inventing volume/network ownership", () => {
  const old = project({ composeProjectName: undefined, runtimeServices: [{ name: "web", image: container.image, status: "running", containerId: container.id }] });
  const c = mapContainer(container, "h1", [old], inventory);
  expect(c.stackId).toBe("h1~example-project");
  expect(mapStack(old, "h1", [c], inventory).services).toHaveLength(1);
  expect(mapVolumes([volume], "h1", [], [old], inventory)[0]?.stackId).toBeUndefined();
  expect(mapTopology([network], "h1", [], [old], inventory).networks[0]?.stackId).toBeUndefined();
 });
 it.each([
  ["Legacy Custom!", "legacycustom"], ["__--Foo_Bar!", "foo_bar"], ["!!!", "!!!"],
 ])("matches backend normalized nil/empty fallback for %s", (name, label) => {
  for (const metadata of [undefined, "", label]) {
   const p = project({ name, composeProjectName: metadata });
   const raw = { ...container, labels: { ...labels, "com.docker.compose.project": label } };
   expect(mapContainer(raw, "h1", [p], inventory).stackId).toBe("h1~example-project");
  }
 });
 it("does not retain removed observations, claim unlabelled resources or match other owners", () => {
  const removed = project({ composeProjectName: "alpha" });
  expect(mapContainer(container, "h1", [removed], inventory).stackId).toBeUndefined();
  const otherLabels: Record<string, string>[] = [{}, { "com.docker.compose.project": "other_stack" }];
  for (const ownLabels of otherLabels) {
   expect(mapVolumes([{ ...volume, labels: ownLabels }], "h1", [], [project()], inventory)[0]?.stackId).toBeUndefined();
   const topology = mapTopology([{ ...network, labels: ownLabels }], "h1", [], [project()], inventory);
   expect(topology.networks[0]?.stackId).toBeUndefined();
   // An external Compose label protects deletion without inventing a hub stack link.
   expect(removeBlock(topology, topology.networks[0]!)).toBe(ownLabels["com.docker.compose.project"] ? "compose-managed" : undefined);
  }
 });
});

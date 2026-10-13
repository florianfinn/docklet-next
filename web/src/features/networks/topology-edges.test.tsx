import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import type { ContainerInfo, Host, Topology } from "@/domain/model";

import { containerHeight, networkSize } from "./topology-geometry";
import { decorate, layoutHost, type ContainerData, type NetworkData, type TopoNode } from "./topology-layout";

let CONTAINERS: ContainerInfo[] = [];
let TOPO: Topology;
beforeEach(async () => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  const adapter = createDemoAdapter();
  CONTAINERS = await adapter.listContainers();
  TOPO = await adapter.getTopology();
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const host = (id: string) => HOSTS.find((h) => h.id === id) as Host;
const opts = { query: "", onlyProblems: false, showDependencies: true };
const layoutOf = (id: string, topology: Topology = TOPO) => layoutHost({ topology, containers: CONTAINERS, host: host(id), expanded: new Set(topology.networks.filter((n) => n.hostId === id).map((n) => n.id)) });
const lines = (d: ContainerData) => d.addresses.reduce((s, a) => s + (a.address6 ? 2 : 1), 0);
const box = (n: TopoNode) => ({ x: n.position.x, y: n.position.y, w: (n.data as NetworkData | ContainerData).width, h: n.type === "network" ? networkSize((n.data as NetworkData).network).height : containerHeight(lines(n.data as ContainerData)) });

describe("topology edges", () => {
  it("draws exactly the configured dependency broker → webserver and memberships as a side bus, never a chain", () => {
    const { edges } = decorate(layoutOf("atlas"), { ...opts, focus: null });
    const paperless = edges.filter((e) => e.source.startsWith("c-paperless") || e.target.startsWith("c-paperless"));
    const deps = paperless.filter((e) => e.className === "edge-dependency").map((e) => `${e.source}>${e.target}`);
    expect(deps).toEqual(["c-paperless-broker>c-paperless-webserver"]);
    expect(paperless.find((e) => e.className === "edge-dependency")?.markerEnd !== undefined).toBe(true);
    // Membership edges all start at the network and enter each container from the side.
    const members = paperless.filter((e) => e.className === "edge-membership");
    expect([members.length, members.every((e) => e.source === "n-atlas-paperless" && e.sourceHandle === "m" && e.targetHandle === "m"), members.some((e) => e.targetHandle === "t" || e.sourceHandle === "b")]).toEqual([3, true, false]);
    // No edge of any kind between db and the others.
    expect(paperless.some((e) => e.className === "edge-dependency" && (e.source === "c-paperless-db" || e.target === "c-paperless-db"))).toBe(false);
  });

  it("keeps focus addresses on the node and the layout identical while the selection changes", () => {
    const layout = layoutOf("atlas");
    const a = decorate(layout, { ...opts, focus: "c-paperless-webserver" });
    const b = decorate(layout, { ...opts, focus: "c-paperless-broker" });
    const c = decorate(layout, { ...opts, focus: null });
    const web = (g: typeof a) => g.nodes.find((n) => n.id === "c-paperless-webserver") as TopoNode;
    expect([(web(a).data as ContainerData).addresses.map((x) => x.address), (web(c).data as ContainerData).addresses]).toEqual([["198.51.100.162"], []]);
    expect([a, b, c].every((g) => g.nodes.every((n, i) => n.position === layout.nodes[i]?.position))).toBe(true);
    // Edges never carry address labels.
    expect(a.edges.every((e) => e.label === undefined)).toBe(true);
  });

  it("organises containers without networks in their own lane and nothing overlaps", () => {
    const topology = { ...TOPO, attachments: TOPO.attachments.filter((x) => x.containerId !== "c-whoami-test" && x.containerId !== "c-uptime-kuma") };
    const layout = layoutOf("boreal", topology);
    const lane = layout.nodes.find((n) => n.id === "lane-isolated");
    const isolated = layout.nodes.filter((n) => n.id === "c-whoami-test" || n.id === "c-uptime-kuma");
    expect([lane?.type, isolated.length, isolated.every((n) => n.position.y > (lane?.position.y ?? Infinity))]).toEqual(["lane", 2, true]);
    const boxes = layout.nodes.filter((n) => n.type !== "lane").map(box);
    const overlap = boxes.some((p, i) => boxes.some((q, j) => i < j && p.x < q.x + q.w && q.x < p.x + p.w && p.y < q.y + q.h && q.y < p.y + p.h));
    expect(overlap).toBe(false);
  });

  it("shows both addresses of a dual-stack member and none it does not know", () => {
    const layout = layoutOf("atlas");
    const { nodes } = decorate(layout, { ...opts, focus: "n-atlas-br0" });
    expect((nodes.find((n) => n.id === "c-krusader")?.data as ContainerData).addresses.map((x) => [x.address, x.address6])).toEqual([["192.0.2.201", "2001:db8:a::201"]]);
  });
});

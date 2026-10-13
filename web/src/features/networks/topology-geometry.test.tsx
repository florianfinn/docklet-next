import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import type { ContainerInfo, Host, Topology, TopologyNetwork } from "@/domain/model";
import { renderRoutes } from "@/test/render";

import NetworksPage from "./NetworksPage";
import { CANVAS_MIN_H, canvasHeight, containerHeight, FIT_PAD, FIT_ZOOM, fitViewport, layoutBudget, LINE, membershipPoints, NAME_MAX_LINES, networkNameLines, networkSize, NODE_MAX_W, NODE_MIN_W, textWidth, titleWidth } from "./topology-geometry";
import { decorate, layoutHost, type ContainerData, type HostLayout, type NetworkData, type TopoNode } from "./topology-layout";

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
  vi.restoreAllMocks();
});

const host = (id: string) => HOSTS.find((h) => h.id === id) as Host;

/**
 * Atlas plus three dual-stack networks with long names, one IPv6-only
 * network, and members in several of them with both families (RFC 5737 and
 * RFC 3849 addresses only).
 */
function richAtlas(): { topology: Topology; containers: ContainerInfo[] } {
  const template = CONTAINERS.find((c) => c.hostId === "atlas")!;
  const net = (i: number, name: string, extra: Partial<TopologyNetwork> = {}): TopologyNetwork => ({
    id: `n-rich-${i}`,
    name,
    hostId: "atlas",
    driver: "bridge",
    subnet: `198.51.100.${192 + i * 16}/28`,
    gateway: `198.51.100.${193 + i * 16}`,
    ipv6: true,
    subnet6: `2001:db8:4d2:${i}f00:abcd::/80`,
    internal: false,
    ...extra,
  });
  const networks = [
    net(0, "observability_pipeline_collector_default", { stackId: "observability" }),
    net(1, "home_automation_zigbee_bridge_network"),
    net(2, "backup_offsite_replication_internal", { internal: true }),
    net(3, "lab_v6_only", { subnet: undefined, gateway: undefined, ipv4: false }),
  ];
  const containers = ["observability-opentelemetry-collector-contrib", "zigbee2mqtt", "restic-offsite-replicator"].map((name, i): ContainerInfo => ({ ...template, id: `c-rich-${i}`, name, serviceName: name, stackId: i === 0 ? "observability" : undefined, dockerLabels: undefined }));
  const v6 = (i: number, n: number) => `2001:db8:4d2:${n}f00:abcd:ef01:2345:${(100 + i).toString(16)}`;
  const attachments = containers.flatMap((c, i) => networks.map((n, j) => ({ containerId: c.id, networkId: n.id, ip: n.subnet ? `198.51.100.${194 + j * 16 + i}` : undefined, ip6: v6(i, j) })));
  return {
    topology: { networks: [...TOPO.networks, ...networks], attachments: [...TOPO.attachments, ...attachments], dependencies: [...TOPO.dependencies, { from: "c-rich-1", to: "c-rich-0" }, { from: "c-rich-2", to: "c-rich-0" }] },
    containers: [...CONTAINERS, ...containers],
  };
}

const expandedAll = (topology: Topology, id: string) => new Set(topology.networks.filter((n) => n.hostId === id).map((n) => n.id));
const lines = (d: ContainerData) => d.addresses.reduce((s, a) => s + (a.address6 ? 2 : 1), 0);
const box = (n: TopoNode) => {
  const d = n.data as NetworkData | ContainerData;
  const h = n.type === "network" ? networkSize((d as NetworkData).network).height : n.type === "container" ? containerHeight(lines(d as ContainerData)) : 34;
  return { id: n.id, x: n.position.x, y: n.position.y, w: n.type === "lane" ? (n.data.width as number) : d.width, h };
};
const overlaps = (nodes: TopoNode[]) => {
  const boxes = nodes.map(box);
  return boxes.flatMap((p, i) => boxes.slice(i + 1).filter((q) => p.x < q.x + q.w && q.x < p.x + p.w && p.y < q.y + q.h && q.y < p.y + p.h).map((q) => `${p.id}/${q.id}`));
};
const within = (layout: HostLayout, nodes: TopoNode[]) => nodes.map(box).every((b) => b.x + b.w <= layout.bounds.width && b.y + b.h <= layout.bounds.height);

describe("content-driven topology geometry", () => {
  it("widens a column to its longest name or address and keeps every block of the column at that width", () => {
    const { topology, containers } = richAtlas();
    const layout = layoutHost({ topology, containers, host: host("atlas"), expanded: expandedAll(topology, "atlas") });
    const nets = layout.nodes.filter((n) => n.type === "network");
    const long = nets.find((n) => n.id === "n-rich-0")!;
    // The long stack network gets more than the minimum, yet stays readable instead of growing without limit.
    expect([long.data.width > NODE_MIN_W, long.data.width <= NODE_MAX_W, long.data.width >= textWidth("observability_pipeline_collector_default", "title")]).toEqual([true, true, true]);
    // Short names keep the compact width (plain Atlas data).
    const plain = layoutHost({ topology: TOPO, containers: CONTAINERS, host: host("atlas"), expanded: expandedAll(TOPO, "atlas") });
    expect(plain.nodes.find((n) => n.id === "n-atlas-media")?.data.width).toBe(NODE_MIN_W);
    // Members stacked under a network share its width, so the column reads as one block.
    const widthOf = new Map(nets.map((n) => [n.id, n.data.width]));
    const members = layout.nodes.filter((n) => n.type === "container" && layout.homeOf.has(n.id));
    expect([members.length > 10, members.every((n) => (n.data as ContainerData).width === widthOf.get(layout.homeOf.get(n.id)!))]).toEqual([true, true]);
  });

  it("gives dual-stack networks one more subnet line and IPv6 addresses their own line", () => {
    const { topology, containers } = richAtlas();
    const dual = networkSize(topology.networks.find((n) => n.id === "n-rich-1")!);
    const single = networkSize(topology.networks.find((n) => n.id === "n-atlas-media")!);
    const v6only = networkSize(topology.networks.find((n) => n.id === "n-rich-3")!);
    expect([dual.height - single.height, v6only.height]).toEqual([LINE.meta, single.height]);
    const layout = layoutHost({ topology, containers, host: host("atlas"), expanded: expandedAll(topology, "atlas") });
    const focused = decorate(layout, { query: "", onlyProblems: false, showDependencies: true, focus: "c-rich-1" });
    const own = (focused.nodes.find((n) => n.id === "c-rich-1")!.data as ContainerData).addresses;
    // Three dual-stack rows and the IPv6-only one: 4 networks, 7 address lines, nothing joined into one line.
    expect([own.length, lines(focused.nodes.find((n) => n.id === "c-rich-1")!.data as ContainerData), own.every((a) => !a.address?.includes(" "))]).toEqual([4, 7, true]);
  });

  it("wraps network names wider than the column cap onto whole extra lines, reserving their height", () => {
    const { topology, containers } = richAtlas();
    // Measured in Chromium: 30 × W is 429 px, far above the 8.1 px average estimate (243 px).
    expect([titleWidth("W".repeat(30)) >= 429, titleWidth("observability_pipeline_collector_default") >= 254, titleWidth("observability_pipeline_collector_default") < textWidth("observability_pipeline_collector_default", "title")]).toEqual([true, true, true]);
    const wide = (i: number, name: string, extra: Partial<TopologyNetwork> = {}): TopologyNetwork => ({ id: `n-wide-${i}`, name, hostId: "atlas", driver: "bridge", subnet: `192.0.2.${i * 16}/28`, internal: false, ...extra });
    const nets = [wide(0, "WWWWMMMMWWWWMMMM_WWWWMMMM_net"), wide(1, "MMMMWWWW_mmmm_wwww_" + "W".repeat(40), { ipv6: true, subnet6: "2001:db8:5::/64", internal: true }), wide(2, "W".repeat(200))];
    const base = networkSize({ ...nets[0]!, name: "short" }).height;
    const sized = nets.map((n) => ({ ...networkSize(n), lines: networkNameLines(n, networkSize(n).width) }));
    // A name that fits under the cap widens the block (estimate with W/M widths); past the cap it wraps.
    expect(sized.map((s) => [s.width, s.lines, s.height - base - (s.lines - 1) * LINE.title])).toEqual([
      [2 + 24 + 20 + titleWidth(nets[0]!.name), 1, 0],
      [NODE_MAX_W, 3, LINE.meta],
      [NODE_MAX_W, NAME_MAX_LINES, 0],
    ]);
    // Ordinary names stay on one line at their usual size.
    expect(topology.networks.every((n) => networkNameLines(n, networkSize(n).width) === 1)).toBe(true);
    // With the wrapped names in place, no block overlaps another, focused or not.
    const extra = { ...topology, networks: [...topology.networks, ...nets], attachments: [...topology.attachments, ...nets.map((n, i) => ({ containerId: `c-rich-${i}`, networkId: n.id, ip: `192.0.2.${i * 16 + 2}` }))] };
    for (const budget of [Infinity, layoutBudget(420), layoutBudget(1000)]) {
      const layout = layoutHost({ topology: extra, containers, host: host("atlas"), expanded: expandedAll(extra, "atlas"), budget });
      for (const focus of [null, ...nets.map((n) => n.id), "c-rich-1"]) {
        const { nodes } = decorate(layout, { query: "", onlyProblems: false, showDependencies: true, focus });
        expect([overlaps(nodes), within(layout, nodes)]).toEqual([[], true]);
      }
    }
  });

  it("never overlaps blocks or lane headings, whatever is focused, filtered or collapsed", () => {
    const { topology, containers } = richAtlas();
    const ids = [...expandedAll(topology, "atlas")];
    for (const [budget, expanded] of [[Infinity, new Set(ids)], [layoutBudget(356), new Set(ids)], [layoutBudget(1000), new Set(ids.slice(0, 4))], [layoutBudget(640), new Set<string>()]] as const) {
      const layout = layoutHost({ topology, containers, host: host("atlas"), expanded, budget });
      const placed = layout.nodes.filter((n) => n.type !== "lane").map((n) => n.id);
      for (const focus of [null, ...placed]) {
        const { nodes } = decorate(layout, { query: focus ? "" : "zigbee", onlyProblems: false, showDependencies: true, focus });
        expect(overlaps(nodes)).toEqual([]);
        expect(within(layout, nodes)).toBe(true);
      }
    }
  });

  it("routes memberships through free bands and the rail, never across a block", () => {
    const { topology, containers } = richAtlas();
    for (const width of [356, 1000, 2400]) {
      const layout = layoutHost({ topology, containers, host: host("atlas"), expanded: expandedAll(topology, "atlas"), budget: layoutBudget(width) });
      const { nodes, edges } = decorate(layout, { query: "", onlyProblems: false, showDependencies: true, focus: null });
      const byId = new Map(nodes.map((n) => [n.id, n]));
      const blocks = nodes.filter((n) => n.type !== "lane").map(box);
      const hits: string[] = [];
      for (const e of edges.filter((x) => x.type === "membership")) {
        const s = box(byId.get(e.source)!);
        const t = box(byId.get(e.target)!);
        // Handle positions as the node components place them.
        const points = membershipPoints({ sx: s.x, sy: s.y + s.h - 17, tx: t.x, ty: t.y + 22, ...(e.data as { from: number; to: number }) });
        points.slice(1).forEach(([x2, y2], i) => {
          const [x1, y1] = points[i]!;
          const seg = { x: Math.min(x1, x2), y: Math.min(y1, y2), w: Math.abs(x2 - x1), h: Math.abs(y2 - y1) };
          // Ends inside the own source and target blocks are allowed (the edge enters them there).
          for (const b of blocks) if (b.id !== s.id && b.id !== t.id && seg.x < b.x + b.w && b.x < seg.x + seg.w && seg.y < b.y + b.h && b.y < seg.y + seg.h) hits.push(`${e.id}@${width}:${b.id}`);
        });
        expect(points.every(([x]) => x >= layout.bounds.x)).toBe(true);
      }
      expect(hits).toEqual([]);
    }
  });

  it("packs lane rows into the canvas width", () => {
    const { topology, containers } = richAtlas();
    const at = (width: number) => layoutHost({ topology, containers, host: host("atlas"), expanded: expandedAll(topology, "atlas"), budget: layoutBudget(width) });
    const columns = (layout: HostLayout) => new Set(layout.nodes.filter((n) => n.type !== "lane").map((n) => n.position.x)).size;
    const phone = at(356);
    const desktop = at(1400);
    // A phone gets one readable column; the desktop spreads the user networks over several, never wider than its budget.
    expect([columns(phone), Math.max(...phone.nodes.filter((n) => n.type !== "lane").map((n) => box(n).x + box(n).w)) <= NODE_MAX_W]).toEqual([1, true]);
    const rightEdge = (l: HostLayout) => Math.max(...l.nodes.filter((n) => n.type !== "lane").map((n) => box(n).x + box(n).w));
    expect([columns(desktop) >= 4, rightEdge(desktop) <= layoutBudget(1400), desktop.bounds.height < phone.bounds.height]).toEqual([true, true, true]);
    // A very wide canvas takes the whole user lane in one row instead of stacking.
    const ultra = at(2400);
    const userRow = (l: HostLayout) => new Set(l.nodes.filter((n) => n.type === "network" && !n.data.network.builtIn && n.data.network.driver === "bridge").map((n) => n.position.y)).size;
    expect([userRow(ultra), userRow(desktop) > 1, layoutBudget(0)]).toEqual([1, true, Infinity]);
  });

  it("sizes the canvas by its fitted content and fits legibly", () => {
    const { topology, containers } = richAtlas();
    const delta = layoutHost({ topology: TOPO, containers: CONTAINERS, host: host("delta"), expanded: expandedAll(TOPO, "delta") });
    const rich = layoutHost({ topology, containers, host: host("atlas"), expanded: expandedAll(topology, "atlas"), budget: layoutBudget(1100) });
    const collapsed = layoutHost({ topology, containers, host: host("atlas"), expanded: new Set(), budget: layoutBudget(1100) });
    const [small, large, flat] = [canvasHeight(delta.bounds, 1100), canvasHeight(rich.bounds, 1100), canvasHeight(collapsed.bounds, 1100)];
    expect([small >= CANVAS_MIN_H, small < large, flat < large, canvasHeight(rich.bounds, 0)]).toEqual([true, true, true, CANVAS_MIN_H]);
    // Small content is centred and never blown up past the maximum zoom.
    const fitSmall = fitViewport(delta.bounds, { width: 1100, height: small });
    const screenLeft = fitSmall.x + delta.bounds.x * fitSmall.zoom;
    expect([fitSmall.zoom <= FIT_ZOOM.max, Math.abs(screenLeft * 2 + delta.bounds.width * fitSmall.zoom - 1100) <= 2]).toEqual([true, true]);
    // Content taller than a capped canvas starts at the top left at the smallest legible zoom and pans from there.
    const capped = fitViewport(rich.bounds, { width: 360, height: 500 });
    expect([capped.zoom, capped.x, capped.y]).toEqual([FIT_ZOOM.min, Math.round(FIT_PAD.x - rich.bounds.x * FIT_ZOOM.min), FIT_PAD.top]);
  });
});

describe("topology canvas on the page", () => {
  /** happy-dom has no layout: report a canvas width so the page sizes and fits it. */
  const mockCanvas = (width: number, height: number) => {
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockImplementation(function (this: HTMLElement) {
      return this.dataset.slot === "topology-canvas" ? width : 0;
    });
    vi.spyOn(HTMLElement.prototype, "clientHeight", "get").mockImplementation(function (this: HTMLElement) {
      return this.dataset.slot === "topology-canvas" ? height : 0;
    });
  };
  const canvas = () => document.querySelector<HTMLElement>('[data-slot="topology-canvas"]')!;
  const px = () => parseFloat(canvas().style.height);

  it("follows the content height when networks collapse and expand", async () => {
    mockCanvas(1000, 640);
    renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=atlas"]);
    await screen.findByRole("region", { name: "Netzwerke auf Atlas" });
    await waitFor(() => expect(px() > CANVAS_MIN_H).toBe(true));
    const expanded = px();
    await userEvent.click(screen.getByRole("button", { name: "Alle zuklappen" }));
    await waitFor(() => expect(px() < expanded).toBe(true));
    await userEvent.click(screen.getByRole("button", { name: "Alle aufklappen" }));
    await waitFor(() => expect(px()).toBe(expanded));
  });

  it("lists both addresses of a dual-stack member on separate lines of its block", async () => {
    mockCanvas(1000, 640);
    renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=atlas&node=c-krusader"]);
    const block = await waitFor(() => {
      const el = document.querySelector<HTMLElement>('.react-flow__node[data-id="c-krusader"] [data-slot="focus-addresses"]');
      if (!el) throw new Error("addresses missing");
      return el;
    });
    const texts = [...block.querySelectorAll<HTMLElement>(".font-mono > span")].map((s) => s.textContent);
    expect(texts).toEqual(["192.0.2.201", "2001:db8:a::201"]);
  });
});
